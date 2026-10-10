package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

func browseMyBooks(ctx context.Context, store MyBooksStore, owner, query, language, disposition string, history, showHidden bool, offset, limit int) (persistence.MyBooksBrowseResult, error) {
	return store.ListMyBooksBrowseWithVisibility(ctx, owner, query, language, disposition, history, showHidden, offset, limit)
}

const myBooksLoadFailureMessage = "My Books could not be loaded. Try refreshing the page."

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/library")
}
func (h *Handler) library(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "HX-Request-Type")
	u := user(r)
	activeLanguage, activeLanguageLabel := activeStudyLanguageForContext(r.Context())
	currentReading, currentReadingErr := h.services.Store.MyBooks.GetCurrentReading(r.Context(), u.ID, activeLanguage)
	if currentReadingErr != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	query, page, needsLanguage, disposition, history := parseMyBooksBrowseRequestWithHistory(r.URL)
	showHidden := parseMyBooksShowHidden(r.URL)
	if _, hasLanguage := r.URL.Query()["language"]; hasLanguage {
		// #nosec G710 -- myBooksScopedURL constructs only a local /library URL and escapes query values.
		redirectMyBooksBrowse(w, r, myBooksScopedURL(query, page, needsLanguage, disposition, false, showHidden))
		return
	}
	requestedLanguage := activeLanguage
	if needsLanguage {
		requestedLanguage = domain.LanguageUnknown
	}
	result, err := browseMyBooks(r.Context(), h.services.Store.MyBooks, u.ID, query, requestedLanguage, string(disposition), history, showHidden, myBooksPageOffset(page), myBooksPageSize)
	if err != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	books := result.Items
	if activeLanguage == "" && !needsLanguage {
		books = nil
		result.Total = 0
	}
	browse := myBooksBrowseState(query, page, needsLanguage, disposition, activeLanguage, activeLanguageLabel, result)
	browse.History = history
	browse.ShowHidden = showHidden
	browse.HiddenCount = result.HiddenCount
	if result.Total > 0 && myBooksPageOffset(page) >= result.Total {
		lastPage := myBooksPageCount(result.Total)
		redirectMyBooksBrowse(w, r, myBooksScopedURL(query, lastPage, needsLanguage, disposition, history, showHidden))
		return
	}
	if page > 1 && result.Total == 0 {
		lastPage := 1
		if query == "" && requestedLanguage != "" && disposition == "" {
			lastPage = myBooksPageCount(result.ScopeTotal)
			if lastPage == 0 {
				lastPage = 1
			}
		}
		redirectMyBooksBrowse(w, r, myBooksScopedURL(query, lastPage, needsLanguage, disposition, history, showHidden))
		return
	}
	if err = h.annotateMyBooksWithDisposition(r.Context(), u.ID, books); err != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	refreshableBookIDs, err := h.refreshableMyBookIDs(r.Context(), u.ID, books)
	if err != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	browse.RefreshableBookIDs = refreshableBookIDs
	connections, err := h.services.Store.Catalogs.ListOpdsConnections(r.Context(), u.ID)
	if err != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	currentReadingBookID := ""
	if currentReading.IsActive() {
		currentReadingBookID = currentReading.BookID
	}
	if isPartialHTMXRequest(r) {
		render(w, r, MyBooksResults(h.csrf(w, r), books, browse))
		return
	}
	render(w, r, MyBooksPage(u, h.csrf(w, r), books, r.URL.Query().Get("message"), r.URL.Query().Get("error"), currentReadingBookID, len(connections) > 0, browse))
}

// redirectMyBooksBrowse preserves ordinary browser redirects while asking
// HTMX to navigate the whole page instead of swapping a followed 3xx document
// into the results region.
func redirectMyBooksBrowse(w http.ResponseWriter, r *http.Request, path string) {
	if isPartialHTMXRequest(r) {
		w.Header().Set("Hx-Redirect", webauth.SafeReturnPath(path))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, webauth.SafeReturnPath(path), http.StatusSeeOther)
}

func (h *Handler) renderMyBooksFailure(w http.ResponseWriter, r *http.Request, u domain.User) {
	retryURL := "/library"
	if r.URL != nil {
		retryURL = (&url.URL{Path: "/library", RawQuery: r.URL.RawQuery}).String()
	}
	if isPartialHTMXRequest(r) {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksResultsError(myBooksLoadFailureMessage, retryURL))
		return
	}
	renderStatus(w, r, http.StatusInternalServerError, MyBooksFailurePage(u, h.csrf(w, r), myBooksLoadFailureMessage, retryURL))
}

func (h *Handler) markBookPreviouslyRead(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner, bookID := user(r).ID, r.PathValue("id")
	if _, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, bookID); errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	if _, err := h.services.Store.MyBooks.ImportPreviouslyRead(r.Context(), owner, bookID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	book, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	activeLanguage, _ := activeStudyLanguageForContext(r.Context())
	page, err := myBooksPageForBook(r.Context(), h.services.Store.MyBooks, owner, activeLanguage, book)
	if err != nil {
		fail(w, err)
		return
	}
	location := myBookVisibleBucketURL(book, page)
	separator := "?"
	if strings.Contains(location, "?") {
		separator = "&"
	}
	location += separator + "message=" + url.QueryEscape("Previously read history recorded. Vocabulary was not changed.")
	redirect(w, r, location)
}

