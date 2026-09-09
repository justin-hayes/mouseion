package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	if err := h.annotateBookWithJourney(r.Context(), u.ID, &detail); err != nil {
		fail(w, err)
		return
	}
	if detail.EvidenceState() == domain.BookNotAcquired {
		eligible, err := h.bookRefreshEligible(r.Context(), u.ID, detail.Book.ID)
		if err != nil {
			fail(w, err)
			return
		}
		render(w, r, MetadataOnlyBookPage(u, h.csrf(w, r), detail, r.URL.Query().Get("message"), eligible))
		return
	}
	if detail.Acquired == nil {
		http.NotFound(w, r)
		return
	}
	summary := *detail.Acquired
	var coverage *domain.AnalysisCoverage
	statisticsUnavailable := false
	if summary.AnalysisStatus == "analyzed" && h.services.AnalysisInsights != nil {
		value, err := h.services.AnalysisInsights.Coverage(r.Context(), u.ID, summary.CorpusID)
		if errors.Is(err, analysisinsights.ErrStatisticsUnavailable) {
			statisticsUnavailable = true
		} else if err != nil {
			fail(w, err)
			return
		} else {
			coverage = &value
		}
	}
	history, err := h.services.Store.ListAnalysisJobs(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	bookHistory := make([]domain.AnalysisJob, 0, len(history))
	for _, job := range history {
		if job.SourceMaterialID == summary.Source.ID {
			bookHistory = append(bookHistory, job)
		}
	}
	preparation, journeyAction, ok := h.currentBookPreparation(w, r, u.ID, summary)
	if !ok {
		return
	}
	if preparation == nil {
		preparation, ok = h.currentVocabularyStudyPreparation(w, r, u.ID, summary, false)
		if !ok {
			return
		}
	}
	render(w, r, BookPageWithHistoryAndPreparation(u, h.csrf(w, r), summary, coverage, statisticsUnavailable, bookHistory, r.URL.Query().Get("message"), preparation, journeyAction))
}

func (h *Handler) bookDetail(w http.ResponseWriter, r *http.Request, owner, id string) (domain.MyBook, bool) {
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return domain.MyBook{}, false
	}
	detail, err := h.services.Store.GetBookDetail(r.Context(), owner, id)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return domain.MyBook{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.MyBook{}, false
	}
	return detail, true
}

type catalogueAliasReader interface {
	GetBookCatalogEntryAlias(context.Context, string, string) (domain.BookAlias, error)
}

