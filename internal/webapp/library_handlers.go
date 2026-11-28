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
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/readingintent"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

const myBooksLoadFailureMessage = "My Books could not be loaded. Try refreshing the page."

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, studyLanguagePath(pageKindMyBooks, shellViewFromContext(r.Context()).lastLanguage()))
}
func (h *Handler) library(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "HX-Request-Type")
	u := user(r)
	activeLanguage, activeLanguageLabel := activeStudyLanguageForContext(r.Context())
	request, ok := parseMyBooksBrowseRequest(r.URL)
	if !ok {
		http.NotFound(w, r)
		return
	}
	result, err := h.listMyBooksPage(r.Context(), u.ID, request, activeLanguage)
	if err != nil {
		h.renderMyBooksFailure(w, r, u)
		return
	}
	books := result.Items
	if activeLanguage == "" {
		books = nil
		result.Total = 0
	}
	browse := myBooksBrowseState(request.query, request.page, request.disposition, activeLanguage, activeLanguageLabel, result)
	browse.History = request.history
	browse.NeedsAction = request.needsAction
	browse.MostReadable = request.mostReadable
	if page, redirect := myBooksRedirectPage(request, result); redirect {
		redirectMyBooksBrowse(w, r, myBooksFilteredURL(activeLanguage, request.query, page, request.disposition, request.history, request.needsAction, request.mostReadable))
		return
	}
	if !request.needsAction {
		if err = h.loadLibraryBrowsingExtras(r.Context(), u.ID, books); err != nil {
			h.renderMyBooksFailure(w, r, u)
			return
		}
	}
	if isPartialHTMXRequest(r) {
		render(w, r, MyBooksResults(h.csrf(w, r), books, browse))
		return
	}
	if activeLanguage != "" {
		readingView, viewErr := h.buildReadingView(r.Context(), u.ID, activeLanguage)
		if viewErr != nil {
			h.renderMyBooksFailure(w, r, u)
			return
		}
		browse.CurrentReading = readingView.CurrentReading
	}
	render(w, r, MyBooksPage(u, h.csrf(w, r), books, r.URL.Query().Get("message"), r.URL.Query().Get("error"), browse))
}

// listMyBooksPage reads the requested page. The needs-action filter is derived
// from evidence the store pages cannot filter on, such as pending Analysis
// updates, and the most-readable sort orders by the Mature coverage the store
// decorates each row with; both read the whole tab, keep the Books that carry
// the request's order, and page those. Nothing about either is stored.
func (h *Handler) listMyBooksPage(ctx context.Context, owner string, request myBooksLibraryRequest, language string) (domain.MyBooksBrowseResult, error) {
	store := h.services.Store.MyBooks
	if !request.needsAction && !request.mostReadable {
		return store.ListMyBooksBrowse(ctx, owner, request.query, language, string(request.disposition), request.history, myBooksPageOffset(request.page), myBooksPageSize)
	}
	kept, result, err := h.listMyBooksWholeTab(ctx, owner, request, language)
	if err != nil {
		return domain.MyBooksBrowseResult{}, err
	}
	if request.mostReadable {
		domain.SortMyBooksMostReadableFirst(kept)
	}
	result.Total = len(kept)
	result.Items = myBooksKeptPageItems(kept, request.page)
	return result, nil
}

// listMyBooksWholeTab reads every page of the requested browse tab and returns
// the Books it keeps, with the store's own aggregate result. The needs-action
// filter decorates each page before deciding, because its evidence — such as a
// pending Analysis update — is not something the store pages on.
func (h *Handler) listMyBooksWholeTab(ctx context.Context, owner string, request myBooksLibraryRequest, language string) ([]domain.MyBook, domain.MyBooksBrowseResult, error) {
	store := h.services.Store.MyBooks
	var kept []domain.MyBook
	var result domain.MyBooksBrowseResult
	for offset := 0; ; offset += myBooksPageSize {
		page, err := store.ListMyBooksBrowse(ctx, owner, request.query, language, string(request.disposition), request.history, offset, myBooksPageSize)
		if err != nil {
			return nil, result, err
		}
		result = page
		pageKept, err := h.keptMyBooksFromPage(ctx, owner, request, page.Items)
		if err != nil {
			return nil, result, err
		}
		kept = append(kept, pageKept...)
		if len(page.Items) == 0 || offset+myBooksPageSize >= page.Total {
			return kept, result, nil
		}
	}
}

