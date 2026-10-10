package webapp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
)

func (h *Handler) createDeckPreparation(w http.ResponseWriter, r *http.Request) {
	h.createDeckPreparationForAnalysis(w, r, r.PathValue("id"), "")
}

func (h *Handler) createJourneyEntryDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	detail, result, ok := h.validJourneyDeckBook(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	goal, err := h.services.Store.Reading.GetCurrentReading(r.Context(), u.ID, bookStudyLanguage(detail))
	if err != nil {
		fail(w, err)
		return
	}
	if !goal.IsActive() || goal.BookID != detail.Book.ID || goal.SnapshotSize == 0 || goal.SourceMaterialID != result.SourceMaterialID || goal.AnalysisRunID != result.RunID || goal.CorpusID != result.Corpus.ID {
		http.NotFound(w, r)
		return
	}
	if !expectedCommitmentMatches(r, goal.SnapshotID) {
		h.respondGoal(w, r, "", goalStaleMessage, goal.BookID)
		return
	}
	handle, submitErr := h.submitOrRetryCurrentReadingDeck(r.Context(), u.ID, goal)
	if submitErr != nil {
		handlePreparationError(w, r, submitErr)
		return
	}
	http.Redirect(w, r, "/deck-preparations/"+url.PathEscape(handle.Preparation.ID)+"/status", http.StatusSeeOther)
}

