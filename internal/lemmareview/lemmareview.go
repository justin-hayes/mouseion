// Package lemmareview owns the Book-scoped lemma review workflow: it assesses
// lemma risk, reviews exact occurrences, and previews and confirms keep,
// correction, or exclusion decisions. Handlers render its results and map its
// sentinel errors to responses; nothing here knows about HTTP.
package lemmareview

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/lemmarisk"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
)

const (
	// assessedLanguage is the only language lemma risk is detected for.
	assessedLanguage = "de"
	// assessmentTimeout bounds local detection. An assessment that runs past it
	// counts as not assessed rather than as a clean verdict.
	assessmentTimeout = 250 * time.Millisecond
	suggestionTimeout = 3 * time.Second
)

// Store is the narrow persistence port the workflow needs. The Postgres store
// and the fixture store both satisfy it.
type Store interface {
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
	GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error)
	StartCurrentReading(context.Context, string, string, string) (domain.CurrentReading, error)
	ListLemmaReviewOccurrences(context.Context, string, string, string) ([]domain.LemmaReviewOccurrence, error)
	SaveLemmaReviewFlags(context.Context, []domain.LemmaReviewFlag) error
	ReadLemmaReviewProposal(context.Context, domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error)
	PutLemmaDecisionProposal(context.Context, domain.LemmaReviewProposal, string) error
	ListDeckPreparationsForSourceMaterial(context.Context, string, string) ([]domain.DeckPreparation, error)
}

// Preparer submits the deck preparations that follow a confirmed identity change.
type Preparer interface {
	SubmitForCurrentReading(context.Context, string, string, string) (prepareddeck.Handle, error)
	Reprepare(context.Context, string, string) (prepareddeck.Handle, error)
}

// BrowseCounts schedules the vocabulary count rebuild a committed decision needs.
type BrowseCounts interface {
	EnqueueBrowseCountRebuild(context.Context, string, string) error
}

// Config supplies a Service's collaborators. RiskIndex, Suggestions, Preparer and
// BrowseCounts may be nil; the workflow then runs without the optional step.
type Config struct {
	Store        Store
	RiskIndex    lemmarisk.AlternativeIndex
	Suggestions  enrichment.LemmaSuggestionProvider
	Preparer     Preparer
	BrowseCounts BrowseCounts
}

// Service is the lemma review workflow.
type Service struct {
	store        Store
	riskIndex    lemmarisk.AlternativeIndex
	suggestions  enrichment.LemmaSuggestionProvider
	preparer     Preparer
	browseCounts BrowseCounts
}

func New(cfg Config) *Service {
	return &Service{store: cfg.Store, riskIndex: cfg.RiskIndex, suggestions: cfg.Suggestions, preparer: cfg.Preparer, browseCounts: cfg.BrowseCounts}
}

// Assessment reports whether lemma risk was assessed for a Book, and whether
// any of its occurrences still awaits a learner decision.
type Assessment struct {
	Assessed   bool
	Unresolved bool
}

// Recovery is what a reviewer needs to know about a Book's existing reading and
// deck artifacts before changing its vocabulary identities.
type Recovery struct {
	ActiveReading       bool
	ActiveReadingBookID string
	ReadyPreparationID  string
	ReadyDeckSnapshotID string
	HasCompletedReading bool
	HasIdentityDecision bool
	// ReferenceAssessed is set by the caller from the Assessment that preceded the page.
	ReferenceAssessed bool
}

// Review is the occurrence list for one form, or for every flagged occurrence
// when no form is given, with whether correction is allowed.
type Review struct {
	Occurrences []domain.LemmaReviewOccurrence
	CanCorrect  bool
	Recovery    Recovery
}

// Suggestion is the outcome of one optional LLM lemma suggestion. It is never applied.
type Suggestion struct {
	OccurrenceID string
	Status       SuggestionStatus
	Lemma        string
	Provider     string
	Version      string
}

type SuggestionStatus int

const (
	SuggestionNotConfigured SuggestionStatus = iota
	SuggestionUnavailable
	SuggestionOffered
)