// keptMyBooksFromPage keeps one browse page's Books: only those with a status
// line when the needs-action filter is on, every one of them otherwise.
func (h *Handler) keptMyBooksFromPage(ctx context.Context, owner string, request myBooksLibraryRequest, books []domain.MyBook) ([]domain.MyBook, error) {
	if !request.needsAction {
		return books, nil
	}
	if err := h.loadLibraryBrowsingExtras(ctx, owner, books); err != nil {
		return nil, err
	}
	needing := make([]domain.MyBook, 0, len(books))
	for _, book := range books {
		if _, needs := myBookNeedsAction(book); needs {
			needing = append(needing, book)
		}
	}
	return needing, nil
}

// myBooksKeptPageItems slices the kept Books for the requested page.
func myBooksKeptPageItems(kept []domain.MyBook, page int) []domain.MyBook {
	if start := myBooksPageOffset(page); start < len(kept) {
		return kept[start:min(start+myBooksPageSize, len(kept))]
	}
	return nil
}

// myBooksLibraryRequest is the My Books browse state the /library query asks for.
type myBooksLibraryRequest struct {
	query        string
	page         int
	disposition  domain.BookDisposition
	history      bool
	needsAction  bool
	mostReadable bool
}

// myBooksRedirectPage clamps an out-of-range page. A page past the last page of
// a non-empty result, or any page beyond the first of an empty one, recovers to
// the tab's own first page.
func myBooksRedirectPage(request myBooksLibraryRequest, result domain.MyBooksBrowseResult) (int, bool) {
	rowTotal := result.Total
	if rowTotal > 0 && myBooksPageOffset(request.page) >= rowTotal {
		return myBooksPageCount(rowTotal), true
	}
	if request.page > 1 && result.Total == 0 {
		return 1, true
	}
	return 0, false
}

