package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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

type lemmaDecisionProposal struct {
	Action      string
	Lemma       string
	Occurrences []domain.LemmaReviewOccurrence
	Indices     []int
	Fingerprint string
	Changes     []lemmaOccurrenceChange
	Impacts     []selection.Impact
}

type lemmaOccurrenceChange struct{ Sentence, Before, After string }

type lemmaReviewEvidence struct{ Alternative, Source, Version, EvidenceID string }

type lemmaReviewSuggestion struct {
	Lemma, Provider, Version, Message string
	Target                            int
}

func evidenceForLemmaReview(occurrence domain.LemmaReviewOccurrence) lemmaReviewEvidence {
	value := func(key string) string {
		if occurrence.ReviewFlagProvenance == nil {
			return ""
		}
		text, ok := occurrence.ReviewFlagProvenance[key].(string)
		if !ok {
			return ""
		}
		return text
	}
	return lemmaReviewEvidence{Alternative: value("alternative_lemma"), Source: value("source"), Version: value("version"), EvidenceID: value("evidence_id")}
}

type lemmaReviewRecovery struct {
	ActiveReading       bool
	ActiveReadingBookID string
	ReadyPreparationID  string
	ReadyDeckSnapshotID string
	HasCompletedReading bool
	HasIdentityDecision bool
	ReferenceAssessed   bool
}

func (h *Handler) ensureLemmaReviewFlags(ctx context.Context, owner string, detail domain.MyBook) (bool, error) {
	if h.services.LemmaRiskIndex == nil || detail.Acquired == nil || detail.Book.LanguageTag != "de" {
		return false, nil
	}
	occurrences, err := h.services.Store.Reading.ListLemmaReviewOccurrences(ctx, owner, detail.Book.ID, "")
	if err != nil {
		return false, err
	}
	input := make([]lemmarisk.Occurrence, 0, len(occurrences))
	byID := make(map[string]domain.LemmaReviewOccurrence, len(occurrences))
	for _, occurrence := range occurrences {
		id := lemmaReviewOccurrenceID(occurrence)
		lemma := occurrence.CanonicalLemma
		if occurrence.CorrectedLemma != "" {
			lemma = occurrence.CorrectedLemma
		}
		input = append(input, lemmarisk.Occurrence{ID: id, Language: detail.Book.LanguageTag, Surface: occurrence.Surface, Lemma: lemma, UPOS: occurrence.UPOS, Sentence: occurrence.SentenceText, Reviewed: occurrence.CorrectedLemma != "" || occurrence.ReviewFlagResolution != "", Excluded: occurrence.Excluded})
		byID[id] = occurrence
	}
	assessmentCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	flags, assessed, err := lemmarisk.Detect(assessmentCtx, detail.Book.LanguageTag, input, h.services.LemmaRiskIndex)
	if err != nil {
		return false, nil
	} // A failed optional local index is not a clean assessment or a Reading blocker.
	if !assessed {
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
	if err := h.services.Store.Reading.SaveLemmaReviewFlags(ctx, persisted); err != nil {
		return false, err
	}
	return true, nil
}

func lemmaReviewOccurrenceID(occurrence domain.LemmaReviewOccurrence) string {
	return occurrence.AnalysisRunID + ":" + occurrence.SourceDocumentID + ":" + strconv.FormatInt(occurrence.StartOffset, 10) + ":" + strconv.FormatInt(occurrence.EndOffset, 10)
}

func (h *Handler) lemmaReview(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	store := h.services.Store.Reading
	if store == nil {
		http.Error(w, "Occurrence review is unavailable.", http.StatusServiceUnavailable)
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	assessed, err := h.ensureLemmaReviewFlags(r.Context(), owner.ID, detail)
	if err != nil {
		fail(w, err)
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" || detail.Book.LanguageTag != language {
		http.Error(w, "Choose this Book's study language in Reading before reviewing occurrences.", http.StatusConflict)
		return
	}
	canCorrect, pageError, err := h.lemmaReviewCorrectionAvailability(r.Context(), owner.ID, bookID, language)
	if err != nil {
		fail(w, err)
		return
	}
	recovery, err := h.lemmaReviewRecovery(r, owner, bookID, detail)
	if err != nil {
		fail(w, err)
		return
	}
	recovery.ReferenceAssessed = assessed
	form := strings.TrimSpace(r.URL.Query().Get("form"))
	var occurrences []domain.LemmaReviewOccurrence
	if form != "" {
		occurrences, err = store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
		if err != nil {
			fail(w, err)
			return
		}
		recovery.HasIdentityDecision = lemmaOccurrencesHaveDecision(occurrences)
	} else {
		allOccurrences, listErr := store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, "")
		if listErr != nil {
			fail(w, listErr)
			return
		}
		for _, occurrence := range allOccurrences {
			if occurrence.ReviewFlagReason != "" {
				occurrences = append(occurrences, occurrence)
			}
		}
	}
	var proposal *lemmaDecisionProposal
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, pageError, canCorrect, occurrences, proposal, recovery, nil))
}