func myBooksPageForBook(ctx context.Context, store MyBooksStore, owner, language string, book domain.MyBook) (int, error) {
	bucket := book.WorkflowBucket()
	disposition, _ := bucket.PersistedDisposition()
	if bucket == domain.MyBookBucketCurrentReading {
		disposition = domain.BookDispositionToRead
	}
	history := bucket == domain.MyBookBucketRead
	for offset := 0; ; offset += myBooksPageSize {
		result, err := browseMyBooks(ctx, store, owner, "", language, string(disposition), history, book.Hidden, offset, myBooksPageSize)
		if err != nil {
			return 0, err
		}
		for _, item := range result.Items {
			if item.Book.ID == book.Book.ID {
				return offset/myBooksPageSize + 1, nil
			}
		}
		if offset+myBooksPageSize >= result.Total {
			return 0, fmt.Errorf("book %q is not present in its visible My Books bucket", book.Book.ID)
		}
	}
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

func (h *Handler) moveBookToRead(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	expectedRevision, ok := expectedDispositionRevision(r)
	if !ok {
		redirect(w, r, myBooksFilteredURL("", 1, false, domain.BookDispositionToRead)+"&error="+url.QueryEscape("This My Books form is out of date. Refresh My Books and try again."))
		return
	}
	detail, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	wasToRead := detail.Disposition == domain.BookDispositionToRead
	applied, transitionErr := h.services.Store.MyBooks.TransitionBookDisposition(r.Context(), owner, bookID, expectedRevision, domain.BookDispositionToRead)
	if transitionErr != nil {
		err = transitionErr
		if errors.Is(err, persistence.ErrStaleBookDisposition) {
			redirect(w, r, myBooksScopedURL("", 1, false, domain.BookDispositionToRead, false, detail.Hidden)+"&error="+url.QueryEscape("This Book's decision changed in another tab. No changes were made; refresh My Books and try again."))
			return
		}
		fail(w, err)
		return
	}
	message := "Book moved to To Read."
	if applied && !wasToRead && h.services.Analysis != nil {
		handle, target, title, acquisitionFailed, analysisErr := h.ensureToReadAnalysis(r.Context(), owner, bookID)
		if analysisErr != nil {
			message = toReadAnalysisError(r.Context(), h.services.Store.Catalogs, owner, bookID, title, target, acquisitionFailed, analysisErr)
		} else {
			message = fmt.Sprintf("Book moved to To Read. Analysis job #%d submitted.", handle.DisplayNumber)
		}
	}
	redirect(w, r, myBooksScopedURL("", 1, false, domain.BookDispositionToRead, false, detail.Hidden)+"&message="+url.QueryEscape(message))
}

func expectedDispositionRevision(r *http.Request) (int64, bool) {
	if err := r.ParseForm(); err != nil {
		return 0, false
	}
	revision, err := strconv.ParseInt(r.FormValue("expected_revision"), 10, 64)
	return revision, err == nil && revision > 0
}

func (h *Handler) hideBook(w http.ResponseWriter, r *http.Request) {
	h.setBookVisibility(w, r, true)
}

func (h *Handler) unhideBook(w http.ResponseWriter, r *http.Request) {
	h.setBookVisibility(w, r, false)
}

// setBookVisibility records the desired Hidden value against the revision the
// form rendered. It writes only visibility: Start, End, Finish, disposition,
// reservations, and history are deliberately not consulted or changed, so a
// Hidden Current reading stays in Reading and Hide never ends it.
func (h *Handler) setBookVisibility(w http.ResponseWriter, r *http.Request, hidden bool) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner, bookID := user(r).ID, r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	returnTo := myBooksReturnURL(r.FormValue("return_to"))
	withNotice := func(key, text string) string {
		separator := "?"
		if strings.Contains(returnTo, "?") {
			separator = "&"
		}
		return returnTo + separator + key + "=" + url.QueryEscape(text)
	}
	expectedRevision, err := strconv.ParseInt(r.FormValue("expected_visibility_revision"), 10, 64)
	if err != nil || expectedRevision < 0 {
		redirect(w, r, withNotice("error", "This My Books form is out of date. Refresh My Books and try again."))
		return
	}
	if _, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, bookID); errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	if _, err := h.services.Store.MyBooks.SetBookHidden(r.Context(), owner, bookID, expectedRevision, hidden); err != nil {
		switch {
		case errors.Is(err, persistence.ErrStaleBookVisibility):
			redirect(w, r, withNotice("error", "This Book's visibility changed in another tab. No changes were made; refresh My Books and try again."))
		case errors.Is(err, persistence.ErrNotFound):
			http.NotFound(w, r)
		default:
			fail(w, err)
		}
		return
	}
	if hidden {
		redirect(w, r, withNotice("message", "Book hidden. Use Show hidden books to find it again. Reading intent and history are unchanged."))
		return
	}
	redirect(w, r, withNotice("message", "Book unhidden. Reading intent and history are unchanged."))
}
