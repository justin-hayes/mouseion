package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/library")
}
func (h *Handler) library(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	activeLanguage, activeLanguageLabel := activeStudyLanguageForContext(r.Context())
	goal, goalErr := h.services.Store.Goals.GetPrimaryGoal(r.Context(), u.ID, activeLanguage)
	if goalErr != nil {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksPage(u, h.csrf(w, r), nil, "", "My Books could not be loaded. Try refreshing the page.", "", false, MyBooksBrowseState{}))
		return
	}
	query, page, needsLanguage, disposition, history := parseMyBooksBrowseRequestWithHistory(r.URL)
	if _, hasLanguage := r.URL.Query()["language"]; hasLanguage {
		// #nosec G710 -- myBooksFilteredURL constructs only a local /library URL and escapes query values.
		http.Redirect(w, r, myBooksFilteredURL(query, page, needsLanguage, disposition), http.StatusSeeOther)
		return
	}
	requestedLanguage := activeLanguage
	if needsLanguage {
		requestedLanguage = domain.LanguageUnknown
	}
	var books []domain.MyBook
	var err error
	var browse MyBooksBrowseState
	if reader, ok := h.services.Store.Books.(interface {
		ListMyBooksBrowse(context.Context, string, string, string, string, bool, int, int) (persistence.MyBooksBrowseResult, error)
	}); ok {
		result, readErr := reader.ListMyBooksBrowse(r.Context(), u.ID, query, requestedLanguage, string(disposition), history, myBooksPageOffset(page), myBooksPageSize)
		err = readErr
		books = result.Items
		if activeLanguage == "" && !needsLanguage {
			books = nil
			result.Total = 0
		}
		browse = myBooksBrowseState(query, page, needsLanguage, disposition, activeLanguage, activeLanguageLabel, result)
		browse.History = history
		if err == nil && result.Total > 0 && myBooksPageOffset(page) >= result.Total {
			lastPage := myBooksPageCount(result.Total)
			http.Redirect(w, r, myBooksHistoryURL(query, lastPage, needsLanguage, disposition, history), http.StatusSeeOther)
			return
		}
		if err == nil && page > 1 && result.Total == 0 {
			lastPage := 1
			if query == "" && requestedLanguage != "" && disposition == "" {
				lastPage = myBooksPageCount(result.ScopeTotal)
				if lastPage == 0 {
					lastPage = 1
				}
			}
			http.Redirect(w, r, myBooksHistoryURL(query, lastPage, needsLanguage, disposition, history), http.StatusSeeOther)
			return
		}
	} else if reader, ok := h.services.Store.Books.(interface {
		ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	}); ok {
		books, err = reader.ListMyBooksWithEvidence(r.Context(), u.ID)
	} else {
		// Compatibility for lightweight stores used by older web tests. The
		// production PostgresStore always supplies the complete read model.
		var acquired []domain.SourceMaterialSummary
		acquired, err = h.services.Store.Books.ListSourceMaterials(r.Context(), u.ID)
		for _, source := range acquired {
			books = append(books, domain.MyBook{Book: domain.Book{ID: source.Source.ID, OwnerID: source.Source.OwnerID, Title: canonicalBookTitle(source), Author: source.BookAuthor, LanguageState: domain.LanguageChosen, LanguageTag: source.Source.Language}, Acquired: &source})
		}
	}
	if err != nil {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksPage(u, h.csrf(w, r), nil, "", "My Books could not be loaded. Try refreshing the page.", goal.BookID, false, browse))
		return
	}
	if err = h.annotateMyBooksWithJourney(r.Context(), u.ID, books); err != nil {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksPage(u, h.csrf(w, r), nil, "", "My Books could not be loaded. Try refreshing the page.", goal.BookID, false, browse))
		return
	}
	refreshableBookIDs, err := h.refreshableMyBookIDs(r.Context(), u.ID, books)
	if err != nil {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksPage(u, h.csrf(w, r), nil, "", "My Books could not be loaded. Try refreshing the page.", goal.BookID, false, browse))
		return
	}
	browse.RefreshableBookIDs = refreshableBookIDs
	connections, err := h.services.Store.Catalog.ListOpdsConnections(r.Context(), u.ID)
	if err != nil {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksPage(u, h.csrf(w, r), nil, "", "My Books could not be loaded. Try refreshing the page.", goal.BookID, false, browse))
		return
	}
	goalBookID := ""
	if goal.IsActive() {
		goalBookID = goal.BookID
	}
	if isHTMX(r) && browse.Enabled {
		render(w, r, MyBooksResults(h.csrf(w, r), books, browse))
		return
	}
	render(w, r, MyBooksPage(u, h.csrf(w, r), books, r.URL.Query().Get("message"), r.URL.Query().Get("error"), goalBookID, len(connections) > 0, browse))
}