func (h *Handler) suggestLemma(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	form := strings.TrimSpace(r.FormValue("form"))
	index, err := strconv.Atoi(r.FormValue("target"))
	if err != nil || index < 0 || form == "" {
		http.Error(w, "Choose an occurrence to request a suggestion.", http.StatusBadRequest)
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" || language != detail.Book.LanguageTag {
		http.Error(w, "Choose this Book's study language in Reading before requesting a suggestion.", http.StatusConflict)
		return
	}
	occurrences, err := h.services.Store.Reading.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	if index >= len(occurrences) {
		http.Error(w, "That occurrence is no longer available in this analysis.", http.StatusConflict)
		return
	}
	result := &lemmaReviewSuggestion{Target: index}
	provider := h.services.LemmaSuggestions
	if provider == nil {
		result.Message = "LLM suggestions are not configured. You can still keep, correct, or exclude this occurrence manually."
	} else {
		occurrence := occurrences[index]
		evidence := evidenceForLemmaReview(occurrence)
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		suggestion, suggestErr := provider.SuggestLemma(ctx, enrichment.LemmaSuggestionRequest{
			Language: detail.Book.LanguageTag, Surface: occurrence.Surface, AnalyzedLemma: occurrence.CanonicalLemma,
			UPOS: occurrence.UPOS, Sentence: occurrence.SentenceText, LexicalAlternative: evidence.Alternative,
			LexicalSource: evidence.Source, LexicalVersion: evidence.Version, LexicalEvidenceID: evidence.EvidenceID,
		})
		cancel()
		if suggestErr != nil {
			result.Message = "A lemma suggestion is unavailable right now. You can still keep, correct, or exclude this occurrence manually."
		} else {
			result.Lemma, result.Provider, result.Version = suggestion.Lemma, provider.Name(), provider.Version()
		}
	}
	recovery, err := h.lemmaReviewRecovery(r, owner, bookID, detail)
	if err != nil {
		fail(w, err)
		return
	}
	canCorrect, pageError, err := h.lemmaReviewCorrectionAvailability(r.Context(), owner.ID, bookID, detail.Book.LanguageTag)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, pageError, canCorrect, occurrences, nil, recovery, result))
}

func (h *Handler) lemmaReviewCorrectionAvailability(ctx context.Context, ownerID, bookID, language string) (bool, string, error) {
	current, err := h.services.Store.Reading.GetCurrentReading(ctx, ownerID, language)
	if err != nil {
		return false, "", err
	}
	if currentReadingBlocksLemmaDecision(current, bookID) {
		return false, "This Book is current reading. End current reading before changing its vocabulary; the frozen reading snapshot remains unchanged.", nil
	}
	return true, "", nil
}

func currentReadingBlocksLemmaDecision(current domain.CurrentReading, bookID string) bool {
	return current.IsActive() && current.BookID == bookID
}

