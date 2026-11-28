// Package lemmareview owns the Book-scoped lemma decision workflow: it lists
// exact occurrences, and previews and confirms keep,
// correction, or exclusion decisions. Handlers render its results and map its
// sentinel errors to responses; nothing here knows about HTTP.
package lemmareview

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
)

// Store is the narrow persistence port the workflow needs. The Postgres store
// and the fixture store both satisfy it.
type Store interface {
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
	GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error)
	StartCurrentReading(context.Context, string, string, string) (domain.CurrentReading, error)
	ListLemmaReviewOccurrences(context.Context, string, string, string) ([]domain.LemmaReviewOccurrence, error)
	ListCorpusLemmaReviewOccurrences(ctx context.Context, owner, language, surface, lemma string) ([]domain.LemmaReviewOccurrence, error)
	ReadLemmaReviewProposal(context.Context, domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error)
	PutLemmaDecisionProposal(context.Context, domain.LemmaReviewProposal, string) error
}

// BrowseCounts schedules the vocabulary count rebuild a committed decision needs.
type BrowseCounts interface {
	EnqueueBrowseCountRebuild(context.Context, string, string) error
}

// Config supplies a Service's collaborators. BrowseCounts may be nil; the
// workflow then runs without the count rebuild.
type Config struct {
	Store        Store
	BrowseCounts BrowseCounts
}

// Service is the lemma review workflow.
type Service struct {
	store        Store
	browseCounts BrowseCounts
}

func New(cfg Config) *Service {
	return &Service{store: cfg.Store, browseCounts: cfg.BrowseCounts}
}

// Recovery is what a reviewer needs to know about a Book's existing reading
// before changing its vocabulary identities.
type Recovery struct {
	ActiveReading       bool
	ActiveReadingBookID string
	HasCompletedReading bool
	HasIdentityDecision bool
}

// Review is the occurrence list for one form, with whether correction is allowed.
type Review struct {
	Occurrences []domain.LemmaReviewOccurrence
	CanCorrect  bool
	Recovery    Recovery
}

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
	Action          string
	Lemma           string
	OccurrenceIDs   []string
	Changes         []Change
	Impacts         []selection.Impact
	Fingerprint     string
	IdentityChanged bool
}

// Review lists the occurrences of one exact form and what the learner may do
// with them.
func (s *Service) Review(ctx context.Context, owner, bookID, form string) (Review, error) {
	_, recovery, err := s.load(ctx, owner, bookID)
	if err != nil {
		return Review{}, err
	}
	review := Review{CanCorrect: !recovery.ActiveReading, Recovery: recovery}
	if form == "" {
		return review, nil
	}
	review.Occurrences, err = s.store.ListLemmaReviewOccurrences(ctx, owner, bookID, form)
	if err != nil {
		return Review{}, err
	}
	review.Recovery.HasIdentityDecision = hasDecision(review.Occurrences)
	return review, nil
}

// CorpusMatch is another occurrence in the learner's Corpus of the same observed
// form and analyzer lemma as a reported occurrence, with what the learner may do
// with its Book.
type CorpusMatch struct {
	Occurrence domain.LemmaReviewOccurrence
	BookTitle  string
	// CanCorrect is false while the occurrence's Book is the Current reading,
	// whose vocabulary cannot change.
	CanCorrect bool
}

// CorpusMatches lists the occurrences in the Corpus, in any Book, that share
// the reported occurrence's observed form and analyzer lemma and still await a
// decision. The reported occurrence itself is left out. Nothing is decided: each
// match is previewed and confirmed on its own, in its own Book's analysis, so
// no decision becomes a rule over a form and lemma (ADR 0136 §6).
func (s *Service) CorpusMatches(ctx context.Context, owner, language string, reported domain.LemmaReviewOccurrence) ([]CorpusMatch, error) {
	found, err := s.store.ListCorpusLemmaReviewOccurrences(ctx, owner, language, reported.Surface, reported.RawLemma)
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	editable := map[string]bool{}
	var matches []CorpusMatch
	for _, occurrence := range found {
		if occurrence.ID() == reported.ID() || occurrence.Excluded || occurrence.CorrectedLemma != "" {
			continue
		}
		if _, known := titles[occurrence.BookID]; !known {
			detail, recovery, err := s.load(ctx, owner, occurrence.BookID)
			if err != nil {
				return nil, err
			}
			titles[occurrence.BookID], editable[occurrence.BookID] = detail.Book.Title, !recovery.ActiveReading
		}
		matches = append(matches, CorpusMatch{Occurrence: occurrence, BookTitle: titles[occurrence.BookID], CanCorrect: editable[occurrence.BookID]})
	}
	return matches, nil
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
	return Preview{
		Action: p.Action, Lemma: proposal.Lemma, OccurrenceIDs: occurrenceIDs(proposal.Occurrences),
		Changes: changes(proposal), Impacts: selection.ReviewImpact(impact.States), Fingerprint: impact.Fingerprint,
		IdentityChanged: identityChanged,
	}, nil
}