func (h *Handler) markBookPreviouslyRead(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner, bookID := user(r).ID, r.PathValue("id")
	if _, err := h.services.Store.Books.GetBookDetail(r.Context(), owner, bookID); errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	importer, ok := h.services.Store.Books.(interface {
		ImportPreviouslyRead(context.Context, string, string) (domain.ReadingCompletion, error)
	})
	if !ok {
		fail(w, errors.New("reading history import is unavailable"))
		return
	}
	if _, err := importer.ImportPreviouslyRead(r.Context(), owner, bookID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/library?history=read&message="+url.QueryEscape("Previously read history recorded. Vocabulary was not changed."))
}

func myBooksBrowseState(query string, page int, needsLanguage bool, disposition domain.BookDisposition, language, languageLabel string, result persistence.MyBooksBrowseResult) MyBooksBrowseState {
	browse := MyBooksBrowseState{
		Enabled:            true,
		Query:              query,
		Disposition:        disposition,
		Language:           language,
		LanguageLabel:      languageLabel,
		NeedsLanguage:      needsLanguage,
		NeedsLanguageCount: needsLanguageCount(result.Counts),
		AllCount:           result.AllCount,
		ReadCount:          result.ReadCount,
		ScopeTotal:         result.ScopeTotal,
		Total:              result.Total,
		Page:               page,
		PageCount:          myBooksPageCount(result.Total),
		TextNoMatch:        query != "" && result.Total == 0,
	}
	for _, count := range result.DispositionCounts {
		switch count.Disposition {
		case domain.BookDispositionInbox:
			browse.InboxCount = count.Count
		case domain.BookDispositionToRead:
			browse.ToReadCount = count.Count
		case domain.BookDispositionSetAside:
			browse.SetAsideCount = count.Count
		}
	}
	return browse
}

func needsLanguageCount(counts []persistence.LanguageCount) int {
	for _, count := range counts {
		if count.Tag == domain.LanguageUnknown {
			return count.Count
		}
	}
	return 0
}

func (h *Handler) removeBookFromMyBooks(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	if err := h.services.Store.Books.RemoveBookFromMyBooks(r.Context(), u.ID, r.PathValue("id")); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/library?message="+url.QueryEscape("Book removed from My Books. Acquired content and history remain."))
}

func (h *Handler) moveBookToRead(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	if _, err := h.services.Store.Books.GetBookDetail(r.Context(), owner, bookID); errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	expectedRevision, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("expected_revision")), 10, 64)
	if err != nil {
		redirect(w, r, myBooksFilteredURL("", 1, false, domain.BookDispositionToRead)+"&error="+url.QueryEscape(journeyStaleMessage))
		return
	}
	action, err := h.addBookToReadingJourney(r.Context(), owner, "", bookID, expectedRevision)
	if err != nil {
		fail(w, err)
		return
	}
	if action.Error != "" || action.Message == journeyStaleMessage {
		errorMessage := action.Error
		if errorMessage == "" {
			errorMessage = action.Message
		}
		redirect(w, r, myBooksFilteredURL("", 1, false, domain.BookDispositionToRead)+"&error="+url.QueryEscape(errorMessage))
		return
	}
	message := action.Message
	if message == "" {
		message = "Book moved to To Read."
	}
	redirect(w, r, myBooksFilteredURL("", 1, false, domain.BookDispositionToRead)+"&message="+url.QueryEscape(message))
}

func (h *Handler) setBookAside(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	expectedRevision, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("expected_revision")), 10, 64)
	if err != nil {
		redirect(w, r, myBooksFilteredURL("", 1, false, domain.BookDispositionToRead)+"&error="+url.QueryEscape(journeyStaleMessage))
		return
	}
	bookID := r.PathValue("id")
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	language := strings.TrimSpace(detail.Book.LanguageTag)
	dispositions, ok := h.services.Store.Books.(persistence.BookDispositionStore)
	if !ok {
		fail(w, errors.New("book dispositions are unavailable"))
		return
	}
	if err = dispositions.SetBookAsideAtJourneyRevision(r.Context(), owner, language, detail.Book.ID, expectedRevision); err != nil {
		switch {
		case errors.Is(err, persistence.ErrJourneyStale):
			redirect(w, r, myBooksFilteredURL("", 1, false, domain.BookDispositionToRead)+"&error="+url.QueryEscape(journeyStaleMessage))
		case errors.Is(err, persistence.ErrBookIsPrimaryGoal):
			redirect(w, r, myBooksFilteredURL("", 1, false, domain.BookDispositionToRead)+"&error="+url.QueryEscape("The current current reading cannot be set aside. Finish or clear it first."))
		case errors.Is(err, persistence.ErrNotFound):
			http.NotFound(w, r)
		default:
			fail(w, err)
		}
		return
	}
	redirect(w, r, myBooksFilteredURL("", 1, false, domain.BookDispositionSetAside)+"&message="+url.QueryEscape("Book set aside. Acquired content and history remain."))
}