func (h *Handler) correctLemma(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	store := h.services.Store.Reading
	if store == nil {
		http.Error(w, "Occurrence review is unavailable.", http.StatusServiceUnavailable)
		return
	}
	if r.FormValue("stage") == "preview" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid review form.", http.StatusBadRequest)
			return
		}
		detail, detailErr := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
		if detailErr != nil {
			fail(w, detailErr)
			return
		}
		assessed, detectErr := h.ensureLemmaReviewFlags(r.Context(), owner.ID, detail)
		if detectErr != nil {
			fail(w, detectErr)
			return
		}
		form := strings.TrimSpace(r.FormValue("form"))
		if !h.lemmaReviewWritable(w, r, owner, bookID) {
			return
		}
		occurrences, err := store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
		if err != nil {
			fail(w, err)
			return
		}
		proposal, err := h.lemmaProposal(r, occurrences)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		page, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		recovery, recoveryErr := h.lemmaReviewRecovery(r, owner, bookID, page)
		if recoveryErr != nil {
			fail(w, recoveryErr)
			return
		}
		recovery.HasIdentityDecision = lemmaOccurrencesHaveDecision(occurrences)
		recovery.ReferenceAssessed = assessed
		render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, page.Book.Title, form, "", true, occurrences, proposal, recovery, nil))
		return
	}
	if r.FormValue("stage") == "confirm" {
		if !h.lemmaReviewWritable(w, r, owner, bookID) {
			return
		}
		h.confirmLemmaProposal(w, r, owner)
		return
	}
	http.Error(w, "Review the proposed consequence before confirming a decision.", http.StatusBadRequest)
}

func lemmaOccurrencesHaveDecision(occurrences []domain.LemmaReviewOccurrence) bool {
	for _, occurrence := range occurrences {
		if occurrence.Excluded || occurrence.CorrectedLemma != "" {
			return true
		}
	}
	return false
}

func lemmaProposalChangesIdentity(proposal *lemmaDecisionProposal) bool {
	if proposal == nil {
		return false
	}
	for _, occurrence := range proposal.Occurrences {
		if lemmaDecisionChangesIdentity(occurrence, proposal.Action, proposal.Lemma) {
			return true
		}
	}
	return false
}

func lemmaDecisionChangesIdentity(occurrence domain.LemmaReviewOccurrence, action, lemma string) bool {
	before, after := lemmaDecisionIdentities(occurrence, action, lemma)
	return before != after
}

func (h *Handler) lemmaReviewRecovery(r *http.Request, owner domain.User, bookID string, detail domain.MyBook) (lemmaReviewRecovery, error) {
	language := detail.Book.LanguageTag
	current, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner.ID, language)
	if err != nil {
		return lemmaReviewRecovery{}, err
	}
	recovery := lemmaReviewRecovery{ActiveReading: current.IsActive() && current.BookID == bookID, ActiveReadingBookID: current.BookID, HasCompletedReading: detail.CompletionCount > 0}
	if detail.Acquired == nil || h.services.PreparedDeck == nil {
		return recovery, nil
	}
	preparations, err := h.services.Store.Reading.ListDeckPreparationsForSourceMaterial(r.Context(), owner.ID, detail.Acquired.Source.ID)
	if err != nil {
		return lemmaReviewRecovery{}, err
	}
	for _, preparation := range preparations {
		if preparation.State == domain.DeckPreparationReady {
			recovery.ReadyPreparationID = preparation.ID
			recovery.ReadyDeckSnapshotID = preparation.SnapshotID
			break
		}
	}
	return recovery, nil
}

