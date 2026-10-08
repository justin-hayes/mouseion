package webapp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const myBooksPageSize = 25

// myBookMarginEvidence summarizes facts for the current source analysis only.
// Empty evidence stays empty; it is never converted into a failure or 0%.
func myBookMarginEvidence(book domain.MyBook) string {
	if book.WorkflowBucket() == domain.MyBookBucketRead && book.LatestCompletionAt != nil {
		return "Finished " + myBooksCompletionDateLabel(book.LatestCompletionAt) + "."
	}
	if book.Acquired == nil {
		return ""
	}
	var notes []string
	status := strings.ToLower(strings.TrimSpace(book.Acquired.AnalysisStatus))
	switch status {
	case "analyzing":
		notes = append(notes, "Analysis running.")
	case "analysis failed", "failed":
		notes = append(notes, "Analysis failed.")
	case "analysis cancelled", "cancelled":
		notes = append(notes, "Analysis cancelled.")
	case "stale":
		notes = append(notes, "Analysis out of date.")
	case "content unavailable":
		notes = append(notes, "Content unavailable.")
	case "not analyzed":
		if !myBookHasCurrentContent(book) {
			notes = append(notes, "Content unavailable.")
		} else {
			notes = append(notes, "Not analysed yet.")
		}
	case "analyzed":
		if book.CoverageTotalTokens > 0 {
			known := max(min(book.CoverageKnownTokens, book.CoverageTotalTokens), 0)
			percent := float64(known) * 100 / float64(book.CoverageTotalTokens)
			bandTarget, gap := domain.CoverageGapToNextBand(known, book.CoverageTotalTokens)
			if bandTarget > 0 {
				notes = append(notes, fmt.Sprintf("%.1f%% of running words Known. %d more words to reach %d%%.", percent, gap, bandTarget))
			} else {
				notes = append(notes, fmt.Sprintf("%.1f%% of running words Known.", percent))
			}
		}
	default:
		if !myBookHasCurrentContent(book) {
			notes = append(notes, "Content unavailable.")
		} else {
			notes = append(notes, "Not analysed yet.")
		}
	}
	if book.IsCurrentReading && book.DeckState == "ready" {
		if book.DeckPreparedAt != nil {
			notes = append(notes, fmt.Sprintf("Deck ready: %d cards, prepared %s.", book.DeckCardCount, book.DeckPreparedAt.Local().Format("2 January 2006")))
		} else {
			notes = append(notes, fmt.Sprintf("Deck ready: %d cards.", book.DeckCardCount))
		}
	} else if book.IsCurrentReading && (book.DeckState == "preparing" || book.DeckState == "queued") {
		notes = append(notes, "Deck preparation running.")
	} else if book.IsCurrentReading && book.DeckState == "failed" {
		notes = append(notes, "Deck preparation failed.")
	} else if book.IsCurrentReading && book.DeckState == "cancelled" {
		notes = append(notes, "Deck preparation cancelled.")
	}
	return strings.Join(notes, " ")
}

func myBookEvidenceRecovery(book domain.MyBook) string {
	if book.Acquired == nil || book.IsCurrentReading {
		return ""
	}
	status := strings.ToLower(strings.TrimSpace(book.Acquired.AnalysisStatus))
	action := ""
	if status == "analysis failed" || status == "failed" || status == "analysis cancelled" || status == "cancelled" || status == "stale" {
		action = "Retry analysis"
	} else if status == "content unavailable" || !myBookHasCurrentContent(book) {
		action = "Retry acquisition"
	}
	if action == "" || book.Disposition == domain.BookDispositionToRead {
		return action
	}
	if book.CompletionCount > 0 {
		return action + " by reading again"
	}
	return action + " by moving to To Read"
}

func myBookEvidenceRecoveryURL(book domain.MyBook) string {
	if book.Disposition == domain.BookDispositionToRead {
		return readingReanalyzeURL(book.Book.ID)
	}
	if book.CompletionCount > 0 {
		return "/library/books/" + url.PathEscape(book.Book.ID) + "/read-again"
	}
	return "/library/books/" + url.PathEscape(book.Book.ID) + "/to-read"
}

func myBookHasCurrentContent(book domain.MyBook) bool {
	return book.Acquired != nil && book.Acquired.Source.ContentRevisionID != "" && book.Acquired.Source.ContentSnapshotID != ""
}

