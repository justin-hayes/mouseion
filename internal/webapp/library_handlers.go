package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	request := parseMyBooksLibraryRequest(r.URL)
	if request.hasLanguage {
		redirectMyBooksBrowse(w, r, myBooksScopedURL(request.query, request.page, request.needsLanguage, request.disposition, false, request.showHidden))
		return
	}
	requestedLanguage := activeLanguage
	if request.needsLanguage {
		requestedLanguage = domain.LanguageUnknown
	}
	result, err := browseMyBooks(r.Context(), h.services.Store.MyBooks, u.ID, request.query, requestedLanguage, string(request.disposition), request.history, request.showHidden, myBooksPageOffset(request.page), myBooksPageSize)
	if err != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	books := result.Items
	if activeLanguage == "" && !request.needsLanguage {
		books = nil
		result.Total = 0
	}
	browse := myBooksBrowseState(request.query, request.page, request.needsLanguage, request.disposition, activeLanguage, activeLanguageLabel, result)
	browse.History = request.history
	browse.ShowHidden = request.showHidden
	browse.HiddenCount = result.HiddenCount
	if page, redirect := myBooksRedirectPage(request, requestedLanguage, result); redirect {
		redirectMyBooksBrowse(w, r, myBooksScopedURL(request.query, page, request.needsLanguage, request.disposition, request.history, request.showHidden))
		return
	}
	refreshableBookIDs, connectionCount, err := h.loadLibraryBrowsingExtras(r.Context(), u.ID, books)
	if err != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	browse.RefreshableBookIDs = refreshableBookIDs
	currentReadingBookID := ""
	if currentReading.IsActive() {
		currentReadingBookID = currentReading.BookID
	}
	if isPartialHTMXRequest(r) {
		render(w, r, MyBooksResults(h.csrf(w, r), books, browse))
		return
	}
	render(w, r, MyBooksPage(u, h.csrf(w, r), books, r.URL.Query().Get("message"), r.URL.Query().Get("error"), currentReadingBookID, connectionCount > 0, browse))
}

// myBooksLibraryRequest is the My Books browse state the /library query asks for.
type myBooksLibraryRequest struct {
	query         string
	page          int
	needsLanguage bool
	disposition   domain.BookDisposition
	history       bool
	showHidden    bool
	hasLanguage   bool
}

func parseMyBooksLibraryRequest(rURL *url.URL) myBooksLibraryRequest {
	query, page, needsLanguage, disposition, history := parseMyBooksBrowseRequestWithHistory(rURL)
	return myBooksLibraryRequest{
		query:         query,
		page:          page,
		needsLanguage: needsLanguage,
		disposition:   disposition,
		history:       history,
		showHidden:    parseMyBooksShowHidden(rURL),
		hasLanguage:   rURL.Query().Has("language"),
	}
}

// myBooksRedirectPage clamps an out-of-range page. A page past the last page of
// a non-empty result, or any page beyond the first of an empty one, recovers to
// the bucket's own location; an unfiltered empty scope recovers to its last page.
// It reports the page to redirect to, and whether a redirect is needed at all.
func myBooksRedirectPage(request myBooksLibraryRequest, requestedLanguage string, result persistence.MyBooksBrowseResult) (int, bool) {
	rowTotal := myBooksRowCount(result)
	if rowTotal > 0 && myBooksPageOffset(request.page) >= rowTotal {
		return myBooksPageCount(rowTotal), true
	}
	if request.page > 1 && result.Total == 0 {
		if request.query == "" && requestedLanguage != "" && request.disposition == "" {
			if lastPage := myBooksPageCount(result.ScopeTotal); lastPage > 0 {
				return lastPage, true
			}
		}
		return 1, true
	}
	return 0, false
}