func (h *Handler) lemmaProposal(r *http.Request, matches []domain.LemmaReviewOccurrence) (*lemmaDecisionProposal, error) {
	index, err := strconv.Atoi(r.FormValue("target"))
	if err != nil || index < 0 || index >= len(matches) {
		return nil, errors.New("The selected occurrence changed. Find it again before reviewing.")
	}
	selected := map[int]bool{index: true}
	for _, raw := range r.Form["also"] {
		i, parseErr := strconv.Atoi(raw)
		if parseErr != nil || i < 0 || i >= len(matches) {
			return nil, errors.New("One selected occurrence is no longer available. Find the exact form again.")
		}
		selected[i] = true
	}
	chosen := make([]domain.LemmaReviewOccurrence, 0, len(selected))
	for i := range matches {
		if selected[i] {
			chosen = append(chosen, matches[i])
		}
	}
	action := r.FormValue("decision")
	lemma := ""
	if action == "correct" {
		language, _ := activeStudyLanguageForContext(r.Context())
		profile, profileErr := canonicalization.For(language)
		if profileErr != nil {
			return nil, errors.New("Choose a supported study language before correcting a lemma.")
		}
		lemma, err = normalizedLemma(profile, r.FormValue("lemma"))
		if err != nil {
			return nil, err
		}
	} else if action != "keep" && action != "exclude" {
		return nil, errors.New("Choose a decision to preview.")
	}
	indices := make([]int, 0, len(selected))
	for i := range matches {
		if selected[i] {
			indices = append(indices, i)
		}
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	proposalIdentities := lemmaProposalIdentities(chosen, action, lemma, language)
	fingerprint, err := h.services.Store.Reading.LemmaReviewStateFingerprint(r.Context(), user(r).ID, strings.TrimSpace(r.PathValue("bookID")), language, strings.TrimSpace(r.FormValue("form")), proposalIdentities)
	if lemmaCountsRefreshing(err) {
		return nil, errors.New("Vocabulary counts are being refreshed. Preview this decision again shortly.")
	}
	if err != nil {
		return nil, err
	}
	p := &lemmaDecisionProposal{Action: action, Lemma: lemma, Occurrences: chosen, Indices: indices, Fingerprint: lemmaProposalFingerprint(fingerprint, action, lemma, indices)}
	p.Changes, p.Impacts, err = h.lemmaProposalImpacts(r, user(r).ID, strings.TrimSpace(r.PathValue("bookID")), language, chosen, action, lemma)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (h *Handler) lemmaProposalImpacts(r *http.Request, ownerID, bookID, language string, chosen []domain.LemmaReviewOccurrence, action, lemma string) ([]lemmaOccurrenceChange, []selection.Impact, error) {
	changes := make([]lemmaOccurrenceChange, 0, len(chosen))
	decisions := make([]domain.LemmaReviewDecision, 0, len(chosen))
	for _, occurrence := range chosen {
		before, after := lemmaDecisionIdentities(occurrence, action, lemma)
		beforeDisplay := before
		if occurrence.Excluded {
			beforeDisplay = "excluded"
		}
		afterDisplay := after
		if afterDisplay == "" {
			afterDisplay = "excluded"
		}
		changes = append(changes, lemmaOccurrenceChange{Sentence: occurrence.SentenceText, Before: beforeDisplay, After: afterDisplay})
		decisions = append(decisions, domain.LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: after, Excluded: action == "exclude"})
	}
	counts, err := h.services.Store.Reading.PreviewLemmaDecisionCounts(r.Context(), ownerID, bookID, decisions)
	if lemmaCountsRefreshing(err) {
		return nil, nil, errors.New("Vocabulary counts are being refreshed. Preview this decision again shortly.")
	}
	if err != nil {
		return nil, nil, err
	}
	known, err := h.services.Store.Reading.ListKnownVocabulary(r.Context(), ownerID, language)
	if err != nil {
		return nil, nil, err
	}
	reserved := make([]domain.DeckPreparationVocabulary, 0, len(counts))
	impactCounts := make([]selection.ImpactCounts, 0, len(counts))
	for _, c := range counts {
		isReserved, reserveErr := h.services.Store.Reading.IsReservedVocabulary(r.Context(), ownerID, c.Language, c.CanonicalLemma, c.UPOS)
		if reserveErr != nil {
			return nil, nil, reserveErr
		}
		if isReserved {
			reserved = append(reserved, domain.DeckPreparationVocabulary{Language: c.Language, CanonicalLemma: c.CanonicalLemma, UPOS: c.UPOS})
		}
		impactCounts = append(impactCounts, selection.ImpactCounts{
			Identity: selection.Identity{Language: c.Language, CanonicalLemma: c.CanonicalLemma, UPOS: c.UPOS},
			Before:   c.Before, After: c.After, OtherBooks: c.OtherBooks,
		})
	}
	return changes, selection.NewEligibility(known, reserved).ReviewImpact(impactCounts), nil
}

func lemmaCountsRefreshing(err error) bool {
	return errors.Is(err, persistence.ErrVocabularyBrowseCountsPending) || errors.Is(err, persistence.ErrVocabularyBrowseCountsUnavailable)
}

func lemmaProposalIdentities(occurrences []domain.LemmaReviewOccurrence, action, lemma, language string) []domain.LemmaReviewIdentity {
	identities := make(map[domain.LemmaReviewIdentity]bool)
	for _, occurrence := range occurrences {
		before, after := lemmaDecisionIdentities(occurrence, action, lemma)
		if occurrence.Excluded {
			before = ""
		}
		if before != "" {
			identities[domain.LemmaReviewIdentity{Language: language, CanonicalLemma: before, UPOS: occurrence.UPOS}] = true
		}
		if after != "" {
			identities[domain.LemmaReviewIdentity{Language: language, CanonicalLemma: after, UPOS: occurrence.UPOS}] = true
		}
	}
	result := make([]domain.LemmaReviewIdentity, 0, len(identities))
	for identity := range identities {
		result = append(result, identity)
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].Language + "\x00" + result[i].CanonicalLemma + "\x00" + result[i].UPOS
		right := result[j].Language + "\x00" + result[j].CanonicalLemma + "\x00" + result[j].UPOS
		return left < right
	})
	return result
}