// MyBooksBrowseState carries the server-rendered collection controls and
// result state. Enabled deliberately distinguishes the new read model from
// the zero-value legacy rendering used by lightweight stores.
type MyBooksBrowseState struct {
	Enabled            bool
	Query              string
	Disposition        domain.BookDisposition
	History            bool
	Language           string
	LanguageLabel      string
	NeedsLanguage      bool
	NeedsLanguageCount int
	AllCount           int
	ScopeTotal         int
	Total              int
	Page               int
	PageCount          int
	TextNoMatch        bool
	InboxCount         int
	ToReadCount        int
	SetAsideCount      int
	ReadCount          int
	RefreshableBookIDs map[string]bool
}

func myBooksURL(query string, page int, needsLanguage bool) string {
	return myBooksFilteredURL(query, page, needsLanguage, "")
}

func myBooksFilteredURL(query string, page int, needsLanguage bool, disposition domain.BookDisposition) string {
	return myBooksHistoryURL(query, page, needsLanguage, disposition, false)
}

func myBooksHistoryURL(query string, page int, needsLanguage bool, disposition domain.BookDisposition, history bool) string {
	values := url.Values{}
	if needsLanguage {
		values.Set("needs-language", "")
	}
	if query != "" {
		values.Set("q", query)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if disposition != "" {
		values.Set("disposition", string(disposition))
	}
	if history {
		values.Set("history", "read")
	}
	if encoded := values.Encode(); encoded != "" {
		if needsLanguage {
			encoded = strings.Replace(encoded, "needs-language=", "needs-language", 1)
		}
		return "/library?" + encoded
	}
	return "/library"
}

func parseMyBooksBrowseRequest(rURL *url.URL) (query string, page int, needsLanguage bool) {
	query, page, needsLanguage, _ = parseMyBooksBrowseRequestWithDisposition(rURL)
	return query, page, needsLanguage
}

func parseMyBooksBrowseRequestWithDisposition(rURL *url.URL) (query string, page int, needsLanguage bool, disposition domain.BookDisposition) {
	query = strings.TrimSpace(rURL.Query().Get("q"))
	needsLanguage = rURL.Query().Has("needs-language")
	disposition = domain.BookDisposition(strings.TrimSpace(rURL.Query().Get("disposition")))
	if disposition.Validate() != nil {
		disposition = ""
	}
	page = 1
	if parsed, err := strconv.Atoi(rURL.Query().Get("page")); err == nil && parsed >= 1 {
		page = parsed
	}
	return query, page, needsLanguage, disposition
}

func parseMyBooksBrowseRequestWithHistory(rURL *url.URL) (query string, page int, needsLanguage bool, disposition domain.BookDisposition, history bool) {
	query, page, needsLanguage, disposition = parseMyBooksBrowseRequestWithDisposition(rURL)
	history = rURL.Query().Get("history") == "read"
	return query, page, needsLanguage, disposition, history
}

func myBooksResultsURL(browse MyBooksBrowseState, page int) string {
	return myBooksHistoryURL(browse.Query, page, browse.NeedsLanguage, browse.Disposition, browse.History)
}

func myBooksHistoryFilterURL(browse MyBooksBrowseState, history bool) string {
	return myBooksHistoryURL(browse.Query, 1, browse.NeedsLanguage, "", history)
}

func myBookVisibleBucketURL(book domain.MyBook, page int) string {
	switch book.WorkflowBucket() {
	case domain.MyBookBucketInbox:
		return myBooksFilteredURL("", page, false, domain.BookDispositionInbox)
	case domain.MyBookBucketToRead:
		return myBooksFilteredURL("", page, false, domain.BookDispositionToRead)
	case domain.MyBookBucketRead:
		return myBooksHistoryURL("", page, false, "", true)
	case domain.MyBookBucketSetAside:
		return myBooksFilteredURL("", page, false, domain.BookDispositionSetAside)
	case domain.MyBookBucketCurrentReading:
		return myBooksFilteredURL("", page, false, domain.BookDispositionToRead)
	}
	return "/library"
}

func myBooksDispositionURL(browse MyBooksBrowseState, disposition domain.BookDisposition) string {
	return myBooksFilteredURL(browse.Query, 1, browse.NeedsLanguage, disposition)
}

func myBooksDispositionCount(browse MyBooksBrowseState, disposition domain.BookDisposition) int {
	switch disposition {
	case domain.BookDispositionInbox:
		return browse.InboxCount
	case domain.BookDispositionToRead:
		return browse.ToReadCount
	case domain.BookDispositionSetAside:
		return browse.SetAsideCount
	default:
		return browse.ScopeTotal
	}
}

func myBooksReadCountLabel(count int) string {
	if count == 1 {
		return "1 completion"
	}
	return fmt.Sprintf("%d completions", count)
}

func myBooksCompletionSourceLabel(source domain.ReadingCompletionSource) string {
	if source == domain.ReadingCompletionPreviouslyRead {
		return "Read before Mouseion"
	}
	return "Finished in Mouseion"
}

func myBooksCompletionDateLabel(completedAt *time.Time) string {
	if completedAt == nil {
		return "Date unavailable"
	}
	return completedAt.Local().Format("2 Jan 2006")
}

func myBooksDispositionLabel(disposition domain.BookDisposition) string {
	switch disposition {
	case domain.BookDispositionInbox:
		return "Inbox"
	case domain.BookDispositionToRead:
		return "To Read"
	case domain.BookDispositionSetAside:
		return "Set Aside"
	default:
		return "All"
	}
}

func myBookDisposition(book domain.MyBook) domain.BookDisposition {
	if book.Disposition != "" {
		return book.Disposition
	}
	if myBookInJourney(book) {
		return domain.BookDispositionToRead
	}
	return domain.BookDispositionInbox
}

func myBookWorkflowLabel(book domain.MyBook) string {
	switch book.WorkflowBucket() {
	case domain.MyBookBucketCurrentReading:
		return "Currently reading"
	case domain.MyBookBucketToRead:
		return "To Read"
	case domain.MyBookBucketInbox:
		return "Inbox"
	case domain.MyBookBucketRead:
		return "Read"
	case domain.MyBookBucketSetAside:
		return "Set Aside"
	default:
		return "Inbox"
	}
}

func myBookWorkflowTone(book domain.MyBook) StatusTone {
	if book.WorkflowBucket() == domain.MyBookBucketCurrentReading {
		return StatusInfo
	}
	return StatusNeutral
}

func myBookSetAsideConfirmationText(book domain.MyBook) string {
	retained := " Acquired content, analysis, provenance, and reading history remain."
	if myBookDisposition(book) == domain.BookDispositionToRead {
		return "This removes the Book from Reading." + retained
	}
	return "This sets aside the Book without adding it to Reading." + retained
}

func myBooksAllDispositionCount(browse MyBooksBrowseState) int {
	if browse.Enabled {
		return browse.ScopeTotal
	}
	return browse.AllCount
}

func myBooksPageCount(total int) int {
	if total <= 0 {
		return 0
	}
	return (total + myBooksPageSize - 1) / myBooksPageSize
}

func myBooksSelectedAttributes(selected bool) templ.Attributes {
	if selected {
		return templ.Attributes{"aria-current": "page"}
	}
	return nil
}

func myBooksResultCount(total int) string {
	if total == 1 {
		return "1 book found"
	}
	return fmt.Sprintf("%d books found", total)
}

func myBooksHasActiveResultFilter(browse MyBooksBrowseState) bool {
	return browse.Query != "" || browse.Disposition != "" || browse.History || browse.NeedsLanguage
}

func myBooksResultAnnouncementAttributes() templ.Attributes {
	return templ.Attributes{"role": "status", "aria-live": "polite", "aria-atomic": "true"}
}

func myBooksLanguageName(browse MyBooksBrowseState) string {
	if browse.LanguageLabel != "" {
		return browse.LanguageLabel
	}
	return browse.Language
}

func myBooksBookInLanguage(browse MyBooksBrowseState, book domain.MyBook) bool {
	if browse.Language == "" {
		return true
	}
	return book.Book.LanguageState == domain.LanguageChosen && strings.EqualFold(strings.TrimSpace(book.Book.LanguageTag), browse.Language)
}

func myBooksPageOffset(page int) int {
	if page <= 1 {
		return 0
	}
	maxInt := int(^uint(0) >> 1)
	if page-1 > maxInt/myBooksPageSize {
		return maxInt
	}
	return (page - 1) * myBooksPageSize
}