// loadLibraryBrowsingExtras runs the My Books reads that decorate the result
// rows: disposition annotations, Analysis update offers, refreshable IDs, and the OPDS
// connection count that decides whether acquisition is offered. It keeps their
// order, which the validated rows depend on.
func (h *Handler) loadLibraryBrowsingExtras(ctx context.Context, owner string, books []domain.MyBook) (map[string]bool, int, error) {
	if err := h.annotateMyBooksWithDisposition(ctx, owner, books); err != nil {
		return nil, 0, err
	}
	h.annotateAnalysisUpdates(ctx, owner, books)
	h.annotateSourceStructureUpdates(ctx, owner, books)
	refreshableBookIDs, err := h.refreshableMyBookIDs(ctx, owner, books)
	if err != nil {
		return nil, 0, err
	}
	connections, err := h.services.Store.Catalogs.ListOpdsConnections(ctx, owner)
	if err != nil {
		return nil, 0, err
	}
	return refreshableBookIDs, len(connections), nil
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
	// #nosec G710 -- SafeReturnPath rejects external destinations; My Books paths are local.
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
		if offset+myBooksPageSize >= myBooksRowCount(result) {
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
		BookTotal:          result.BookTotal,
		WorkTotal:          result.WorkTotal,
		RowCount:           result.RowCount,
		HasWorks:           result.HasWorks,
		Rows:               result.Rows,
		Page:               page,
		PageCount:          myBooksPageCount(myBooksRowCount(result)),
		TextNoMatch:        query != "" && result.Total == 0,
	}
	if browse.BookTotal == 0 {
		browse.BookTotal = result.Total
	}
	if browse.RowCount == 0 {
		browse.RowCount = result.Total
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

// myBooksRowCount is the rendered-row count that paging uses. Ordinary Books
// and grouped Work rows each count once.
func myBooksRowCount(result persistence.MyBooksBrowseResult) int {
	if result.RowCount > 0 {
		return result.RowCount
	}
	return result.Total
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
	// A classical Book is prepared on its dedicated queue when it moves to To
	// Read. The disposition change is the learner's intent; no EPUB acquisition
	// or analysis is involved.
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
	switch {
	case applied && !wasToRead && detail.ReadingUnit != nil && !detail.Classification().AnalysisInEffect():
		if prepared, ok := h.queueClassicalPreparation(r.Context(), owner, bookID); ok {
			message = prepared
		}
	case applied && !wasToRead && detail.ReadingUnit == nil && h.services.Analysis != nil:
		handle, target, title, acquisitionFailed, analysisErr := h.ensureToReadAnalysis(r.Context(), owner, bookID)
		if analysisErr != nil {
			logToReadAnalysisFailure(owner, bookID, acquisitionFailed, analysisErr)
			message = toReadAnalysisError(r.Context(), h.services.Store.Catalogs, owner, bookID, title, target, acquisitionFailed, analysisErr)
		} else {
			message = fmt.Sprintf("Book moved to To Read. Analysis job #%d submitted.", handle.DisplayNumber)
		}
	}
	redirect(w, r, myBooksScopedURL("", 1, false, domain.BookDispositionToRead, false, detail.Hidden)+"&message="+url.QueryEscape(message))
}

// queueClassicalPreparation submits the Book's ensure-once preparation job.
// It reports the learner-facing message and whether the seam was available.
func (h *Handler) queueClassicalPreparation(ctx context.Context, owner, bookID string) (string, bool) {
	if h.services.ClassicalPreparation == nil {
		return "", false
	}
	handle, err := h.services.ClassicalPreparation.SubmitClassicalPreparation(ctx, owner, bookID)
	if err != nil {
		log.Printf("mouseion: classical preparation submission failed for owner %s book %s: %s", owner, strconv.Quote(bookID), strconv.Quote(err.Error()))
		return "Book moved to To Read, but preparation could not start. Retry analysis from Reading when ready.", true
	}
	if handle.Complete {
		return "Book moved to To Read. Analysis is already current.", true
	}
	return "Book moved to To Read. " + fmt.Sprintf("Preparation job #%d submitted.", handle.JobID), true
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