func (h *Handler) validJourneyDeckBook(w http.ResponseWriter, r *http.Request, owner, bookID string) (domain.MyBook, analysis.CompletedAnalysis, bool) {
	detail, ok := h.bookDetail(w, r, owner, bookID)
	if !ok || detail.Acquired == nil {
		http.NotFound(w, r)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	goal, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, bookStudyLanguage(detail))
	if err != nil {
		fail(w, err)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	if !goal.IsActive() || goal.BookID != detail.Book.ID {
		http.NotFound(w, r)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	book := *detail.Acquired
	if book.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(book) {
		http.NotFound(w, r)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	if err := h.annotateBookToReadLanguage(r.Context(), owner, bookStudyLanguage(detail), &detail); err != nil {
		fail(w, err)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	if !detail.IsToRead {
		http.NotFound(w, r)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	reader, ok := h.services.Analysis.(CompletedAnalysisReader)
	if !ok {
		http.NotFound(w, r)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	result, err := reader.GetCompletedAnalysis(r.Context(), owner, book.Source.ID, book.AnalysisRunID)
	if errors.Is(err, analysis.ErrNotFound) || errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	if result.OwnerID != owner || result.SourceMaterialID != book.Source.ID || result.RunID != book.AnalysisRunID || result.Corpus.ID != book.CorpusID {
		http.NotFound(w, r)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	if _, detectErr := h.ensureLemmaReviewFlags(r.Context(), owner, detail); detectErr != nil {
		fail(w, detectErr)
		return domain.MyBook{}, analysis.CompletedAnalysis{}, false
	}
	return detail, result, true
}

type journeyDeckPreparationView struct {
	Book             domain.SourceMaterialSummary
	BookID           string
	AnalysisRunID    string
	GoalSnapshotID   string
	GoalSnapshotSize int
	Goal             bool
	Preparation      *domain.DeckPreparation
	Missing          bool
	Unavailable      bool
}

func (h *Handler) newJourneyDeckPreparation(w http.ResponseWriter, r *http.Request) {
	owner := user(r).ID
	detail, result, ok := h.validJourneyDeckBook(w, r, owner, r.PathValue("bookID"))
	if !ok {
		return
	}
	book := *detail.Acquired
	book.BookID = detail.Book.ID
	book.BookTitle = detail.Book.Title
	task := journeyDeckPreparationView{Book: book, BookID: detail.Book.ID, AnalysisRunID: result.RunID}
	goal, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, bookStudyLanguage(detail))
	if err != nil {
		fail(w, err)
		return
	}
	if !goal.IsActive() || goal.BookID != detail.Book.ID || goal.SourceMaterialID != result.SourceMaterialID || goal.AnalysisRunID != result.RunID || goal.CorpusID != result.Corpus.ID || goal.ContentRevisionID != book.Source.ContentRevisionID || goal.ContentSnapshotID != book.Source.ContentSnapshotID {
		http.NotFound(w, r)
		return
	}
	task.Goal = true
	task.GoalSnapshotID = goal.SnapshotID
	task.GoalSnapshotSize = goal.SnapshotSize
	if goal.SnapshotSize > 0 {
		reader, readerOK := h.services.PreparedDeck.(PreparedDeckForGoalSnapshot)
		if !readerOK {
			task.Unavailable = true
		} else if preparation, preparationErr := reader.GetForGoalSnapshot(r.Context(), owner, goal.SnapshotID); preparationErr == nil {
			if !currentReadingPreparationMatches(preparation, owner, goal) {
				http.NotFound(w, r)
				return
			}
			task.Preparation = &preparation
		} else if errors.Is(preparationErr, persistence.ErrNotFound) {
			task.Missing = true
		} else {
			fail(w, preparationErr)
			return
		}
	}
	render(w, r, JourneyDeckPreparationPage(user(r), h.csrf(w, r), task, readingBookURLInActiveLanguage(r.Context(), detail)))
}

func (h *Handler) createDeckPreparationForAnalysis(w http.ResponseWriter, r *http.Request, analysisID, sourceMaterialID string) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.submitCurrentBookDeckPreparation(w, r, analysisID) {
		return
	}
	if handled := h.preflightDirectDeckLemmaReview(w, r, analysisID); handled {
		return
	}
	h.submitDeckPreparation(w, r, analysisID, sourceMaterialID)
}

// submitCurrentBookDeckPreparation keeps the former analysis-job route from
// becoming an independent selection path. A Book can use it only while it is
// Current reading, and the submitted preparation is bound to that exact frozen
// snapshot. Non-Book analysis results retain the generic preparation path.
func (h *Handler) submitCurrentBookDeckPreparation(w http.ResponseWriter, r *http.Request, analysisID string) bool {
	jobID, err := strconv.ParseInt(analysisID, 10, 64)
	if err != nil || jobID <= 0 {
		return false
	}
	if h.services.Analysis == nil {
		return false
	}
	status, err := h.services.Analysis.Get(r.Context(), user(r).ID, jobID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) || errors.Is(err, analysis.ErrNotFound) {
			http.NotFound(w, r)
		} else {
			fail(w, err)
		}
		return true
	}
	bookID, isBook, err := h.resolveBookID(r.Context(), user(r).ID, status.SourceMaterialID)
	if err != nil {
		fail(w, err)
		return true
	}
	if !isBook {
		return false
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), user(r).ID, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
		} else {
			fail(w, err)
		}
		return true
	}
	goal, err := h.services.Store.Reading.GetCurrentReading(r.Context(), user(r).ID, bookStudyLanguage(detail))
	if err != nil {
		fail(w, err)
		return true
	}
	if !goal.IsActive() || goal.BookID != bookID || goal.SourceMaterialID != status.SourceMaterialID || goal.AnalysisRunID != status.RunID {
		http.NotFound(w, r)
		return true
	}
	if h.services.PreparedDeck == nil || goal.SnapshotSize == 0 {
		http.NotFound(w, r)
		return true
	}
	if !expectedCommitmentMatches(r, goal.SnapshotID) {
		h.respondGoal(w, r, "", goalStaleMessage, goal.BookID)
		return true
	}
	handle, err := h.services.PreparedDeck.SubmitForGoal(r.Context(), user(r).ID, goal.AnalysisRunID, goal.SnapshotID)
	if err != nil {
		handlePreparationError(w, r, err)
		return true
	}
	http.Redirect(w, r, "/deck-preparations/"+url.PathEscape(handle.Preparation.ID)+"/status", http.StatusSeeOther)
	return true
}

func (h *Handler) preflightDirectDeckLemmaReview(w http.ResponseWriter, r *http.Request, analysisID string) bool {
	jobID, err := strconv.ParseInt(analysisID, 10, 64)
	if err != nil || h.services.Analysis == nil {
		return false
	}
	status, err := h.services.Analysis.Get(r.Context(), user(r).ID, jobID)
	if err != nil || status.RunID == "" || status.SourceMaterialID == "" {
		return false
	}
	reader, ok := h.services.Analysis.(CompletedAnalysisReader)
	if !ok {
		return false
	}
	completed, err := reader.GetCompletedAnalysis(r.Context(), user(r).ID, status.SourceMaterialID, status.RunID)
	if err != nil {
		return false
	}
	books, err := h.services.Store.Reading.ListSourceMaterials(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return true
	}
	for _, item := range books {
		if item.Source.ID != status.SourceMaterialID || item.BookID == "" {
			continue
		}
		detail, detailErr := h.services.Store.Reading.GetBookDetail(r.Context(), user(r).ID, item.BookID)
		if detailErr != nil {
			fail(w, detailErr)
			return true
		}
		if detail.Acquired == nil || detail.Acquired.AnalysisRunID != completed.RunID || detail.Acquired.CorpusID != completed.Corpus.ID {
			return false
		}
		if _, detectErr := h.ensureLemmaReviewFlags(r.Context(), user(r).ID, detail); detectErr != nil {
			fail(w, detectErr)
			return true
		}
		occurrences, listErr := h.services.Store.Reading.ListLemmaReviewOccurrences(r.Context(), user(r).ID, item.BookID, "")
		if listErr != nil {
			fail(w, listErr)
			return true
		}
		for _, occurrence := range occurrences {
			if occurrence.ReviewFlagReason != "" && occurrence.ReviewFlagResolution == "" {
				http.Redirect(w, r, "/reading/books/"+url.PathEscape(item.BookID)+"/lemma-review", http.StatusSeeOther)
				return true
			}
		}
		return false
	}
	return false
}

func (h *Handler) submitDeckPreparation(w http.ResponseWriter, r *http.Request, analysisID, sourceMaterialID string) {
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	handle, err := h.services.PreparedDeck.Submit(r.Context(), user(r).ID, analysisID)
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if sourceMaterialID != "" && (handle.Preparation.SourceMaterialID != sourceMaterialID || handle.Preparation.AnalysisRunID != analysisID) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Location", "/deck-preparations/"+url.PathEscape(handle.Preparation.ID)+"/status")
	w.WriteHeader(http.StatusSeeOther)
}

type deckPreparationResponse struct {
	ID               string                         `json:"id"`
	State            domain.DeckPreparationState    `json:"state"`
	Phase            string                         `json:"phase,omitempty"`
	Progress         int                            `json:"progress"`
	Ready            bool                           `json:"ready"`
	Error            string                         `json:"error,omitempty"`
	FailureClass     string                         `json:"failure_class,omitempty"`
	AnalysisRunID    string                         `json:"analysis_run_id,omitempty"`
	Filename         string                         `json:"filename"`
	DeckName         string                         `json:"deck_name"`
	DeckRevision     int                            `json:"deck_revision"`
	DownloadURL      string                         `json:"download_url,omitempty"`
	Completeness     deckCompletenessResponse       `json:"completeness"`
	MeaningOmissions []deckMeaningOmissionResponse  `json:"meaning_omissions,omitempty"`
	Evidence         []deckEvidenceCoverageResponse `json:"evidence_coverage,omitempty"`
	Translation      deckTranslationResponse        `json:"translation"`
	Batch            deckBatchResponse              `json:"batch"`
}

type deckMeaningOmissionResponse struct {
	Target string `json:"target"`
	Reason string `json:"reason"`
}

type deckEvidenceCoverageResponse struct {
	Source     string `json:"source"`
	Configured bool   `json:"configured"`
	Selected   int    `json:"selected"`
	Matched    int    `json:"matched"`
	Candidates int    `json:"candidates"`
	Omitted    int    `json:"omitted_candidates"`
}

type deckCompletenessResponse struct {
	TotalCards               int  `json:"total_cards"`
	CardsWithEnglish         int  `json:"cards_with_english"`
	CardsWithEnglishSentence int  `json:"cards_with_contextual_sentence_translations"`
	CardsWithFallbackGloss   *int `json:"cards_with_fallback_gloss,omitempty"`
	CardsWithContextualGloss *int `json:"cards_with_contextual_gloss,omitempty"`
	ContextOnlyGlosses       *int `json:"context_only_glosses,omitempty"`
	QualityOmissions         int  `json:"quality_omissions"`
}

type deckTranslationResponse struct {
	Eligible  int `json:"eligible"`
	Completed int `json:"completed"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Retrying  int `json:"retrying"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

type deckBatchResponse struct {
	AgeSeconds        int64 `json:"age_seconds"`
	Chunks            int   `json:"chunks"`
	SubmittedChunks   int   `json:"submitted_chunks"`
	PollingChunks     int   `json:"polling_chunks"`
	ReconcilingChunks int   `json:"reconciling_chunks"`
	CompletedChunks   int   `json:"completed_chunks"`
	FailedChunks      int   `json:"failed_chunks"`
	CancelledChunks   int   `json:"cancelled_chunks"`
	Requests          int   `json:"requests"`
	Completed         int   `json:"completed"`
	Failed            int   `json:"failed"`
	Expired           int   `json:"expired"`
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
}

func preparationResponse(p domain.DeckPreparation) deckPreparationResponse {
	errorMessage := ""
	if p.Error == domain.DeckPreparationRequiresRepreparationError {
		errorMessage = domain.DeckPreparationRequiresRepreparationError
	} else if p.State == domain.DeckPreparationFailed {
		errorMessage = preparationFailureMessageFor(p)
	}
	var fallbackGlossCount *int
	var contextualGlossCount, contextOnlyGlossCount *int
	if p.State == domain.DeckPreparationReady {
		if p.ContextualGlossesReported {
			contextualGlossCount = &p.ContextualGlosses
			contextOnlyGlossCount = &p.ContextOnlyGlosses
		} else {
			fallbackGlossCount = &p.CardsWithFallbackGloss
		}
	}
	response := deckPreparationResponse{ID: p.ID, State: p.State, Phase: p.Phase, Progress: preparationProgress(p), Ready: p.State == domain.DeckPreparationReady && p.Error != domain.DeckPreparationRequiresRepreparationError, Error: errorMessage, FailureClass: p.FailureClass, AnalysisRunID: p.AnalysisRunID, Filename: p.Filename, DeckName: p.DeckName, DeckRevision: p.DeckRevision, Completeness: deckCompletenessResponse{TotalCards: p.TotalCards, CardsWithEnglish: p.CardsWithEnglish, CardsWithEnglishSentence: p.CardsWithContextualSentenceTranslations, CardsWithFallbackGloss: fallbackGlossCount, CardsWithContextualGloss: contextualGlossCount, ContextOnlyGlosses: contextOnlyGlossCount, QualityOmissions: p.QualityOmissions}, Translation: deckTranslationResponse{Eligible: p.TranslationEligible, Completed: p.TranslationDone, Pending: p.TranslationPending, Running: p.TranslationRunning, Retrying: p.TranslationRetrying, Failed: p.TranslationFailed, Cancelled: p.TranslationCancelled}, Batch: deckBatchResponse{AgeSeconds: int64(p.BatchAge / time.Second), Chunks: p.BatchChunkCount, SubmittedChunks: p.BatchSubmittedChunks, PollingChunks: p.BatchPollingChunks, ReconcilingChunks: p.BatchReconcilingChunks, CompletedChunks: p.BatchCompletedChunks, FailedChunks: p.BatchFailedChunks, CancelledChunks: p.BatchCancelledChunks, Requests: p.BatchRequestCount, Completed: p.BatchCompletedRequests, Failed: p.BatchFailedRequests, Expired: p.BatchExpiredRequests, InputTokens: p.BatchInputTokens, OutputTokens: p.BatchOutputTokens}}
	for _, coverage := range p.EvidenceCoverage {
		response.Evidence = append(response.Evidence, deckEvidenceCoverageResponse{Source: coverage.Source, Configured: coverage.Configured, Selected: coverage.Selected, Matched: coverage.Matched, Candidates: coverage.Candidates, Omitted: coverage.Omitted})
	}
	for _, omission := range p.MeaningOmissions {
		response.MeaningOmissions = append(response.MeaningOmissions, deckMeaningOmissionResponse{Target: omission.TargetWord, Reason: omission.Reason})
	}
	if response.Ready && !deckPreparationEmpty(p) && p.Error != domain.DeckPreparationRequiresRepreparationError {
		response.DownloadURL = "/deck-preparations/" + url.PathEscape(p.ID) + "/download"
	}
	return response
}

func preparationFailureMessage(class string) string {
	switch class {
	case "configuration", "unsupported_model":
		return "External translation is not available for this preparation. Check the provider configuration and retry."
	case "ambiguous_submission":
		return "Translation submission could not be confirmed safely. Retry the preparation later."
	case "validation", "malformed_result", "missing_result", "duplicate_result", "unknown_result", "reconciliation":
		return "Translation results could not be verified. Retry the preparation."
	case "provider", "upload", "poll":
		return "The translation provider is temporarily unavailable. Retry the preparation later."
	case "cancelled":
		return ""
	default:
		return "Deck preparation could not be completed. Retry the preparation."
	}
}

func preparationFailureMessageFor(p domain.DeckPreparation) string {
	if strings.Contains(strings.ToLower(p.Error), "configured translation provider") {
		return "Contextual translation is required for every new deck. Configure the translation provider, then retry; no local-only deck was published."
	}
	return preparationFailureMessage(p.FailureClass)
}

func writePreparationStatus(w http.ResponseWriter, p domain.DeckPreparation) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(preparationResponse(p)); err != nil {
		log.Printf("write deck preparation status: %v", err)
	}
}

func (h *Handler) deckPreparationStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Accept")
	w.Header().Add("Vary", "HX-Request-Type")
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.services.PreparedDeck.Get(r.Context(), user(r).ID, r.PathValue("id"))
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if wantsPreparationJSON(r) {
		writePreparationStatus(w, p)
		return
	}
	journeyAction := emptyDeckJourneyAction()
	if p.SourceMaterialID != "" {
		journeyAction, err = h.deckJourneyStatus(r.Context(), user(r).ID, p.SourceMaterialID)
		if err != nil {
			fail(w, err)
			return
		}
	}
	resultURL := ""
	if journeyAction.BookID != "" {
		resultURL, err = h.reachablePreparationReturnURL(r.Context(), user(r).ID, journeyAction)
		if errors.Is(err, errPreparationOtherLanguage) {
			err = nil
			journeyAction = emptyDeckJourneyAction()
		}
		if err != nil {
			fail(w, err)
			return
		}
	}
	if isPartialHTMXRequest(r) {
		render(w, r, DeckPreparationStatus(h.csrf(w, r), p, resultURL, journeyAction))
		return
	}
	render(w, r, DeckPreparationStatusPage(user(r), h.csrf(w, r), p, resultURL, journeyAction))
}

func (h *Handler) cancelDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.services.PreparedDeck.Cancel(r.Context(), user(r).ID, r.PathValue("id"))
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if wantsPreparationJSON(r) {
		writePreparationStatus(w, p)
		return
	}
	h.redirectToPreparationStatus(w, r)
}