func (h *Handler) bookRefreshEligible(ctx context.Context, owner, bookID string) (bool, error) {
	reader, ok := h.services.Store.(catalogueAliasReader)
	if !ok {
		return false, nil
	}
	alias, err := reader.GetBookCatalogEntryAlias(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if alias.AliasType != domain.AliasCatalogEntry || alias.Namespace != domain.NamespaceSourceIdentifier || strings.TrimSpace(alias.Value) == "" {
		return false, nil
	}
	if strings.TrimSpace(alias.ConnectionID) == "" {
		return false, nil
	}
	_, err = h.services.Store.GetOpdsConnection(ctx, owner, alias.ConnectionID)
	if errors.Is(err, persistence.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (h *Handler) refreshBookMetadata(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	refresher, ok := h.services.CatalogueSync.(CatalogueMetadataRefresher)
	if !ok {
		h.renderBookRefreshFailure(w, r, u, r.PathValue("id"))
		return
	}
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	result, err := refresher.RefreshEntry(r.Context(), u.ID, detail.Book.ID)
	if errors.Is(err, cataloguesync.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		result.Failed = true
	}
	if result.Book.ID == "" {
		result.Book = detail.Book
	}
	message := refreshMessage(result)
	if isHTMX(r) {
		render(w, r, MetadataOnlyBookMetadataRegion(h.csrf(w, r), domain.MyBook{Book: result.Book}, message))
		return
	}
	redirect(w, r, "/books/"+url.PathEscape(r.PathValue("id"))+"?message="+url.QueryEscape(message))
}

func (h *Handler) renderBookRefreshFailure(w http.ResponseWriter, r *http.Request, u domain.User, bookID string) {
	message := "Metadata could not be refreshed. Check the connection and try again."
	if isHTMX(r) {
		book, ok := h.bookDetail(w, r, u.ID, bookID)
		if !ok {
			return
		}
		render(w, r, MetadataOnlyBookMetadataRegion(h.csrf(w, r), domain.MyBook{Book: book.Book}, message))
		return
	}
	redirect(w, r, "/books/"+url.PathEscape(bookID)+"?message="+url.QueryEscape(message))
}

func refreshMessage(result cataloguesync.RefreshResult) string {
	switch {
	case result.Updated:
		return "Metadata refreshed."
	case result.Missing:
		return "The catalogue entry is no longer available. Your book and its metadata are unchanged."
	case result.Failed:
		return "Metadata could not be refreshed. Check the connection and try again."
	default:
		return "Metadata is already up to date."
	}
}

func (h *Handler) analysisResult(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	if detail.Acquired == nil {
		http.NotFound(w, r)
		return
	}
	book := *detail.Acquired
	runID := r.PathValue("runID")
	redirectBookID := detail.Book.ID
	if book.AnalysisRunID != runID {
		jobs, err := h.services.Store.ListAnalysisJobs(r.Context(), u.ID)
		if err != nil {
			fail(w, err)
			return
		}
		found := false
		for _, job := range jobs {
			if job.SourceMaterialID == book.Source.ID && job.AnalysisRunID == runID {
				found = true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
	}
	http.Redirect(w, r, "/books/"+url.PathEscape(redirectBookID), http.StatusSeeOther)
}

func (h *Handler) currentBookPreparation(w http.ResponseWriter, r *http.Request, owner string, book domain.SourceMaterialSummary) (*domain.DeckPreparation, deckJourneyActionView, bool) {
	journeyAction := emptyDeckJourneyAction()
	if book.AnalysisStatus != "analyzed" || book.AnalysisState != "completed" || book.AnalysisRunID == "" || h.services.PreparedDeck == nil {
		return nil, journeyAction, true
	}
	reader, ok := h.services.PreparedDeck.(PreparedDeckForAnalysis)
	if !ok {
		return nil, journeyAction, true
	}
	preparation, err := reader.GetForAnalysis(r.Context(), owner, book.Source.ID, book.AnalysisRunID)
	if errors.Is(err, persistence.ErrNotFound) {
		return nil, journeyAction, true
	}
	if err != nil {
		handlePreparationError(w, r, err)
		return nil, journeyAction, false
	}
	if preparation.State == domain.DeckPreparationReady {
		journeyAction, err = h.deckJourneyAction(r.Context(), owner, preparation.ID, preparation.SourceMaterialID)
		if err != nil {
			fail(w, err)
			return nil, journeyAction, false
		}
		if studyStore, ok := h.services.Store.(VocabularyStudyStore); ok {
			preparation.VocabularyCount, err = studyStore.CountDeckPreparationVocabularyToGraduate(r.Context(), owner, preparation.ID)
			if err != nil {
				fail(w, err)
				return nil, journeyAction, false
			}
		}
	}
	return &preparation, journeyAction, true
}

func (h *Handler) bookVocabularyStudyPreparation(w http.ResponseWriter, r *http.Request) (domain.DeckPreparation, bool) {
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return domain.DeckPreparation{}, false
	}
	if detail.Acquired == nil {
		http.NotFound(w, r)
		return domain.DeckPreparation{}, false
	}
	preparation, _, ok := h.currentBookPreparation(w, r, u.ID, *detail.Acquired)
	if !ok {
		return domain.DeckPreparation{}, false
	}
	if preparation == nil {
		preparation, ok = h.currentVocabularyStudyPreparation(w, r, u.ID, *detail.Acquired, true)
		if !ok {
			return domain.DeckPreparation{}, false
		}
	}
	if preparation == nil {
		http.Error(w, "this Book has no current prepared deck", http.StatusConflict)
		return domain.DeckPreparation{}, false
	}
	return *preparation, true
}

func (h *Handler) currentVocabularyStudyPreparation(w http.ResponseWriter, r *http.Request, owner string, book domain.SourceMaterialSummary, includeReady bool) (*domain.DeckPreparation, bool) {
	if book.AnalysisStatus != "analyzed" || book.AnalysisState != "completed" || book.AnalysisRunID == "" {
		return nil, true
	}
	reader, ok := h.services.Store.(VocabularyStudyPreparationReader)
	if !ok {
		return nil, true
	}
	preparation, err := reader.GetDeckPreparationForAnalysis(r.Context(), owner, book.Source.ID, book.AnalysisRunID)
	if errors.Is(err, persistence.ErrNotFound) {
		return nil, true
	}
	if err != nil {
		handlePreparationError(w, r, err)
		return nil, false
	}
	if preparation.VocabularyStudyStatus() == domain.VocabularyStudyNotStarted && !includeReady {
		preparation, err = reader.GetActiveDeckVocabularyStudy(r.Context(), owner, book.Source.ID)
		if errors.Is(err, persistence.ErrNotFound) {
			return nil, true
		}
		if err != nil {
			handlePreparationError(w, r, err)
			return nil, false
		}
	}
	if preparation.State == domain.DeckPreparationReady {
		studyStore, ok := h.services.Store.(VocabularyStudyStore)
		if ok {
			preparation.VocabularyCount, err = studyStore.CountDeckPreparationVocabularyToGraduate(r.Context(), owner, preparation.ID)
			if err != nil {
				fail(w, err)
				return nil, false
			}
		}
	}
	return &preparation, true
}

func (h *Handler) startBookVocabularyStudy(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	store, ok := h.services.Store.(VocabularyStudyStore)
	if !ok {
		http.NotFound(w, r)
		return
	}
	preparation, ok := h.bookVocabularyStudyPreparation(w, r)
	if !ok {
		return
	}
	updated, err := store.StartDeckVocabularyStudy(r.Context(), user(r).ID, preparation.ID)
	if errors.Is(err, persistence.ErrActiveVocabularyStudy) {
		redirectBookVocabularyStudyError(w, r, "Finish or release the other Book's vocabulary study before starting this one.")
		return
	}
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirectBookVocabularyStudyError(w, r, "Only a ready, non-empty deck can start a vocabulary study.")
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	message := "Study this Book's vocabulary started. Its reserved vocabulary is not counted as known."
	if updated.ID == "" {
		message = "This Book's vocabulary study is already in progress."
	}
	redirectBookVocabularyStudy(w, r, message)
}

func (h *Handler) confirmBookVocabularyReview(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	store, ok := h.services.Store.(VocabularyStudyStore)
	if !ok {
		http.NotFound(w, r)
		return
	}
	preparation, ok := h.bookVocabularyStudyPreparation(w, r)
	if !ok {
		return
	}
	count := preparation.VocabularyCount
	updated, err := store.ConfirmDeckVocabularyReview(r.Context(), user(r).ID, preparation.ID)
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirectBookVocabularyStudyError(w, r, "Start this Book's vocabulary study before confirming its deck review.")
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirectBookVocabularyStudy(w, r, fmt.Sprintf("Deck review confirmed. %d vocabulary identities graduated to known vocabulary.", count))
	_ = updated
}

func (h *Handler) releaseBookVocabularyStudy(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	store, ok := h.services.Store.(VocabularyStudyStore)
	if !ok {
		http.NotFound(w, r)
		return
	}
	preparation, ok := h.bookVocabularyStudyPreparation(w, r)
	if !ok {
		return
	}
	if _, err := store.ReleaseDeckVocabularyStudy(r.Context(), user(r).ID, preparation.ID); errors.Is(err, persistence.ErrInvalidTransition) {
		redirectBookVocabularyStudyError(w, r, "A reviewed Book vocabulary study cannot be released.")
	} else if err != nil {
		fail(w, err)
	} else {
		redirectBookVocabularyStudy(w, r, "Vocabulary study released. Its provenance remains recorded and the vocabulary is eligible again.")
	}
}

func redirectBookVocabularyStudy(w http.ResponseWriter, r *http.Request, message string) {
	redirect(w, r, "/books/"+url.PathEscape(r.PathValue("id"))+"?message="+url.QueryEscape(message))
}

func redirectBookVocabularyStudyError(w http.ResponseWriter, r *http.Request, message string) {
	// Book detail currently has one feedback slot; keep blocked-study copy in
	// that visible slot rather than silently dropping it on the redirect.
	redirect(w, r, "/books/"+url.PathEscape(r.PathValue("id"))+"?message="+url.QueryEscape("Study action blocked: "+message))
}

func (h *Handler) analyzeBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	if detail.Acquired == nil {
		target, err := h.acquireBookForAnalysis(r, u.ID, detail.Book.ID)
		if err != nil {
			message := analysisAcquisitionError(r.Context(), h.services.Store, u.ID, detail.Book.ID, detail.Book.Title, target, err)
			redirect(w, r, "/books/"+url.PathEscape(detail.Book.ID)+"?message="+url.QueryEscape(message))
			return
		}
		// Acquisition promotes the existing Book. Reload it so analysis uses the
		// persisted content revision and extracted-unit snapshot.
		detail, ok = h.bookDetail(w, r, u.ID, detail.Book.ID)
		if !ok {
			return
		}
	}
	if detail.Acquired == nil {
		http.NotFound(w, r)
		return
	}
	book := *detail.Acquired
	if book.Source.MediaType != "application/epub+zip" {
		http.Error(w, "Analysis requires an EPUB source.", http.StatusConflict)
		return
	}
	handle, err := h.services.Analysis.SubmitAnalysis(r.Context(), u.ID, book.Source.ID)
	if err != nil {
		if errors.Is(err, domain.ErrExtractedUnitsUnavailable) || errors.Is(err, analysis.ErrEPUBRequired) {
			http.Error(w, "Analysis is unavailable because this book has no extracted EPUB units.", http.StatusConflict)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/books/%s?message=Analysis+job+%d+submitted", r.PathValue("id"), handle.DisplayNumber))
}

func (h *Handler) acquireBookForAnalysis(r *http.Request, owner, bookID string) (cataloguesync.AcquisitionTarget, error) {
	return h.acquireBookForAnalysisContext(r.Context(), owner, bookID)
}

func (h *Handler) acquireBookForAnalysisContext(ctx context.Context, owner, bookID string) (cataloguesync.AcquisitionTarget, error) {
	provider, ok := h.services.CatalogueSync.(CatalogueAcquisitionTargetProvider)
	if !ok {
		return cataloguesync.AcquisitionTarget{}, errors.New("catalogue acquisition is unavailable")
	}
	target, err := provider.FindAcquisitionTarget(ctx, owner, bookID)
	if err != nil {
		return target, err
	}
	if h.services.OPDS == nil {
		return target, errors.New("catalogue acquisition cannot promote this book")
	}
	_, err = h.services.OPDS.AcquireForBook(ctx, owner, target.ConnectionID, target.Language, bookID, target.Entry)
	return target, err
}

func analysisAcquisitionError(ctx context.Context, store Store, owner, bookID, bookTitle string, target cataloguesync.AcquisitionTarget, err error) string {
	connectionName, entryTitle := "catalogue connection", "this book"
	if strings.TrimSpace(target.Entry.Title) != "" {
		entryTitle = target.Entry.Title
	} else if strings.TrimSpace(bookTitle) != "" {
		entryTitle = bookTitle
	}
	if strings.TrimSpace(target.ConnectionID) != "" {
		connectionName = target.ConnectionID
	}
	if provider, ok := store.(catalogueAliasReader); ok {
		if alias, aliasErr := provider.GetBookCatalogEntryAlias(ctx, owner, bookID); aliasErr == nil {
			if connection, connectionErr := store.GetOpdsConnection(ctx, owner, alias.ConnectionID); connectionErr == nil {
				if strings.TrimSpace(connection.Name) != "" {
					connectionName = connection.Name
				}
			}
		}
	}
	return fmt.Sprintf("Could not acquire %q from connection %s: %s", entryTitle, connectionName, opdsErrorMessage(err))
}