// Confirm commits a previewed proposal if its state still matches fingerprint,
// then queues the Browse count rebuild it needs. The decision commits in its
// own transaction; a failed follow-on step is logged, not an error, because the
// decision is already safe.
func (s *Service) Confirm(ctx context.Context, owner, bookID string, p Proposal, fingerprint string) error {
	if len(p.OccurrenceIDs) == 0 {
		return domain.ErrLemmaReviewNoSelection
	}
	if err := s.rejectCurrentReading(ctx, owner, bookID, p.Language); err != nil {
		return err
	}
	proposal, err := s.resolve(ctx, owner, bookID, p)
	if err != nil {
		return err
	}
	if err := s.store.PutLemmaDecisionProposal(ctx, proposal, fingerprint); err != nil {
		return mapStoreError(err)
	}
	if s.browseCounts != nil {
		if err := s.browseCounts.EnqueueBrowseCountRebuild(ctx, owner, bookID); err != nil {
			// The committed decision is safe: startup reconciliation will enqueue
			// this missing projection if the immediate queue write is unavailable.
			log.Printf("mouseion: could not enqueue Browse count rebuild; startup reconciliation will retry")
		}
	}
	return nil
}

// load reads the Book and the reading state that decides whether a vocabulary
// change is allowed.
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
	return detail, recovery, nil
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
	switch p.Action {
	case "correct", "keep", "exclude":
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
	profile, err := normalizationProfile(p.Language, chosen)
	if err != nil {
		return domain.LemmaReviewProposal{}, domain.ErrLemmaReviewUnsupportedLanguage
	}
	lemma := ""
	if p.Action == "correct" {
		lemma = profile.Canonical(strings.TrimSpace(p.Lemma))
		if !lexical.IsAdmissibleLemma(p.Language, lemma) {
			return domain.LemmaReviewProposal{}, domain.ErrLemmaReviewLemma
		}
	}
	return domain.LemmaReviewProposal{
		OwnerID: owner, BookID: bookID, Language: p.Language, Surface: p.Form,
		Action: p.Action, Lemma: lemma, Occurrences: chosen,
		NormalizationProfile: profile.Name(), NormalizationVersion: profile.Version(),
	}, nil
}

// normalizationProfile resolves the profile a correction is normalized with. A
// dedicated-profile language (Ancient Greek) is normalized by exactly the
// profile its analysis recorded, looked up by version: a later active profile
// never rewrites a decision on an earlier analysis, and an analysis that
// recorded no resolvable profile is unsupported rather than quietly casefolded.
// Every other language uses its active profile, so modern lemma review is
// unchanged.
func normalizationProfile(language string, occurrences []domain.LemmaReviewOccurrence) (canonicalization.Profile, error) {
	if canonicalization.DedicatedProfileLanguage(language) {
		for _, occurrence := range occurrences {
			if occurrence.NormalizationProfile == "" || occurrence.NormalizationVersion == "" {
				continue
			}
			profile, err := canonicalization.Lookup(language, occurrence.NormalizationVersion)
			if err == nil && profile.Name() == occurrence.NormalizationProfile {
				return profile, nil
			}
		}
		return nil, canonicalization.ErrProfileNotFound
	}
	return canonicalization.For(language)
}

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
	case errors.Is(err, domain.ErrLemmaDecisionCurrentReading):
		return domain.ErrLemmaReviewCurrentReading
	case errors.Is(err, domain.ErrLemmaReviewPreviewStale), errors.Is(err, persistence.ErrNotFound):
		return domain.ErrLemmaReviewStale
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsPending), errors.Is(err, persistence.ErrVocabularyBrowseCountsUnavailable):
		return domain.ErrLemmaReviewCountsRefreshing
	}
	return err
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