func (h *Handler) retryDeckPreparation(w http.ResponseWriter, r *http.Request) {
	h.runPreparationGeneration(w, r, preparationRetry)
}

func (h *Handler) reprepareDeckPreparation(w http.ResponseWriter, r *http.Request) {
	h.runPreparationGeneration(w, r, preparationReprepare)
}

// rerenderDeckPreparation is an operational trigger for verification and
// maintenance.
func (h *Handler) rerenderDeckPreparation(w http.ResponseWriter, r *http.Request) {
	h.runPreparationGeneration(w, r, preparationRerender)
}

type preparationGenerationAction uint8

const (
	preparationRetry preparationGenerationAction = iota
	preparationReprepare
	preparationRerender
)

func (h *Handler) runPreparationGeneration(w http.ResponseWriter, r *http.Request, action preparationGenerationAction) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	owner, id := user(r).ID, r.PathValue("id")
	preparation, err := h.services.PreparedDeck.Get(r.Context(), owner, id)
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if !h.allowPreparationGeneration(w, r, owner, preparation) {
		return
	}
	var handle prepareddeck.Handle
	switch action {
	case preparationRetry:
		handle, err = h.services.PreparedDeck.Retry(r.Context(), owner, id)
	case preparationReprepare:
		handle, err = h.services.PreparedDeck.Reprepare(r.Context(), owner, id)
	case preparationRerender:
		handle, err = h.services.PreparedDeck.Rerender(r.Context(), owner, id)
	default:
		fail(w, errors.New("unknown deck preparation generation action"))
		return
	}
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if wantsPreparationJSON(r) {
		writePreparationStatus(w, handle.Preparation)
		return
	}
	http.Redirect(w, r, "/deck-preparations/"+url.PathEscape(handle.Preparation.ID)+"/status", http.StatusSeeOther)
}