// loadLibraryBrowsingExtras runs the My Books reads that decorate the result
// rows: disposition annotations and Analysis update offers. It keeps their
// order, which the validated rows depend on.
func (h *Handler) loadLibraryBrowsingExtras(ctx context.Context, owner string, books []domain.MyBook) error {
	if err := h.annotateMyBooksWithDisposition(ctx, owner, books); err != nil {
		return err
	}
	h.annotateAnalysisUpdates(ctx, owner, books)
	h.annotateSourceStructureUpdates(ctx, owner, books)
	return nil
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
	language, _ := activeStudyLanguageForContext(r.Context())
	retryURL := myBooksPath(language)
	if r.URL != nil {
		retryURL = (&url.URL{Path: retryURL, RawQuery: r.URL.RawQuery}).String()
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
	language := h.bookStudyLanguage(r.Context(), owner, bookID)
	if err := h.services.Store.MyBooks.MarkBookPreviouslyRead(r.Context(), owner, bookID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, domain.ErrBookIsCurrentReading) {
			redirectMyBooksBrowse(w, r, myBooksPath(language)+"?error="+url.QueryEscape(catalogueReadCurrentReadingRefusal))
			return
		}
		if errors.Is(err, domain.ErrPreviouslyReadIneligible) {
			redirectMyBooksBrowse(w, r, myBooksPath(language)+"?error="+url.QueryEscape(previouslyReadIneligibleRefusal))
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
	page, err := myBooksPageForBook(r.Context(), h.services.Store.MyBooks, owner, language, book)
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

// bookStudyLanguage is the language a Book-addressed action returns to: the
// Book's own chosen language (ADR 0131), else the page's or the Last study
// language when the Book has none or cannot be read.
func (h *Handler) bookStudyLanguage(ctx context.Context, owner, bookID string) string {
	if book, err := h.services.Store.MyBooks.GetBookDetail(ctx, owner, bookID); err == nil {
		if language := myBookStudyLanguage(book); language != "" {
			return language
		}
	}
	return fallbackBookLanguage(ctx)
}

// fallbackBookLanguage is the language for a Book-addressed action whose Book
// has no chosen language: the page's, else the Last study language.
func fallbackBookLanguage(ctx context.Context) string {
	view := shellViewFromContext(ctx)
	if language := view.pageLanguageOrEmpty(); language != "" {
		return language
	}
	return view.lastLanguage()
}

func myBooksPageForBook(ctx context.Context, store MyBooksStore, owner, language string, book domain.MyBook) (int, error) {
	bucket := book.WorkflowBucket()
	disposition, _ := bucket.PersistedDisposition()
	if bucket == domain.MyBookBucketCurrentReading {
		disposition = domain.BookDispositionToRead
	}
	history := bucket == domain.MyBookBucketRead
	for offset := 0; ; offset += myBooksPageSize {
		result, err := store.ListMyBooksBrowse(ctx, owner, "", language, string(disposition), history, offset, myBooksPageSize)
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

func myBooksBrowseState(query string, page int, disposition domain.BookDisposition, language, languageLabel string, result domain.MyBooksBrowseResult) MyBooksBrowseState {
	browse := MyBooksBrowseState{
		Enabled:       true,
		Query:         query,
		Disposition:   disposition,
		Language:      language,
		LanguageLabel: languageLabel,
		AllCount:      result.AllCount,
		ReadCount:     result.ReadCount,
		ScopeTotal:    result.ScopeTotal,
		Total:         result.Total,
		Page:          page,
		PageCount:     myBooksPageCount(result.Total),
		TextNoMatch:   query != "" && result.Total == 0,
	}
	for _, count := range result.DispositionCounts {
		if count.Disposition == domain.BookDispositionToRead {
			browse.ToReadCount = count.Count
		}
	}
	return browse
}

func (h *Handler) removeBookFromMyBooks(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner, bookID := user(r).ID, r.PathValue("id")
	book, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, bookID)
	// A retried removal may no longer have an active My Books detail. The
	// store still decides whether the Book exists and can be removed.
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		fail(w, err)
		return
	}
	hasDetail := err == nil
	returnTo := myBooksReturnURL(r.FormValue("return_to"), removedBookLanguage(r.Context(), book, hasDetail))
	err = h.services.Store.MyBooks.RemoveBookFromMyBooks(r.Context(), owner, bookID)
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
	notice := "Removed from My Books. Its analysis and history are unchanged."
	switch {
	case book.ReadingUnit != nil && book.ReadingUnit.Withdrawn:
		notice += " This Reading unit is no longer in the Catalog, so it can't be added back."
	case hasDetail:
		notice += " The Catalog can add it back."
	}
	myBooksRedirectWithNotice(w, r, returnTo, notice)
}

func (h *Handler) moveBookToRead(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	language := h.bookStudyLanguage(r.Context(), owner, bookID)
	expectedRevision, ok := expectedDispositionRevision(r)
	if !ok {
		redirect(w, r, myBooksScopedURL(language, "", 1, domain.BookDispositionToRead, false)+"&error="+url.QueryEscape("This My Books form is out of date. Refresh My Books and try again."))
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
	message := moveToReadMessage(outcome)
	queryParam := "message"
	// A stale form is the only move outcome reported as an error notice.
	if outcome.Kind == readingintent.OutcomeStaleRevision {
		queryParam = "error"
	}
	redirect(w, r, myBooksScopedURL(language, "", 1, domain.BookDispositionToRead, false)+"&"+queryParam+"="+url.QueryEscape(message))
}

// moveToReadMessage maps a reading-intent outcome to the learner-facing
// sentence. The module returns no text of its own; an EPUB Book reports the
// same plain confirmation whether acquisition was queued now or is in progress.
func moveToReadMessage(outcome readingintent.Outcome) string {
	switch outcome.Kind {
	case readingintent.OutcomeStaleRevision:
		return "This Book's decision changed in another tab. No changes were made; refresh My Books and try again."
	case readingintent.OutcomeAlreadyCurrent:
		return "Book moved to To Read. Analysis is already current."
	case readingintent.OutcomeJobQueued:
		switch {
		case outcome.Route == readingintent.RouteClassical:
			return fmt.Sprintf("Book moved to To Read. Preparation job #%d submitted.", outcome.JobNumber)
		case outcome.Acquisition:
			return "Book moved to To Read. " + epubPreparationConfirmation
		default:
			return fmt.Sprintf("Book moved to To Read. Analysis job #%d submitted.", outcome.JobNumber)
		}
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

// unavailableEPUBAcquisition stands in for a deployment with no acquisition
// queue. A Book that needs a source cannot start preparation.
type unavailableEPUBAcquisition struct{}

func (unavailableEPUBAcquisition) SubmitEPUBAcquisition(context.Context, string, string) (analysis.EPUBAcquisitionHandle, error) {
	return analysis.EPUBAcquisitionHandle{}, readingintent.ErrAcquisitionUnavailable
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
	epubAcquisition := h.services.EPUBAcquisition
	if epubAcquisition == nil {
		epubAcquisition = unavailableEPUBAcquisition{}
	}
	preparation, err := readingintent.NewPreparation(readingintent.PreparationConfig{
		Classical:       classicalPreparation,
		Analysis:        analysisService,
		EPUBAcquisition: epubAcquisition,
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

func myBooksRedirectWithNotice(w http.ResponseWriter, r *http.Request, returnTo, text string) {
	separator := "?"
	if strings.Contains(returnTo, "?") {
		separator = "&"
	}
	redirect(w, r, returnTo+separator+"message="+url.QueryEscape(text))
}

// removedBookLanguage is the language a removal returns to, taken from the Book
// the handler already loaded.
func removedBookLanguage(ctx context.Context, book domain.MyBook, hasDetail bool) string {
	if hasDetail {
		if language := myBookStudyLanguage(book); language != "" {
			return language
		}
	}
	return fallbackBookLanguage(ctx)
}
