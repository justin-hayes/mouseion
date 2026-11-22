package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/readingintent"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

func browseMyBooks(ctx context.Context, store MyBooksStore, owner, query, language, disposition string, history bool, offset, limit int) (domain.MyBooksBrowseResult, error) {
	return store.ListMyBooksBrowse(ctx, owner, query, language, disposition, history, offset, limit)
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
		redirectMyBooksBrowse(w, r, myBooksScopedURL(request.query, request.page, request.needsLanguage, request.disposition, false))
		return
	}
	requestedLanguage := activeLanguage
	if request.needsLanguage {
		requestedLanguage = domain.LanguageUnknown
	}
	result, err := browseMyBooks(r.Context(), h.services.Store.MyBooks, u.ID, request.query, requestedLanguage, string(request.disposition), request.history, myBooksPageOffset(request.page), myBooksPageSize)
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
	if page, redirect := myBooksRedirectPage(request, requestedLanguage, result); redirect {
		redirectMyBooksBrowse(w, r, myBooksScopedURL(request.query, page, request.needsLanguage, request.disposition, request.history))
		return
	}
	connectionCount, err := h.loadLibraryBrowsingExtras(r.Context(), u.ID, books)
	if err != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	currentReadingBookID := ""
	if currentReading.IsActive() {
		currentReadingBookID = currentReading.BookID
		currentWorkBook, hasCurrentWorkBook, currentErr := h.currentWorkBookForMyBooks(r.Context(), u.ID, currentReading.BookID, browse.Rows)
		if currentErr != nil {
			h.renderMyBooksFailure(w, r, u)
			return
		}
		attachCurrentWorkBook(browse.Rows, currentWorkBook, hasCurrentWorkBook)
	}
	if isPartialHTMXRequest(r) {
		render(w, r, MyBooksResults(h.csrf(w, r), books, browse, currentReadingBookID))
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
		hasLanguage:   rURL.Query().Has("language"),
	}
}

