package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) journeyEntry(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("bookID"))
	if !ok {
		return
	}
	if detail.Acquired == nil {
		http.NotFound(w, r)
		return
	}

	summary := *detail.Acquired
	summary.BookID = detail.Book.ID
	summary.BookTitle = detail.Book.Title
	language := strings.TrimSpace(detail.Book.LanguageTag)
	if err := h.annotateBookWithJourneyLanguage(r.Context(), u.ID, language, &detail); err != nil {
		fail(w, err)
		return
	}
	if !detail.JourneyMember || summary.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(summary) {
		http.NotFound(w, r)
		return
	}
	summary.JourneyMember = detail.JourneyMember
	summary.JourneyGoal = detail.JourneyGoal
	summary.JourneyRevision = detail.JourneyRevision
	if !h.renderBookPage(w, r, u, summary, journeyBookPageOptions(summary), r.URL.Query().Get("message")) {
		return
	}
}

func (h *Handler) renderBookPage(w http.ResponseWriter, r *http.Request, u domain.User, summary domain.SourceMaterialSummary, page bookPageOptions, message string) bool {
	var coverage *domain.AnalysisCoverage
	statisticsUnavailable := false
	if summary.AnalysisStatus == "analyzed" && h.services.AnalysisInsights != nil {
		value, err := h.services.AnalysisInsights.Coverage(r.Context(), u.ID, summary.CorpusID)
		if errors.Is(err, analysisinsights.ErrStatisticsUnavailable) {
			statisticsUnavailable = true
		} else if err != nil {
			fail(w, err)
			return false
		} else {
			coverage = &value
		}
	}
	preparation, journeyAction, ok := h.currentBookPreparation(w, r, u.ID, summary)
	if !ok {
		return false
	}
	if preparation == nil {
		var studyErr error
		preparation, studyErr = h.currentVocabularyStudyPreparation(r.Context(), u.ID, summary, false)
		if studyErr != nil {
			fail(w, studyErr)
			return false
		}
	}
	history, historyErr := h.vocabularyStudyHistory(r.Context(), u.ID, summary.Source.ID, preparation)
	if historyErr != nil {
		fail(w, historyErr)
		return false
	}
	page.VocabularyStudyHistory = history
	render(w, r, BookPageWithOptions(u, h.csrf(w, r), summary, coverage, statisticsUnavailable, message, page, preparation, journeyAction))
	return true
}

func (h *Handler) vocabularyStudyHistory(ctx context.Context, owner, sourceMaterialID string, current *domain.DeckPreparation) ([]domain.DeckPreparation, error) {
	reader, ok := h.services.Store.(VocabularyStudyPreparationReader)
	if !ok {
		return nil, nil
	}
	preparations, err := reader.ListDeckPreparationsForSourceMaterial(ctx, owner, sourceMaterialID)
	if err != nil {
		return nil, err
	}
	currentID := ""
	if current != nil {
		currentID = current.ID
	}
	history := make([]domain.DeckPreparation, 0, len(preparations))
	for _, preparation := range preparations {
		if preparation.ID == currentID || preparation.VocabularyStudyStatus() == domain.VocabularyStudyNotStarted {
			continue
		}
		history = append(history, preparation)
	}
	return history, nil
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

func (h *Handler) refreshableMyBookIDs(ctx context.Context, owner string, books []domain.MyBook) (map[string]bool, error) {
	refreshable := make(map[string]bool)
	for _, book := range books {
		if book.EvidenceState() != domain.BookNotAcquired {
			continue
		}
		eligible, err := h.bookRefreshEligible(ctx, owner, book.Book.ID)
		if err != nil {
			return nil, err
		}
		if eligible {
			refreshable[book.Book.ID] = true
		}
	}
	return refreshable, nil
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
		row := domain.MyBook{Book: result.Book}
		if err := h.annotateBookWithJourney(r.Context(), u.ID, &row); err != nil {
			fail(w, err)
			return
		}
		refreshEligible, err := h.bookRefreshEligible(r.Context(), u.ID, row.Book.ID)
		if err != nil {
			fail(w, err)
			return
		}
		goalBookID := ""
		if row.JourneyGoal {
			goalBookID = row.Book.ID
		}
		render(w, r, MyBookRow(h.csrf(w, r), row, goalBookID, refreshEligible, message))
		return
	}
	redirect(w, r, "/library?message="+url.QueryEscape(message))
}

func (h *Handler) renderBookRefreshFailure(w http.ResponseWriter, r *http.Request, u domain.User, bookID string) {
	message := "Metadata could not be refreshed. Check the connection and try again."
	if isHTMX(r) {
		book, ok := h.bookDetail(w, r, u.ID, bookID)
		if !ok {
			return
		}
		if err := h.annotateBookWithJourney(r.Context(), u.ID, &book); err != nil {
			fail(w, err)
			return
		}
		goalBookID := ""
		if book.JourneyGoal {
			goalBookID = book.Book.ID
		}
		render(w, r, MyBookRow(h.csrf(w, r), book, goalBookID, false, message))
		return
	}
	redirect(w, r, "/library?message="+url.QueryEscape(message))
}

func refreshMessage(result cataloguesync.RefreshResult) string {
	switch {
	case result.Updated:
		return "Metadata refreshed."
	case result.Missing:
		return "The catalog entry is no longer available. Your book and its metadata are unchanged."
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
	if !bookHasCompletedAnalysis(book) {
		http.NotFound(w, r)
		return
	}
	runID := r.PathValue("runID")
	if book.AnalysisRunID != runID {
		http.NotFound(w, r)
		return
	}
	if err := h.annotateBookWithJourneyLanguage(r.Context(), u.ID, detail.Book.LanguageTag, &detail); err != nil {
		fail(w, err)
		return
	}
	if !detail.JourneyMember {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, journeyEntryURL(detail.Book.ID), http.StatusSeeOther)
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
	preparation, err = h.attachVocabularyStudyPreparation(r.Context(), owner, book, preparation)
	if err != nil {
		fail(w, err)
		return nil, journeyAction, false
	}
	if preparation.State == domain.DeckPreparationReady {
		journeyAction, err = h.deckJourneyAction(r.Context(), owner, preparation.ID, preparation.SourceMaterialID)
		if err != nil {
			fail(w, err)
			return nil, journeyAction, false
		}
	}
	return &preparation, journeyAction, true
}

func (h *Handler) acquireBookForJourneyContext(ctx context.Context, owner, bookID string) (cataloguesync.AcquisitionTarget, error) {
	provider, ok := h.services.CatalogueSync.(CatalogueAcquisitionTargetProvider)
	if !ok {
		return cataloguesync.AcquisitionTarget{}, errors.New("catalog acquisition is unavailable")
	}
	target, err := provider.FindAcquisitionTarget(ctx, owner, bookID)
	if err != nil {
		return target, err
	}
	if h.services.OPDS == nil {
		return target, errors.New("catalog acquisition cannot promote this book")
	}
	_, err = h.services.OPDS.AcquireForBook(ctx, owner, target.ConnectionID, target.Language, bookID, target.Entry)
	return target, err
}

func journeyAcquisitionError(ctx context.Context, store Store, owner, bookID, bookTitle string, target cataloguesync.AcquisitionTarget, err error) string {
	connectionName, entryTitle := "catalog connection", "this book"
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