func lemmaDecisionIdentities(occurrence domain.LemmaReviewOccurrence, action, lemma string) (before, after string) {
	before = occurrence.CanonicalLemma
	if occurrence.CorrectedLemma != "" {
		before = occurrence.CorrectedLemma
	}
	if occurrence.Excluded {
		before = ""
	}
	after = lemma
	if action == "keep" {
		after = occurrence.CanonicalLemma
	}
	if action == "exclude" {
		after = ""
	}
	return before, after
}

func lemmaProposalFingerprint(state, action, lemma string, selected []int) string {
	values := make([]string, len(selected))
	for i, value := range selected {
		values[i] = strconv.Itoa(value)
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(state + "\x00" + action + "\x00" + lemma + "\x00" + strings.Join(values, ",")))
	return hex.EncodeToString(digest[:])
}

func (h *Handler) lemmaReviewWritable(w http.ResponseWriter, r *http.Request, owner domain.User, bookID string) bool {
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return false
	}
	if err != nil {
		fail(w, err)
		return false
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" || detail.Book.LanguageTag != language {
		http.Error(w, "Choose this Book's study language in Reading before reviewing occurrences.", http.StatusConflict)
		return false
	}
	current, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return false
	}
	if currentReadingBlocksLemmaDecision(current, bookID) {
		http.Error(w, persistence.ErrLemmaDecisionCurrentReading.Error(), http.StatusConflict)
		return false
	}
	return true
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func normalizedLemma(profile canonicalization.Profile, value string) (string, error) {
	lemma := profile.Canonical(strings.TrimSpace(value))
	if !lexical.IsLemma(lemma) {
		return "", errors.New("Enter one valid canonical lemma without spaces.")
	}
	return lemma, nil
}