// Proposal is one learner decision as requested: the form its occurrences were
// found by, the action, the lemma for a correction, and the occurrence IDs it
// applies to. The workflow resolves the IDs against the current analysis.
type Proposal struct {
	Language      string
	Form          string
	Action        string
	Lemma         string
	OccurrenceIDs []string
}

// Change shows one selected occurrence's effective identity before and after.
type Change struct{ Sentence, Before, After string }

// Preview is what confirming a proposal would do, bound to the fingerprint of
// the state it was previewed against.
type Preview struct {
	Action            string
	Lemma             string
	OccurrenceIDs     []string
	Changes           []Change
	Impacts           []selection.Impact
	Fingerprint       string
	IdentityChanged   bool
	RequiresReprepare bool
}

// OutcomeKind is how far a committed confirmation got. The decision itself is
// committed in every case; the kinds describe the follow-on steps.
type OutcomeKind int

const (
	// OutcomeSaved: the decision is saved and no re-preparation was needed.
	OutcomeSaved OutcomeKind = iota
	// OutcomeFlagsUnresolved: the decision is saved, but restarting the Book is
	// blocked until its unresolved flags are reviewed.
	OutcomeFlagsUnresolved
	// OutcomePreparing: the decision is saved and a new deck preparation is queued.
	OutcomePreparing
	// OutcomeRestartFailed: the decision is saved, but the Book could not be restarted.
	OutcomeRestartFailed
	// OutcomeQueueingFailed: the decision is saved, but re-preparation could not be queued.
	OutcomeQueueingFailed
)

// Outcome is the result of a confirmation. PreparationID is set for OutcomePreparing.
type Outcome struct {
	Kind          OutcomeKind
	PreparationID string
}

// Assess runs local lemma risk detection for a Book, saves the flags it finds
// without reopening resolved ones, and reports whether any flag is unresolved.
// A German Book with no usable index, or one whose detection fails or times out,
// is not assessed; that is never an error.
func (s *Service) Assess(ctx context.Context, owner, bookID string) (Assessment, error) {
	detail, err := s.store.GetBookDetail(ctx, owner, bookID)
	if err != nil {
		return Assessment{}, err
	}
	assessed, err := s.detect(ctx, owner, bookID, detail)
	if err != nil {
		return Assessment{}, err
	}
	occurrences, err := s.store.ListLemmaReviewOccurrences(ctx, owner, bookID, "")
	if err != nil {
		return Assessment{}, err
	}
	return Assessment{Assessed: assessed, Unresolved: anyUnresolved(occurrences)}, nil
}

func (s *Service) detect(ctx context.Context, owner, bookID string, detail domain.MyBook) (bool, error) {
	if s.riskIndex == nil || detail.Acquired == nil || detail.Book.LanguageTag != assessedLanguage {
		return false, nil
	}
	occurrences, err := s.store.ListLemmaReviewOccurrences(ctx, owner, bookID, "")
	if err != nil {
		return false, err
	}
	input := make([]lemmarisk.Occurrence, 0, len(occurrences))
	byID := make(map[string]domain.LemmaReviewOccurrence, len(occurrences))
	for _, occurrence := range occurrences {
		id := occurrence.ID()
		input = append(input, lemmarisk.Occurrence{
			ID: id, Language: assessedLanguage, Surface: occurrence.Surface,
			Lemma: selection.ReviewIdentityNow(assessedLanguage, occurrence).CanonicalLemma, UPOS: occurrence.UPOS, Sentence: occurrence.SentenceText,
			Reviewed: occurrence.CorrectedLemma != "" || occurrence.ReviewFlagResolution != "", Excluded: occurrence.Excluded,
		})
		byID[id] = occurrence
	}
	assessmentCtx, cancel := context.WithTimeout(ctx, assessmentTimeout)
	defer cancel()
	flags, assessed, err := lemmarisk.Detect(assessmentCtx, assessedLanguage, input, s.riskIndex)
	if err != nil || !assessed {
		// A failed optional local index is not a clean assessment or a Reading blocker.
		return false, nil
	}
	persisted := make([]domain.LemmaReviewFlag, 0, len(flags))
	for _, flag := range flags {
		occurrence, ok := byID[flag.OccurrenceID]
		if !ok {
			continue
		}
		persisted = append(persisted, domain.LemmaReviewFlag{Occurrence: occurrence, Reason: flag.Reason, Provenance: map[string]any{
			"alternative_lemma": flag.Alternative.Lemma, "source": flag.Alternative.Source,
			"version": flag.Alternative.Version, "evidence_id": flag.Alternative.EvidenceID,
		}})
	}
	if err := s.store.SaveLemmaReviewFlags(ctx, persisted); err != nil {
		return false, err
	}
	return true, nil
}