// myBooksRedirectPage clamps an out-of-range page. A page past the last page of
// a non-empty result, or any page beyond the first of an empty one, recovers to
// the bucket's own location; an unfiltered empty scope recovers to its last page.
// It reports the page to redirect to, and whether a redirect is needed at all.
func myBooksRedirectPage(request myBooksLibraryRequest, requestedLanguage string, result domain.MyBooksBrowseResult) (int, bool) {
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
// rows: disposition annotations, Analysis update offers, and the OPDS
// connection count that decides whether acquisition is offered. It keeps their
// order, which the validated rows depend on. Books are the page's visible cells,
// shared by pointer with the grouped rows their Work offers cover.
func (h *Handler) loadLibraryBrowsingExtras(ctx context.Context, owner string, books []domain.MyBook) (int, error) {
	if err := h.annotateMyBooksWithDisposition(ctx, owner, books); err != nil {
		return 0, err
	}
	h.annotateAnalysisUpdates(ctx, owner, books)
	h.annotateSourceStructureUpdates(ctx, owner, books)
	connections, err := h.services.Store.Catalogs.ListOpdsConnections(ctx, owner)
	if err != nil {
		return 0, err
	}
	return len(connections), nil
}

// currentWorkBookForMyBooks loads the current Reading's Book with its Analysis
// update offer. A grouped Work row names it even when a filter keeps its cell
// out of the row.
func (h *Handler) currentWorkBookForMyBooks(ctx context.Context, owner, bookID string, rows []domain.MyBooksBrowseRow) (domain.MyBook, bool, error) {
	if h.services.AnalysisUpdates == nil || !myBooksHasGroupedWork(rows) {
		return domain.MyBook{}, false, nil
	}
	book, err := h.services.Store.Reading.GetBookDetail(ctx, owner, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.MyBook{}, false, nil
	}
	if err != nil {
		return domain.MyBook{}, false, err
	}
	if book.ReadingUnit == nil || strings.TrimSpace(book.ReadingUnit.WorkKey) == "" {
		return domain.MyBook{}, false, nil
	}
	// GetBookDetail does not carry the current-Reading role, and this Book is
	// here precisely because the caller resolved it as the current Reading.
	book.IsCurrentReading = true
	annotated := []domain.MyBook{book}
	h.annotateAnalysisUpdates(ctx, owner, annotated)
	return annotated[0], true, nil
}

// attachCurrentWorkBook points the matching grouped Work row at the current
// Reading's Book. The offer names and skips it; it is never queued.
func attachCurrentWorkBook(rows []domain.MyBooksBrowseRow, current domain.MyBook, ok bool) {
	if !ok || current.ReadingUnit == nil {
		return
	}
	for i := range rows {
		if rows[i].Grouped && rows[i].WorkKey == current.ReadingUnit.WorkKey {
			rows[i].CurrentWorkBook = &current
			return
		}
	}
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

const previouslyReadIneligibleRefusal = "Only an unstarted To Read Book can be marked previously read."

func (h *Handler) markBookPreviouslyRead(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner, bookID := user(r).ID, r.PathValue("id")
	if err := h.services.Store.MyBooks.MarkBookPreviouslyRead(r.Context(), owner, bookID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, domain.ErrBookIsCurrentReading) {
			redirectMyBooksBrowse(w, r, "/library?error="+url.QueryEscape(catalogueReadCurrentReadingRefusal))
			return
		}
		if errors.Is(err, domain.ErrPreviouslyReadIneligible) {
			redirectMyBooksBrowse(w, r, "/library?error="+url.QueryEscape(previouslyReadIneligibleRefusal))
			return
		}
		fail(w, err)
		return
	}
	book, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, bookID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
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
	location := myBookBucketURL(book, page)
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
		result, err := browseMyBooks(ctx, store, owner, "", language, string(disposition), history, offset, myBooksPageSize)
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

func myBooksBrowseState(query string, page int, needsLanguage bool, disposition domain.BookDisposition, language, languageLabel string, result domain.MyBooksBrowseResult) MyBooksBrowseState {
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
func myBooksRowCount(result domain.MyBooksBrowseResult) int {
	if result.RowCount > 0 {
		return result.RowCount
	}
	return result.Total
}

func needsLanguageCount(counts []domain.LanguageCount) int {
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
	owner, bookID := user(r).ID, r.PathValue("id")
	returnTo := myBooksReturnURL(r.FormValue("return_to"))
	err := h.services.Store.MyBooks.RemoveBookFromMyBooks(r.Context(), owner, bookID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, domain.ErrBookIsCurrentReading):
		myBooksRedirectWithNotice(w, r, returnTo, "This Book is your current reading. Finish or end the reading before removing it from My Books.")
		return
	case err != nil:
		fail(w, err)
		return
	}
	myBooksRedirectWithNotice(w, r, returnTo, "Removed from My Books. Its analysis, history and Known vocabulary are unchanged, and the Catalog can add it back.")
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
	intent, err := h.readingIntentModule()
	if err != nil {
		fail(w, err)
		return
	}
	outcome, err := intent.Move(r.Context(), owner, bookID, expectedRevision)
	if err != nil {
		fail(w, err)
		return
	}
	if outcome.Kind == readingintent.OutcomeNotFound {
		http.NotFound(w, r)
		return
	}
	message := moveToReadMessage(r.Context(), h.services.Store.Catalogs, owner, bookID, outcome)
	queryParam := "message"
	// A stale form is the only move outcome reported as an error notice.
	if outcome.Kind == readingintent.OutcomeStaleRevision {
		queryParam = "error"
	}
	redirect(w, r, myBooksScopedURL("", 1, false, domain.BookDispositionToRead, false)+"&"+queryParam+"="+url.QueryEscape(message))
}

// moveToReadMessage maps a reading-intent outcome to the learner-facing
// sentence. The module returns no text of its own; every sentence here predates
// it and must not change.
func moveToReadMessage(ctx context.Context, catalog CatalogsStore, owner, bookID string, outcome readingintent.Outcome) string {
	switch outcome.Kind {
	case readingintent.OutcomeStaleRevision:
		return "This Book's decision changed in another tab. No changes were made; refresh My Books and try again."
	case readingintent.OutcomeAlreadyCurrent:
		return "Book moved to To Read. Analysis is already current."
	case readingintent.OutcomeJobQueued:
		if outcome.Route == readingintent.RouteClassical {
			return fmt.Sprintf("Book moved to To Read. Preparation job #%d submitted.", outcome.JobNumber)
		}
		return fmt.Sprintf("Book moved to To Read. Analysis job #%d submitted.", outcome.JobNumber)
	case readingintent.OutcomeLegacyRows:
		return classical.LegacyRowsReason
	case readingintent.OutcomeWithdrawnDefinition:
		return classicalWithdrawnRefusal
	case readingintent.OutcomeSourceRefused:
		return classicalSourceMismatchRefusal
	case readingintent.OutcomeCouldNotStart:
		if outcome.Route == readingintent.RouteClassical {
			return "Book moved to To Read, but preparation could not start. Retry analysis from Reading when ready."
		}
		return "Book moved to To Read, but analysis could not start. Retry analysis from Reading when ready."
	case readingintent.OutcomeAcquisitionFailed:
		return "Book moved to To Read, but " + readingAcquisitionError(ctx, catalog, owner, bookID, outcome.Title, outcome.Target, outcome.Err) + " The To Read status is retained; analysis is unavailable until the current EPUB can be acquired."
	case readingintent.OutcomeNoUsableEPUBUnits:
		return "Book moved to To Read, but the current source has no usable EPUB units. The To Read status is retained; analysis is unavailable."
	case readingintent.OutcomePreparationUnavailable:
		return "Book moved to To Read."
	default:
		// A real move with nothing to submit, and an unchanged Book, keep the
		// familiar confirmation.
		return "Book moved to To Read."
	}
}

var errReadingIntentAnalysisUnavailable = errors.New("reading intent: To Read analysis is not configured")

// unavailableClassicalPreparation stands in for a deployment with no
// preparation seam. The move still changes disposition and starts no job.
type unavailableClassicalPreparation struct{}

func (unavailableClassicalPreparation) SubmitClassicalPreparation(context.Context, string, string) (analysis.ClassicalPreparationHandle, error) {
	return analysis.ClassicalPreparationHandle{}, readingintent.ErrPreparationUnavailable
}

// unavailableAnalysis stands in for a deployment with no analysis seam.
type unavailableAnalysis struct{}

func (unavailableAnalysis) SubmitToReadBookAnalysis(context.Context, string, string, string) (analysis.Handle, error) {
	return analysis.Handle{}, errReadingIntentAnalysisUnavailable
}

// readingIntentModule returns the module the server composed. A handler without
// one, as in a test that replaces a service before moving, composes the real
// module over its live services so the replacement is observed.
func (h *Handler) readingIntentModule() (*readingintent.Module, error) {
	if h.services.ReadingIntent != nil {
		return h.services.ReadingIntent, nil
	}
	classicalPreparation := h.services.ClassicalPreparation
	if classicalPreparation == nil {
		classicalPreparation = unavailableClassicalPreparation{}
	}
	var analysisService readingintent.Analysis = unavailableAnalysis{}
	if h.services.Analysis != nil {
		analysisService = h.services.Analysis
	}
	var acquisitionTargets func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
	if h.services.CatalogueSync != nil {
		acquisitionTargets = h.services.CatalogueSync.FindAcquisitionTarget
	}
	var bookAcquirer func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
	if h.services.OPDS != nil {
		bookAcquirer = h.services.OPDS.AcquireForBook
	}
	preparation, err := readingintent.NewPreparation(readingintent.PreparationConfig{
		Books:        h.services.Store.MyBooks,
		Classical:    classicalPreparation,
		Analysis:     analysisService,
		FindTarget:   acquisitionTargets,
		Acquire:      bookAcquirer,
		Acquisitions: h.services.Store.MyBooks,
	})
	if err != nil {
		return nil, err
	}
	return readingintent.New(readingintent.Config{
		Books:           h.services.Store.MyBooks,
		CurrentReadings: h.services.Store.Reading,
		Dispositions:    h.services.Store.MyBooks,
		Preparation:     preparation,
	})
}

func expectedDispositionRevision(r *http.Request) (int64, bool) {
	if err := r.ParseForm(); err != nil {
		return 0, false
	}
	revision, err := strconv.ParseInt(r.FormValue("expected_revision"), 10, 64)
	return revision, err == nil && revision > 0
}