func (h *Handler) confirmLemmaProposal(w http.ResponseWriter, r *http.Request, owner domain.User) {
	bookID, form := strings.TrimSpace(r.PathValue("bookID")), strings.TrimSpace(r.FormValue("form"))
	store := h.services.Store.Reading
	matches, err := store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	selected := r.Form["selected"]
	if len(selected) == 0 {
		http.Error(w, "The preview contains no selected occurrences.", http.StatusBadRequest)
		return
	}
	action, lemma := r.FormValue("decision"), strings.TrimSpace(r.FormValue("lemma"))
	language, _ := activeStudyLanguageForContext(r.Context())
	profile, profileErr := canonicalization.For(language)
	if profileErr != nil {
		http.Error(w, "Choose a supported study language before confirming.", http.StatusConflict)
		return
	}
	if action == "correct" {
		lemma, profileErr = normalizedLemma(profile, lemma)
		if profileErr != nil {
			http.Error(w, profileErr.Error(), http.StatusBadRequest)
			return
		}
	}
	if action != "correct" && action != "keep" && action != "exclude" {
		http.Error(w, "Invalid decision.", http.StatusBadRequest)
		return
	}
	indices := make(map[int]bool, len(selected))
	orderedIndices := make([]int, 0, len(selected))
	for _, raw := range selected {
		i, parseErr := strconv.Atoi(raw)
		if parseErr != nil || i < 0 || i >= len(matches) {
			http.Error(w, "The preview selection is invalid.", http.StatusBadRequest)
			return
		}
		indices[i] = true
		orderedIndices = append(orderedIndices, i)
	}
	decisions := make([]domain.LemmaReviewDecision, 0, len(indices))
	chosen := make([]domain.LemmaReviewOccurrence, 0, len(indices))
	identityChanged := false
	for i, occurrence := range matches {
		if !indices[i] {
			continue
		}
		decision := lemma
		excluded := action == "exclude"
		if action == "keep" {
			decision = occurrence.CanonicalLemma
		}
		identityChanged = identityChanged || lemmaDecisionChangesIdentity(occurrence, action, lemma)
		chosen = append(chosen, occurrence)
		decisions = append(decisions, domain.LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: decision, Excluded: excluded, NormalizationProfile: profile.Name(), NormalizationVersion: profile.Version()})
	}
	extras := lemmaProposalIdentities(chosen, action, lemma, language)
	stateFingerprint, err := store.LemmaReviewStateFingerprint(r.Context(), owner.ID, bookID, language, form, extras)
	if lemmaCountsRefreshing(err) {
		http.Error(w, "Vocabulary counts are being refreshed. No decision was saved; review it again shortly.", http.StatusConflict)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if lemmaProposalFingerprint(stateFingerprint, action, lemma, orderedIndices) != r.FormValue("fingerprint") {
		http.Error(w, "Learner vocabulary state or this proposal changed after preview. No decision was saved; review it again.", http.StatusConflict)
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	recovery, err := h.lemmaReviewRecovery(r, owner, bookID, detail)
	if err != nil {
		fail(w, err)
		return
	}
	requiresReprepare := identityChanged && recovery.ReadyPreparationID != ""
	if requiresReprepare && r.FormValue("reprepare_ready_deck") != "yes" {
		http.Error(w, "Explicitly confirm re-preparation of the existing ready deck before accepting this identity change.", http.StatusBadRequest)
		return
	}
	if requiresReprepare && recovery.ReadyDeckSnapshotID != "" && recovery.ActiveReadingBookID != "" {
		http.Error(w, "Stop the other current reading before accepting this change; then Mouseion can start this Book again and prepare its new snapshot.", http.StatusConflict)
		return
	}
	if requiresReprepare && recovery.ReadyDeckSnapshotID != "" && detail.Disposition != domain.BookDispositionToRead {
		http.Error(w, "Move this Book to To Read before accepting this change, so Mouseion can restart it and prepare the new snapshot.", http.StatusConflict)
		return
	}
	if err := store.PutLemmaDecisionProposal(r.Context(), decisions, form, language, extras, stateFingerprint); err != nil {
		if errors.Is(err, persistence.ErrLemmaDecisionCurrentReading) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, persistence.ErrNotFound) || lemmaCountsRefreshing(err) {
			http.Error(w, "Learner vocabulary state changed or froze after preview. No decision was saved; review it again.", http.StatusConflict)
			return
		}
		fail(w, err)
		return
	}
	if err := h.services.Analysis.EnqueueBrowseCountRebuild(r.Context(), owner.ID, bookID); err != nil {
		// The committed decision is safe: startup reconciliation will enqueue
		// this missing projection if the immediate queue write is unavailable.
		log.Printf("mouseion: could not enqueue Browse count rebuild; startup reconciliation will retry")
	}
	if requiresReprepare {
		var handle prepareddeck.Handle
		var reprepareErr error
		if recovery.ReadyDeckSnapshotID != "" {
			// Flag detection is a write the new snapshot depends on, so it runs
			// before the restart freezes it, as Start and Switch do.
			if _, detectErr := h.ensureLemmaReviewFlags(r.Context(), owner.ID, detail); detectErr != nil {
				fail(w, detectErr)
				return
			}
			reading, startErr := h.services.Store.Reading.StartCurrentReading(r.Context(), owner.ID, language, bookID)
			if errors.Is(startErr, persistence.ErrUnresolvedLemmaReviewFlags) {
				redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review?form="+url.QueryEscape(form))
				return
			}
			if startErr != nil {
				http.Error(w, "The identity decision was saved, but a new Reading snapshot could not be started. Start this Book in Reading, then prepare its deck; the historical deck remains available.", http.StatusServiceUnavailable)
				return
			}
			handle, reprepareErr = h.services.PreparedDeck.SubmitForGoal(r.Context(), owner.ID, reading.AnalysisRunID, reading.SnapshotID)
		} else {
			handle, reprepareErr = h.services.PreparedDeck.Reprepare(r.Context(), owner.ID, recovery.ReadyPreparationID)
		}
		if reprepareErr != nil {
			http.Error(w, "The identity decision was saved, but re-preparation could not be queued. The historical deck remains available; retry preparation from the deck task.", http.StatusServiceUnavailable)
			return
		}
		redirect(w, r, "/deck-preparations/"+url.PathEscape(handle.Preparation.ID)+"/status")
		return
	}
	redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review?form="+url.QueryEscape(form))
}