// Review lists the occurrences of one exact form, or the flagged occurrences
// when form is empty, and what the learner may do with them.
func (s *Service) Review(ctx context.Context, owner, bookID, form string) (Review, error) {
	_, recovery, err := s.load(ctx, owner, bookID)
	if err != nil {
		return Review{}, err
	}
	review := Review{CanCorrect: !recovery.ActiveReading, Recovery: recovery}
	if form != "" {
		review.Occurrences, err = s.store.ListLemmaReviewOccurrences(ctx, owner, bookID, form)
		if err != nil {
			return Review{}, err
		}
		review.Recovery.HasIdentityDecision = hasDecision(review.Occurrences)
		return review, nil
	}
	all, err := s.store.ListLemmaReviewOccurrences(ctx, owner, bookID, "")
	if err != nil {
		return Review{}, err
	}
	for _, occurrence := range all {
		if occurrence.ReviewFlagReason != "" {
			review.Occurrences = append(review.Occurrences, occurrence)
		}
	}
	return review, nil
}

// Suggest asks the optional LLM provider for a lemma for one occurrence. The
// answer is advisory: nothing is saved, and a missing or failed provider is a
// status rather than an error.
func (s *Service) Suggest(ctx context.Context, owner, bookID, occurrenceID string) (Suggestion, error) {
	detail, err := s.store.GetBookDetail(ctx, owner, bookID)
	if err != nil {
		return Suggestion{}, err
	}
	all, err := s.store.ListLemmaReviewOccurrences(ctx, owner, bookID, "")
	if err != nil {
		return Suggestion{}, err
	}
	occurrence, ok := findOccurrence(all, occurrenceID)
	if !ok {
		return Suggestion{}, domain.ErrLemmaReviewOccurrenceMissing
	}
	suggestion := Suggestion{OccurrenceID: occurrenceID, Status: SuggestionNotConfigured}
	if s.suggestions == nil {
		return suggestion, nil
	}
	evidence := EvidenceOf(occurrence)
	suggestCtx, cancel := context.WithTimeout(ctx, suggestionTimeout)
	defer cancel()
	result, err := s.suggestions.SuggestLemma(suggestCtx, enrichment.LemmaSuggestionRequest{
		Language: detail.Book.LanguageTag, Surface: occurrence.Surface, AnalyzedLemma: occurrence.CanonicalLemma,
		UPOS: occurrence.UPOS, Sentence: occurrence.SentenceText, LexicalAlternative: evidence.Alternative,
		LexicalSource: evidence.Source, LexicalVersion: evidence.Version, LexicalEvidenceID: evidence.EvidenceID,
	})
	if err != nil {
		suggestion.Status = SuggestionUnavailable
		return suggestion, nil
	}
	suggestion.Status, suggestion.Lemma = SuggestionOffered, result.Lemma
	suggestion.Provider, suggestion.Version = s.suggestions.Name(), s.suggestions.Version()
	return suggestion, nil
}

