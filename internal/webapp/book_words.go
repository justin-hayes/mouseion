package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
)

const vocabularyBrowsePageSize = 25

// bookWordsProblem names why the Book detail page could not show its word list.
// A problem is never rendered as an empty or partial result.
type bookWordsProblem string

const (
	bookWordsReady          bookWordsProblem = ""
	bookWordsFailed         bookWordsProblem = "failed"
	bookWordsTimedOut       bookWordsProblem = "timed_out"
	bookWordsChanged        bookWordsProblem = "changed"
	bookWordsPageMalformed  bookWordsProblem = "page_malformed"
	bookWordsPageOutOfRange bookWordsProblem = "page_out_of_range"
)

// bookWordState is where a word stands in Study (ADR 0136 §3). Mature is the
// only state that counts as knowing the word.
type bookWordState string

const (
	bookWordMature        bookWordState = "Mature"
	bookWordInStudy       bookWordState = "In study"
	bookWordNotIntroduced bookWordState = "Not yet introduced"
)

// bookWordRow is one word of the Book with its Study state.
type bookWordRow struct {
	domain.VocabularyBrowseRow
	State bookWordState
}

// bookWordsView is the read-only list of a Book's words on its detail page.
type bookWordsView struct {
	BookID        string
	Language      string
	Prefix        string
	Query         domain.VocabularyBrowseQuery
	Page          domain.VocabularyBrowsePage
	Rows          []bookWordRow
	Problem       bookWordsProblem
	RequestedPage string
	// FocusRow is the row a Back to book vocabulary request asks to focus, and
	// FocusRowFound reports whether the restored page still has it. A missing row
	// gives focus to the results heading, never to a different row.
	FocusRow      string
	FocusRowFound bool
}

// FocusRowMissing is true only when a restore named a row the page lacks.
func (v bookWordsView) FocusRowMissing() bool {
	return v.FocusRow != "" && !v.FocusRowFound
}

// rowOrigin is the Concordance origin for one word of the list.
func (v bookWordsView) rowOrigin(row domain.VocabularyBrowseRow) browseOrigin {
	return browseOrigin{
		BookID: v.BookID, Prefix: v.Prefix,
		Page: v.Page.Page, Revision: v.Page.CorpusRevision, Row: browseRowKey(row.UPOS, row.CanonicalLemma, row.LexicalDiscriminator),
	}
}

// bookWordStateOf is the Study state the store reports for a word of the list.
func bookWordStateOf(row domain.VocabularyBrowseRow) bookWordState {
	switch row.StudyState {
	case domain.VocabularyStudyMature:
		return bookWordMature
	case domain.VocabularyStudyInStudy:
		return bookWordInStudy
	case domain.VocabularyStudyNotIntroduced:
		return bookWordNotIntroduced
	}
	return bookWordNotIntroduced
}

// vocabularyBrowseRowLemma is the learner-facing form of a word. An imported
// classical identity shows its citation headword, never the stored canonical
// key; every other identity shows the language-formatted lemma.
func vocabularyBrowseRowLemma(language string, row domain.VocabularyBrowseRow) string {
	if row.Headword != "" {
		return row.Headword
	}
	return lemmadisplay.Format(language, row.CanonicalLemma, row.UPOS)
}

func vocabularyBrowseOccurrenceCounts(row domain.VocabularyBrowseRow) string {
	return fmt.Sprintf("In this Book: %d; Across analyzed books: %d", row.OccurrenceCount, row.AcrossBooksOccurrenceCount)
}

func vocabularyBrowseErrorStatus(err error, ctx context.Context) int {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}

func vocabularyBrowseLastPage(total int64) int {
	return int((total + vocabularyBrowsePageSize - 1) / vocabularyBrowsePageSize)
}

// loadBookWords reads the complete word list of one Book with each word's Study
// state and reports the HTTP status the page must carry. A stale evidence
// revision or a page outside the result is refused with an explanation instead
// of silently clamping or falling back.
func (h *Handler) loadBookWords(ctx context.Context, r *http.Request, owner, language, bookID string) (bookWordsView, int) {
	values := r.URL.Query()
	view, problem, status := parseBookWordsRequest(values, language, bookID)
	if problem != bookWordsReady {
		view.Problem = problem
		return view, status
	}
	// The list is an interactive request, not a durable background job. Bound the
	// complete read so an unusually broad corpus can never leave the learner
	// waiting indefinitely or turn a partial read into a successful page.
	readCtx, cancel := context.WithTimeout(ctx, h.interactiveReadTimeout())
	defer cancel()
	page, problem, status := h.loadBookWordsPage(readCtx, owner, language, view.Query)
	view.Page = page
	if problem != bookWordsReady {
		view.Problem = problem
		return view, status
	}
	if problem, status = bookWordsPageCheck(view.Query, page); problem != bookWordsReady {
		view.Problem = problem
		return view, status
	}
	view.Rows = make([]bookWordRow, 0, len(page.Rows))
	for _, row := range page.Rows {
		view.Rows = append(view.Rows, bookWordRow{VocabularyBrowseRow: row, State: bookWordStateOf(row)})
	}
	view.FocusRow = values.Get("row")
	view.FocusRowFound = bookWordsHasRow(page.Rows, view.FocusRow)
	return view, http.StatusOK
}