// allowPreparationGeneration prevents historical Book preparations (including
// ones made by the retired analysis-job path) from being retried, re-prepared,
// or presentation-rerendered. Only the exact active Reading snapshot can
// create another generation, and the request must name that snapshot. Non-Book preparations retain their existing
// lifecycle.
func (h *Handler) allowPreparationGeneration(w http.ResponseWriter, r *http.Request, owner string, preparation domain.DeckPreparation) bool {
	bookID := preparation.BookID
	if bookID == "" {
		return true
	}
	if preparation.GoalSnapshotID == "" {
		http.NotFound(w, r)
		return false
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
		} else {
			fail(w, err)
		}
		return false
	}
	goal, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, bookStudyLanguage(detail))
	if err != nil {
		fail(w, err)
		return false
	}
	if !goal.IsActive() || goal.BookID != bookID || goal.SnapshotID != preparation.GoalSnapshotID || goal.SourceMaterialID != preparation.SourceMaterialID || goal.AnalysisRunID != preparation.AnalysisRunID {
		http.NotFound(w, r)
		return false
	}
	if !expectedCommitmentMatches(r, preparation.GoalSnapshotID) {
		if wantsPreparationJSON(r) {
			http.Error(w, goalStaleMessage, http.StatusConflict)
		} else {
			h.respondGoal(w, r, "", goalStaleMessage, bookID)
		}
		return false
	}
	return true
}

func wantsPreparationJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Query().Get("format") == "json"
}

func (h *Handler) redirectToPreparationStatus(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/deck-preparations/"+url.PathEscape(r.PathValue("id"))+"/status", http.StatusSeeOther)
}

func (h *Handler) reachablePreparationReturnURL(ctx context.Context, owner string, action deckJourneyActionView) (string, error) {
	detail, err := h.services.Store.Reading.GetBookDetail(ctx, owner, action.BookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	if detail.Acquired == nil || detail.Acquired.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(*detail.Acquired) {
		return "", nil
	}
	if err := h.annotateBookToReadLanguage(ctx, owner, bookStudyLanguage(detail), &detail); err != nil {
		return "", err
	}
	if !detail.IsToRead {
		return "", nil
	}
	target := readingBookURLInActiveLanguage(ctx, detail)
	if target == "" {
		return "", errPreparationOtherLanguage
	}
	return target, nil
}

// errPreparationOtherLanguage marks a preparation whose Book belongs to another
// study language, so it offers no Reading link or membership action.
var errPreparationOtherLanguage = errors.New("preparation Book is in another study language")

func (h *Handler) downloadDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.services.PreparedDeck.Download(r.Context(), user(r).ID, r.PathValue("id"))
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.anki")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": p.Filename}))
	w.Header().Set("X-Mouseion-Deck-Name", p.DeckName)
	w.Header().Set("X-Mouseion-Analysis-Run-Id", p.AnalysisRunID)
	w.Header().Set("X-Mouseion-Cards-Total", strconv.Itoa(p.TotalCards))
	w.Header().Set("X-Mouseion-Cards-With-English", strconv.Itoa(p.CardsWithEnglish))
	w.Header().Set("X-Mouseion-Cards-With-English-Sentence", strconv.Itoa(p.CardsWithContextualSentenceTranslations))
	w.Header().Set("X-Mouseion-Cards-Quality-Omitted", strconv.Itoa(p.QualityOmissions))
	w.Header().Set("X-Mouseion-Deck-Revision", strconv.Itoa(p.DeckRevision))
	if _, err = w.Write(p.Artifact); err != nil {
		log.Printf("write deck preparation artifact: %v", err)
	}
}

func handlePreparationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, prepareddeck.ErrAnalysisUnavailable), errors.Is(err, prepareddeck.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, persistence.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, persistence.ErrInvalidTransition):
		http.Error(w, "invalid deck preparation state", http.StatusConflict)
	case errors.Is(err, persistence.ErrUnresolvedLemmaReviewFlags):
		http.Error(w, "Resolve the flagged lemma occurrences before preparing a direct deck.", http.StatusConflict)
	default:
		fail(w, err)
	}
}
