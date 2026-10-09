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

// readingBrowseProblem names why the Reading Working desk could not show a Browse
// page. A problem is never rendered as an empty or partial result.
type readingBrowseProblem string

const (
	readingBrowseReady          readingBrowseProblem = ""
	readingBrowseFailed         readingBrowseProblem = "failed"
	readingBrowseTimedOut       readingBrowseProblem = "timed_out"
	readingBrowseChanged        readingBrowseProblem = "changed"
	readingBrowsePageMalformed  readingBrowseProblem = "page_malformed"
	readingBrowsePageOutOfRange readingBrowseProblem = "page_out_of_range"
)

// readingBrowseView is the live Browse for the active-language Current reading
// Book. Reading is the only surface that owns it.
type readingBrowseView struct {
	Language      string
	Prefix        string
	Query         domain.VocabularyBrowseQuery
	Page          domain.VocabularyBrowsePage
	Problem       readingBrowseProblem
	RequestedPage string
}

func vocabularyBrowseDisplayLemma(language, lemma, upos string) string {
	return lemmadisplay.Format(language, lemma, upos)
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

// loadReadingBrowse reads the complete live Browse for the Current reading Book
// and reports the HTTP status the Reading page must carry. Scope always comes
// from the learner's Current reading; a request naming another Book, a stale
// corpus revision, or a page outside the result is refused with an explanation
// instead of silently widening, clamping, or falling back.
func (h *Handler) loadReadingBrowse(ctx context.Context, r *http.Request, owner, language string, bookID string) (readingBrowseView, int) {
	values := r.URL.Query()
	view := readingBrowseView{Language: language, Prefix: values.Get("q"), RequestedPage: values.Get("page")}
	view.Query = domain.VocabularyBrowseQuery{
		Language: language, Prefix: view.Prefix, CurrentBookID: bookID, ReadingBookID: values.Get("reading"),
		// Legacy Book/POS/state/sort parameters are intentionally ignored. Browse is
		// always the complete current-Book identity set in frequency order.
		Sort: "occurrences", Page: 1, Revision: values.Get("rev"), IncludeAll: values.Get("all") == "1",
	}
	if view.RequestedPage != "" {
		page, err := strconv.Atoi(view.RequestedPage)
		if err != nil || page < 1 {
			view.Problem = readingBrowsePageMalformed
			return view, http.StatusBadRequest
		}
		view.Query.Page = page
	}
	if view.Query.ReadingBookID != "" && view.Query.ReadingBookID != bookID {
		view.Problem = readingBrowseChanged
		return view, http.StatusConflict
	}
	view.Query.ReadingBookID = bookID
	// Browse is an interactive request, not a durable background job. Bound the
	// complete read so an unusually broad corpus can never leave the learner
	// waiting indefinitely or turn a partial read into a successful page.
	browseCtx, cancel := context.WithTimeout(ctx, h.interactiveReadTimeout())
	defer cancel()
	page, err := h.services.Store.VocabularyBrowse.ListVocabularyBrowsePage(browseCtx, owner, language, view.Query)
	if err != nil {
		log.Printf("mouseion: load vocabulary Browse: %v", err)
		status := vocabularyBrowseErrorStatus(err, browseCtx)
		view.Problem = readingBrowseFailed
		if status == http.StatusGatewayTimeout {
			view.Problem = readingBrowseTimedOut
		}
		return view, status
	}
	page.CurrentBookID, page.ReadingBookID, page.Language = bookID, bookID, language
	view.Page = page
	if view.Query.Revision != "" && view.Query.Revision != page.CorpusRevision {
		view.Problem = readingBrowseChanged
		return view, http.StatusConflict
	}
	if view.Query.Page > 1 && view.Query.Page > vocabularyBrowseLastPage(page.Total) && !page.BrowseCountsUpdating && !page.BrowseCountsUnavailable {
		view.Problem = readingBrowsePageOutOfRange
		return view, http.StatusNotFound
	}
	return view, http.StatusOK
}

func vocabularyBrowsePageURL(page int, browse domain.VocabularyBrowsePage, query string) string {
	values := url.Values{}
	if browse.Language != "" {
		values.Set("language", browse.Language)
	}
	if query != "" {
		values.Set("q", query)
	}
	if browse.IncludeAll {
		values.Set("all", "1")
	}
	readingBookID := browse.ReadingBookID
	if readingBookID == "" {
		readingBookID = browse.CurrentBookID
	}
	if readingBookID != "" {
		values.Set("reading", readingBookID)
	}
	if browse.CorpusRevision != "" {
		values.Set("rev", browse.CorpusRevision)
	}
	values.Set("page", strconv.Itoa(page))
	return "/reading?" + values.Encode()
}

// readingBrowseRefreshURL re-checks count readiness for the same Current
// reading, prefix, and page. It is a plain link: it never requeues work.
func readingBrowseRefreshURL(view readingBrowseView) string {
	return vocabularyBrowsePageURL(view.Query.Page, domain.VocabularyBrowsePage{
		CurrentBookID: view.Query.CurrentBookID, IncludeAll: view.Query.IncludeAll, Language: view.Query.Language,
	}, view.Prefix)
}

// readingBrowseClearPrefixURL drops only the prefix, keeping the accounting
// reveal choice.
func readingBrowseClearPrefixURL(view readingBrowseView) string {
	return vocabularyBrowsePageURL(1, domain.VocabularyBrowsePage{
		CurrentBookID: view.Query.CurrentBookID, IncludeAll: view.Query.IncludeAll, Language: view.Query.Language,
	}, "")
}

// readingBrowseFirstPageURL restarts Browse at its first page for the exact
// Current reading, dropping any stale revision or page.
func readingBrowseFirstPageURL(view readingBrowseView) string {
	return vocabularyBrowsePageURL(1, domain.VocabularyBrowsePage{
		CurrentBookID: view.Query.CurrentBookID, IncludeAll: view.Query.IncludeAll, Language: view.Query.Language,
	}, view.Prefix)
}