// parseBookWordsRequest builds the word-list request from the page query and
// rejects a malformed page number.
func parseBookWordsRequest(values url.Values, language, bookID string) (bookWordsView, bookWordsProblem, int) {
	view := bookWordsView{BookID: bookID, Language: language, Prefix: values.Get("q"), RequestedPage: values.Get("page")}
	view.Query = domain.VocabularyBrowseQuery{
		Language: language, Prefix: view.Prefix, CurrentBookID: bookID, ReadingBookID: bookID,
		// The list is always the Book's complete identity set in frequency order.
		// Study state, not an accounting filter, tells the learner where each word
		// stands.
		Sort: "occurrences", Page: 1, Revision: values.Get("rev"),
	}
	if view.RequestedPage != "" {
		page, err := strconv.Atoi(view.RequestedPage)
		if err != nil || page < 1 {
			return view, bookWordsPageMalformed, http.StatusBadRequest
		}
		view.Query.Page = page
	}
	return view, bookWordsReady, http.StatusOK
}

// loadBookWordsPage reads the Book's page and reports why it failed: a timeout
// is a gateway timeout, any other failure is a server error.
func (h *Handler) loadBookWordsPage(ctx context.Context, owner, language string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, bookWordsProblem, int) {
	page, err := h.services.Store.Reading.ListVocabularyBrowsePage(ctx, owner, language, query)
	if err != nil {
		log.Printf("mouseion: load Book words: %v", err)
		if vocabularyBrowseErrorStatus(err, ctx) == http.StatusGatewayTimeout {
			return domain.VocabularyBrowsePage{}, bookWordsTimedOut, http.StatusGatewayTimeout
		}
		return domain.VocabularyBrowsePage{}, bookWordsFailed, http.StatusInternalServerError
	}
	page.CurrentBookID, page.ReadingBookID, page.Language = query.CurrentBookID, query.CurrentBookID, language
	return page, bookWordsReady, http.StatusOK
}

// bookWordsPageCheck refuses a page whose evidence changed or whose number the
// loaded result no longer contains.
func bookWordsPageCheck(query domain.VocabularyBrowseQuery, page domain.VocabularyBrowsePage) (bookWordsProblem, int) {
	if query.Revision != "" && query.Revision != page.CorpusRevision {
		return bookWordsChanged, http.StatusConflict
	}
	if query.Page > 1 && query.Page > vocabularyBrowseLastPage(page.Total) && !page.BrowseCountsUpdating && !page.BrowseCountsUnavailable {
		return bookWordsPageOutOfRange, http.StatusNotFound
	}
	return bookWordsReady, http.StatusOK
}

// bookWordsHasRow reports whether the page still contains the row a Back to
// book vocabulary request named.
func bookWordsHasRow(rows []domain.VocabularyBrowseRow, focusRow string) bool {
	if focusRow == "" {
		return false
	}
	for _, row := range rows {
		if browseRowKey(row.UPOS, row.CanonicalLemma, row.LexicalDiscriminator) == focusRow {
			return true
		}
	}
	return false
}

// bookDetailURL is a Book's detail page. It is Book-addressed (ADR 0131): the
// page runs in the Book's own study language.
func bookDetailURL(bookID string) string {
	return "/books/" + url.PathEscape(bookID)
}

// bookActionRedirect answers a Book action with its message or error. A form
// posted from the Book's own detail page names it in return_to and returns
// there; any other form returns to Reading, so return_to is never an open
// redirect.
func bookActionRedirect(w http.ResponseWriter, r *http.Request, bookID, key, text string) {
	location := myBooksPagePath(r.Context())
	if r.FormValue("return_to") == bookDetailURL(bookID) {
		location = bookDetailURL(bookID)
	}
	redirect(w, r, location+"?"+key+"="+url.QueryEscape(text))
}

// bookWordsURL is the Book detail page at one page and prefix of its word list.
func bookWordsURL(bookID string, page int, prefix, revision, row string) string {
	values := url.Values{}
	if prefix != "" {
		values.Set("q", prefix)
	}
	if revision != "" {
		values.Set("rev", revision)
	}
	if row != "" {
		values.Set("row", row)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	location := bookDetailURL(bookID)
	if encoded := values.Encode(); encoded != "" {
		location += "?" + encoded
	}
	return location
}

func bookWordsPageURL(page int, browse domain.VocabularyBrowsePage, query string) string {
	return bookWordsURL(browse.CurrentBookID, page, query, browse.CorpusRevision, "")
}

// bookWordsRefreshURL re-checks count readiness for the same prefix and page.
// It is a plain link: it never requeues work.
func bookWordsRefreshURL(view bookWordsView) string {
	return bookWordsURL(view.BookID, view.Query.Page, view.Prefix, "", "")
}

// bookWordsClearPrefixURL drops only the prefix.
func bookWordsClearPrefixURL(view bookWordsView) string {
	return bookWordsURL(view.BookID, 1, "", "", "")
}

// bookWordsFirstPageURL restarts the list at its first page, dropping any stale
// revision or page.
func bookWordsFirstPageURL(view bookWordsView) string {
	return bookWordsURL(view.BookID, 1, view.Prefix, "", "")
}