// Preview resolves a proposal against the current analysis and reports its
// consequences. It changes nothing.
func (s *Service) Preview(ctx context.Context, owner, bookID string, p Proposal) (Preview, error) {
	if err := s.rejectCurrentReading(ctx, owner, bookID, p.Language); err != nil {
		return Preview{}, err
	}
	proposal, err := s.resolve(ctx, owner, bookID, p)
	if err != nil {
		return Preview{}, err
	}
	impact, err := s.store.ReadLemmaReviewProposal(ctx, proposal)
	if err != nil {
		return Preview{}, mapStoreError(err)
	}
	identityChanged := changesIdentity(proposal)
	detail, recovery, err := s.load(ctx, owner, bookID)
	if err != nil {
		return Preview{}, err
	}
	action := decideReprepare(detail, recovery, identityChanged, false)
	return Preview{
		Action: p.Action, Lemma: proposal.Lemma, OccurrenceIDs: occurrenceIDs(proposal.Occurrences),
		Changes: changes(proposal), Impacts: selection.ReviewImpact(impact.States), Fingerprint: impact.Fingerprint,
		IdentityChanged: identityChanged, RequiresReprepare: action != domain.LemmaRepreparationNone,
	}, nil
}

// Confirm commits a previewed proposal if its state still matches fingerprint,
// then performs the follow-on steps the Book's ready deck needs. The decision
// commits in its own transaction; a failed follow-on step is an Outcome, not an
// error, because the decision is already safe.
func (s *Service) Confirm(ctx context.Context, owner, bookID string, p Proposal, fingerprint string, repreparationConfirmed bool) (Outcome, error) {
	if len(p.OccurrenceIDs) == 0 {
		return Outcome{}, domain.ErrLemmaReviewNoSelection
	}
	if err := s.rejectCurrentReading(ctx, owner, bookID, p.Language); err != nil {
		return Outcome{}, err
	}
	proposal, err := s.resolve(ctx, owner, bookID, p)
	if err != nil {
		return Outcome{}, err
	}
	detail, recovery, err := s.load(ctx, owner, bookID)
	if err != nil {
		return Outcome{}, err
	}
	action := decideReprepare(detail, recovery, changesIdentity(proposal), repreparationConfirmed)
	if err := action.Err(); err != nil {
		return Outcome{}, err
	}
	if err := s.store.PutLemmaDecisionProposal(ctx, proposal, fingerprint); err != nil {
		return Outcome{}, mapStoreError(err)
	}
	if s.browseCounts != nil {
		if err := s.browseCounts.EnqueueBrowseCountRebuild(ctx, owner, bookID); err != nil {
			// The committed decision is safe: startup reconciliation will enqueue
			// this missing projection if the immediate queue write is unavailable.
			log.Printf("mouseion: could not enqueue Browse count rebuild; startup reconciliation will retry")
		}
	}
	switch action {
	case domain.LemmaRepreparationRestart:
		return s.restart(ctx, owner, bookID, proposal.Language)
	case domain.LemmaRepreparationDirect:
		return s.prepare(func() (prepareddeck.Handle, error) {
			return s.preparer.Reprepare(ctx, owner, recovery.ReadyPreparationID)
		}), nil
	case domain.LemmaRepreparationNone, domain.LemmaRepreparationUnconfirmed, domain.LemmaRepreparationBlockedByReading, domain.LemmaRepreparationNotToRead:
		// Nothing to re-prepare. The rejected actions returned above.
	}
	return Outcome{Kind: OutcomeSaved}, nil
}

// restart starts the Book again under a new snapshot and prepares that snapshot.
// Flag detection is a write the new snapshot depends on, so it runs first.
func (s *Service) restart(ctx context.Context, owner, bookID, language string) (Outcome, error) {
	if _, err := s.Assess(ctx, owner, bookID); err != nil {
		return Outcome{}, err
	}
	reading, err := s.store.StartCurrentReading(ctx, owner, language, bookID)
	switch {
	case errors.Is(err, persistence.ErrUnresolvedLemmaReviewFlags):
		return Outcome{Kind: OutcomeFlagsUnresolved}, nil
	case err != nil:
		// The decision is already committed, so a failed restart is an outcome for
		// the learner to act on, not an error that would discard it.
		return Outcome{Kind: OutcomeRestartFailed}, nil //nolint:nilerr // The committed decision must not be reported as failed.
	}
	return s.prepare(func() (prepareddeck.Handle, error) {
		return s.preparer.SubmitForCurrentReading(ctx, owner, reading.AnalysisRunID, reading.SnapshotID)
	}), nil
}

