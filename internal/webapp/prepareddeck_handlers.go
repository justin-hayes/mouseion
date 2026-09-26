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
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
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
	goal, err := h.services.Store.Goals.GetPrimaryGoal(r.Context(), u.ID, bookStudyLanguage(detail))
	if err != nil {
		fail(w, err)
		return
	}
	if goal.IsActive() && goal.BookID == detail.Book.ID {
		if goal.SnapshotSize == 0 || goal.SourceMaterialID != result.SourceMaterialID || goal.AnalysisRunID != result.RunID || goal.CorpusID != result.Corpus.ID {
			http.NotFound(w, r)
			return
		}
		handle, submitErr := h.submitOrRetryCurrentReadingDeck(r.Context(), u.ID, goal)
		if submitErr != nil {
			handlePreparationError(w, r, submitErr)
			return
		}
		http.Redirect(w, r, "/deck-preparations/"+url.PathEscape(handle.Preparation.ID)+"/status", http.StatusSeeOther)
		return
	}
	h.submitDeckPreparation(w, r, result.RunID, result.SourceMaterialID)
}

func (h *Handler) validJourneyDeckBook(w http.ResponseWriter, r *http.Request, owner, bookID string) (domain.MyBook, analysis.CompletedAnalysis, bool) {
	detail, ok := h.bookDetail(w, r, owner, bookID)
	if !ok || detail.Acquired == nil {
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
	goal, err := h.services.Store.Goals.GetPrimaryGoal(r.Context(), owner, bookStudyLanguage(detail))
	if err != nil {
		fail(w, err)
		return
	}
	if goal.IsActive() && goal.BookID == detail.Book.ID {
		if goal.SourceMaterialID != result.SourceMaterialID || goal.AnalysisRunID != result.RunID || goal.CorpusID != result.Corpus.ID || goal.ContentRevisionID != book.Source.ContentRevisionID || goal.ContentSnapshotID != book.Source.ContentSnapshotID {
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
	} else {
		reader, readerOK := h.services.PreparedDeck.(PreparedDeckForAnalysis)
		if !readerOK {
			task.Unavailable = true
		} else if preparation, preparationErr := reader.GetForAnalysis(r.Context(), owner, result.SourceMaterialID, result.RunID); preparationErr == nil {
			if (preparation.OwnerID != "" && preparation.OwnerID != owner) || preparation.SourceMaterialID != result.SourceMaterialID || preparation.AnalysisRunID != result.RunID || preparation.GoalSnapshotID != "" {
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
	render(w, r, JourneyDeckPreparationPage(user(r), h.csrf(w, r), task, readingBookOrLanguageHandoffURL(r.Context(), detail)))
}

func (h *Handler) createDeckPreparationForAnalysis(w http.ResponseWriter, r *http.Request, analysisID, sourceMaterialID string) {
	if !h.checkCSRF(w, r) {
		return
	}
	h.submitDeckPreparation(w, r, analysisID, sourceMaterialID)
}

func (h *Handler) submitDeckPreparation(w http.ResponseWriter, r *http.Request, analysisID, sourceMaterialID string) {
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	handle, err := h.services.PreparedDeck.Submit(r.Context(), user(r).ID, analysisID, r.FormValue("external_translation_consent") == "on")
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
	ID            string                      `json:"id"`
	State         domain.DeckPreparationState `json:"state"`
	Phase         string                      `json:"phase,omitempty"`
	Progress      int                         `json:"progress"`
	Ready         bool                        `json:"ready"`
	Error         string                      `json:"error,omitempty"`
	FailureClass  string                      `json:"failure_class,omitempty"`
	AnalysisRunID string                      `json:"analysis_run_id,omitempty"`
	Filename      string                      `json:"filename"`
	DeckName      string                      `json:"deck_name"`
	DeckRevision  int                         `json:"deck_revision"`
	DownloadURL   string                      `json:"download_url,omitempty"`
	Completeness  deckCompletenessResponse    `json:"completeness"`
	Translation   deckTranslationResponse     `json:"translation"`
	Batch         deckBatchResponse           `json:"batch"`
}

type deckCompletenessResponse struct {
	TotalCards               int  `json:"total_cards"`
	CardsWithEnglish         int  `json:"cards_with_english"`
	CardsWithEnglishSentence int  `json:"cards_with_contextual_sentence_translations"`
	CardsWithFallbackGloss   *int `json:"cards_with_fallback_gloss,omitempty"`
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
		errorMessage = preparationFailureMessage(p.FailureClass)
	}
	var fallbackGlossCount *int
	if p.State == domain.DeckPreparationReady {
		fallbackGlossCount = &p.CardsWithFallbackGloss
	}
	response := deckPreparationResponse{ID: p.ID, State: p.State, Phase: p.Phase, Progress: preparationProgress(p), Ready: p.State == domain.DeckPreparationReady && p.Error != domain.DeckPreparationRequiresRepreparationError, Error: errorMessage, FailureClass: p.FailureClass, AnalysisRunID: p.AnalysisRunID, Filename: p.Filename, DeckName: p.DeckName, DeckRevision: p.DeckRevision, Completeness: deckCompletenessResponse{TotalCards: p.TotalCards, CardsWithEnglish: p.CardsWithEnglish, CardsWithEnglishSentence: p.CardsWithContextualSentenceTranslations, CardsWithFallbackGloss: fallbackGlossCount, QualityOmissions: p.QualityOmissions}, Translation: deckTranslationResponse{Eligible: p.TranslationEligible, Completed: p.TranslationDone, Pending: p.TranslationPending, Running: p.TranslationRunning, Retrying: p.TranslationRetrying, Failed: p.TranslationFailed, Cancelled: p.TranslationCancelled}, Batch: deckBatchResponse{AgeSeconds: int64(p.BatchAge / time.Second), Chunks: p.BatchChunkCount, SubmittedChunks: p.BatchSubmittedChunks, PollingChunks: p.BatchPollingChunks, ReconcilingChunks: p.BatchReconcilingChunks, CompletedChunks: p.BatchCompletedChunks, FailedChunks: p.BatchFailedChunks, CancelledChunks: p.BatchCancelledChunks, Requests: p.BatchRequestCount, Completed: p.BatchCompletedRequests, Failed: p.BatchFailedRequests, Expired: p.BatchExpiredRequests, InputTokens: p.BatchInputTokens, OutputTokens: p.BatchOutputTokens}}
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

func writePreparationStatus(w http.ResponseWriter, p domain.DeckPreparation) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(preparationResponse(p)); err != nil {
		log.Printf("write deck preparation status: %v", err)
	}
}

func (h *Handler) deckPreparationStatus(w http.ResponseWriter, r *http.Request) {
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
		journeyAction, err = h.deckJourneyAction(r.Context(), user(r).ID, p.ID, p.SourceMaterialID)
		if err != nil {
			fail(w, err)
			return
		}
	}
	resultURL := ""
	if journeyAction.BookID != "" {
		resultURL, err = h.reachablePreparationReturnURL(r.Context(), user(r).ID, journeyAction)
		if err != nil {
			fail(w, err)
			return
		}
		if strings.Contains(resultURL, "language_handoff_book=") {
			journeyAction = emptyDeckJourneyAction()
		}
	}
	if r.Header.Get("Hx-Request") == "true" {
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
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	handle, err := h.services.PreparedDeck.Retry(r.Context(), user(r).ID, r.PathValue("id"), r.FormValue("external_translation_consent") == "on")
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

// rerenderDeckPreparation is an operational trigger for verification and
// maintenance.
func (h *Handler) rerenderDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	handle, err := h.services.PreparedDeck.Rerender(r.Context(), user(r).ID, r.PathValue("id"))
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if wantsPreparationJSON(r) {
		writePreparationStatus(w, handle.Preparation)
		return
	}
	h.redirectToPreparationStatus(w, r)
}

func wantsPreparationJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Query().Get("format") == "json"
}

func (h *Handler) redirectToPreparationStatus(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/deck-preparations/"+url.PathEscape(r.PathValue("id"))+"/status", http.StatusSeeOther)
}

func (h *Handler) reachablePreparationReturnURL(ctx context.Context, owner string, action deckJourneyActionView) (string, error) {
	detail, err := h.services.Store.Books.GetBookDetail(ctx, owner, action.BookID)
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
	return readingBookOrLanguageHandoffURL(ctx, detail), nil
}

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
	default:
		fail(w, err)
	}
}

func (h *Handler) enrichmentJobStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || h.services.Enrichment == nil {
		http.NotFound(w, r)
		return
	}
	status, err := h.services.Enrichment.Get(r.Context(), user(r).ID, id)
	if errors.Is(err, enrichmentjob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, EnrichmentJobStatus(status, h.csrf(w, r)))
}

func (h *Handler) cancelEnrichmentJob(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || h.services.Enrichment == nil {
		http.NotFound(w, r)
		return
	}
	status, err := h.services.Enrichment.Cancel(r.Context(), user(r).ID, id)
	if errors.Is(err, enrichmentjob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, EnrichmentJobStatus(status, h.csrf(w, r)))
}

func enrichmentJobLabel(status string) string {
	switch status {
	case "available", "scheduled", "retryable", "pending":
		return "Queued"
	case "running":
		return "Running"
	case "completed":
		return "Completed"
	case "cancelled":
		return "Cancelled"
	case "discarded":
		return "Discarded"
	default:
		return status
	}
}
