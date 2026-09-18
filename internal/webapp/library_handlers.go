package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"

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
	query, page, needsLanguage := parseMyBooksBrowseRequest(r.URL)
	if _, hasLanguage := r.URL.Query()["language"]; hasLanguage {
		http.Redirect(w, r, myBooksURL(query, page, needsLanguage), http.StatusSeeOther)
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
		ListMyBooksBrowse(context.Context, string, string, string, int, int) (persistence.MyBooksBrowseResult, error)
	}); ok {
		result, readErr := reader.ListMyBooksBrowse(r.Context(), u.ID, query, requestedLanguage, myBooksPageOffset(page), myBooksPageSize)
		err = readErr
		books = result.Items
		if activeLanguage == "" && !needsLanguage {
			books = nil
			result.Total = 0
		}
		browse = MyBooksBrowseState{
			Enabled:            true,
			Query:              query,
			Language:           activeLanguage,
			LanguageLabel:      activeLanguageLabel,
			NeedsLanguage:      needsLanguage,
			NeedsLanguageCount: needsLanguageCount(result.Counts),
			AllCount:           result.AllCount,
			ScopeTotal:         result.ScopeTotal,
			Total:              result.Total,
			Page:               page,
			PageCount:          myBooksPageCount(result.Total),
			TextNoMatch:        query != "" && result.Total == 0,
		}
		if err == nil && result.Total > 0 && myBooksPageOffset(page) >= result.Total {
			lastPage := myBooksPageCount(result.Total)
			http.Redirect(w, r, myBooksURL(query, lastPage, needsLanguage), http.StatusSeeOther)
			return
		}
		if err == nil && page > 1 && result.Total == 0 {
			lastPage := 1
			if query == "" && requestedLanguage != "" {
				lastPage = myBooksPageCount(result.ScopeTotal)
				if lastPage == 0 {
					lastPage = 1
				}
			}
			http.Redirect(w, r, myBooksURL(query, lastPage, needsLanguage), http.StatusSeeOther)
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
			books = append(books, domain.MyBook{Book: domain.Book{ID: source.Source.ID, OwnerID: source.Source.OwnerID, Title: canonicalBookTitle(source), LanguageState: domain.LanguageChosen, LanguageTag: source.Source.Language}, Acquired: &source})
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
		render(w, r, MyBooksResults(h.csrf(w, r), books, goalBookID, browse))
		return
	}
	render(w, r, MyBooksPage(u, h.csrf(w, r), books, r.URL.Query().Get("message"), r.URL.Query().Get("error"), goalBookID, len(connections) > 0, browse))
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