func (s *Service) prepare(submit func() (prepareddeck.Handle, error)) Outcome {
	handle, err := submit()
	if err != nil {
		return Outcome{Kind: OutcomeQueueingFailed}
	}
	return Outcome{Kind: OutcomePreparing, PreparationID: handle.Preparation.ID}
}

// load reads the Book and the artifacts that decide what a vocabulary change
// does to it.
func (s *Service) load(ctx context.Context, owner, bookID string) (domain.MyBook, Recovery, error) {
	detail, err := s.store.GetBookDetail(ctx, owner, bookID)
	if err != nil {
		return domain.MyBook{}, Recovery{}, err
	}
	current, err := s.store.GetCurrentReading(ctx, owner, detail.Book.LanguageTag)
	if err != nil {
		return domain.MyBook{}, Recovery{}, err
	}
	recovery := Recovery{
		ActiveReading: current.IsActive() && current.BookID == bookID, ActiveReadingBookID: current.BookID,
		HasCompletedReading: detail.CompletionCount > 0,
	}
	if detail.Acquired == nil || s.preparer == nil {
		return detail, recovery, nil
	}
	preparations, err := s.store.ListDeckPreparationsForSourceMaterial(ctx, owner, detail.Acquired.Source.ID)
	if err != nil {
		return domain.MyBook{}, Recovery{}, err
	}
	for _, preparation := range preparations {
		if preparation.State == domain.DeckPreparationReady {
			recovery.ReadyPreparationID, recovery.ReadyDeckSnapshotID = preparation.ID, preparation.SnapshotID
			break
		}
	}
	return detail, recovery, nil
}

func decideReprepare(detail domain.MyBook, recovery Recovery, identityChanged, confirmed bool) domain.LemmaRepreparationAction {
	return domain.DecideLemmaReprepare(domain.LemmaRepreparationFacts{
		IdentityChanged: identityChanged, ReadyDeck: recovery.ReadyPreparationID != "", SnapshotBound: recovery.ReadyDeckSnapshotID != "",
		OtherCurrentReading: recovery.ActiveReadingBookID != "" && !recovery.ActiveReading,
		ToRead:              detail.Disposition == domain.BookDispositionToRead, Confirmed: confirmed,
	})
}

// rejectCurrentReading refuses vocabulary changes to the Book the learner is
// reading now. The store enforces the same rule under lock; this check gives
// the preview and the confirmation the same answer before any work starts.
func (s *Service) rejectCurrentReading(ctx context.Context, owner, bookID, language string) error {
	current, err := s.store.GetCurrentReading(ctx, owner, language)
	if err != nil {
		return err
	}
	if current.IsActive() && current.BookID == bookID {
		return domain.ErrLemmaReviewCurrentReading
	}
	return nil
}

// resolve turns a requested proposal into the domain proposal the store
// applies, selecting exactly the occurrences whose IDs were named.
func (s *Service) resolve(ctx context.Context, owner, bookID string, p Proposal) (domain.LemmaReviewProposal, error) {
	profile, err := canonicalization.For(p.Language)
	if err != nil {
		return domain.LemmaReviewProposal{}, domain.ErrLemmaReviewUnsupportedLanguage
	}
	lemma := ""
	switch p.Action {
	case "correct":
		lemma = profile.Canonical(strings.TrimSpace(p.Lemma))
		if !lexical.IsLemma(lemma) {
			return domain.LemmaReviewProposal{}, domain.ErrLemmaReviewLemma
		}
	case "keep", "exclude":
	default:
		return domain.LemmaReviewProposal{}, domain.ErrLemmaReviewDecision
	}
	occurrences, err := s.store.ListLemmaReviewOccurrences(ctx, owner, bookID, p.Form)
	if err != nil {
		return domain.LemmaReviewProposal{}, err
	}
	wanted := make(map[string]bool, len(p.OccurrenceIDs))
	for _, id := range p.OccurrenceIDs {
		wanted[id] = true
	}
	chosen := make([]domain.LemmaReviewOccurrence, 0, len(wanted))
	for _, occurrence := range occurrences {
		if wanted[occurrence.ID()] {
			chosen = append(chosen, occurrence)
		}
	}
	if len(wanted) == 0 || len(chosen) != len(wanted) {
		return domain.LemmaReviewProposal{}, domain.ErrLemmaReviewOccurrenceUnavailable
	}
	return domain.LemmaReviewProposal{
		OwnerID: owner, BookID: bookID, Language: p.Language, Surface: p.Form,
		Action: p.Action, Lemma: lemma, Occurrences: chosen,
		NormalizationProfile: profile.Name(), NormalizationVersion: profile.Version(),
	}, nil
}

// EvidenceOf is the local-reference provenance a flag recorded on an occurrence,
// empty when the occurrence is unflagged.
func EvidenceOf(occurrence domain.LemmaReviewOccurrence) Evidence {
	value := func(key string) string {
		text, ok := occurrence.ReviewFlagProvenance[key].(string)
		if !ok {
			return ""
		}
		return text
	}
	return Evidence{Alternative: value("alternative_lemma"), Source: value("source"), Version: value("version"), EvidenceID: value("evidence_id")}
}

// Evidence is the provenance a lemma risk flag stored with its alternative.
type Evidence struct{ Alternative, Source, Version, EvidenceID string }

func changesIdentity(proposal domain.LemmaReviewProposal) bool {
	for _, decision := range proposal.Decisions() {
		if selection.ReviewIdentityNow(proposal.Language, decision.Occurrence) != selection.ReviewIdentityAfter(proposal.Language, decision.Occurrence, decision) {
			return true
		}
	}
	return false
}

// changes shows each selected occurrence's identity before and after. An
// excluded side shows as "excluded".
func changes(proposal domain.LemmaReviewProposal) []Change {
	result := make([]Change, 0, len(proposal.Occurrences))
	for _, decision := range proposal.Decisions() {
		before := selection.ReviewIdentityNow(proposal.Language, decision.Occurrence).CanonicalLemma
		if before == "" {
			before = "excluded"
		}
		after := selection.ReviewIdentityAfter(proposal.Language, decision.Occurrence, decision).CanonicalLemma
		if after == "" {
			after = "excluded"
		}
		result = append(result, Change{Sentence: decision.Occurrence.SentenceText, Before: before, After: after})
	}
	return result
}

// mapStoreError translates the store's rejections into the workflow's
// sentinels. Any other error is an infrastructure failure and passes through.
func mapStoreError(err error) error {
	switch {
	case errors.Is(err, persistence.ErrLemmaDecisionCurrentReading):
		return domain.ErrLemmaReviewCurrentReading
	case errors.Is(err, persistence.ErrLemmaReviewPreviewStale), errors.Is(err, persistence.ErrNotFound):
		return domain.ErrLemmaReviewStale
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsPending), errors.Is(err, persistence.ErrVocabularyBrowseCountsUnavailable):
		return domain.ErrLemmaReviewCountsRefreshing
	}
	return err
}

func findOccurrence(occurrences []domain.LemmaReviewOccurrence, id string) (domain.LemmaReviewOccurrence, bool) {
	for _, occurrence := range occurrences {
		if occurrence.ID() == id {
			return occurrence, true
		}
	}
	return domain.LemmaReviewOccurrence{}, false
}

func occurrenceIDs(occurrences []domain.LemmaReviewOccurrence) []string {
	ids := make([]string, 0, len(occurrences))
	for _, occurrence := range occurrences {
		ids = append(ids, occurrence.ID())
	}
	return ids
}

func hasDecision(occurrences []domain.LemmaReviewOccurrence) bool {
	for _, occurrence := range occurrences {
		if occurrence.Excluded || occurrence.CorrectedLemma != "" {
			return true
		}
	}
	return false
}

func anyUnresolved(occurrences []domain.LemmaReviewOccurrence) bool {
	for _, occurrence := range occurrences {
		if occurrence.ReviewFlagReason != "" && occurrence.ReviewFlagResolution == "" {
			return true
		}
	}
	return false
}
