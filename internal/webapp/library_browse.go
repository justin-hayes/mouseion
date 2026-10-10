package webapp

import (
	"fmt"
	"net/http"
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
	classification := book.Classification()
	if note := reanalysisMarginNote(classification); note != "" {
		notes = append(notes, note)
	}
	switch {
	case classification.PublicationPending():
		notes = append(notes, analysisPublicationPendingNote)
	case classification.Phase == domain.PhaseAnalyzing:
		notes = append(notes, "Analysis running.")
	case classification.Phase == domain.PhaseFailed:
		notes = append(notes, "Analysis failed.")
	case classification.Phase == domain.PhaseCancelled:
		notes = append(notes, "Analysis cancelled.")
	case classification.Phase == domain.PhaseStale:
		notes = append(notes, "Analysis out of date.")
	case classification.Phase == domain.PhaseAnalyzed:
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
	case classification.Phase == domain.PhaseNotAnalyzed:
		if classification.Evidence == domain.BookUnavailable {
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

// reanalysisMarginNote names a newer run that a published analysis outlives.
// The published analysis stays the evidence, so the note is secondary.
func reanalysisMarginNote(classification domain.BookEvidenceClassification) string {
	if !classification.ReAnalysisShadowed() {
		return ""
	}
	switch classification.Run {
	case domain.RunQueued:
		return "Re-analysis queued."
	case domain.RunRunning:
		return "Re-analysis running."
	case domain.RunFailed, domain.RunJobFailed:
		return "Re-analysis failed."
	case domain.RunCancelled:
		return "Re-analysis cancelled."
	case domain.RunNone, domain.RunPublicationPending, domain.RunPublicationFailed, domain.RunCompleted:
	}
	return ""
}

func myBookEvidenceRecovery(book domain.MyBook) string {
	if book.Acquired == nil || book.IsCurrentReading {
		return ""
	}
	action := ""
	switch book.Classification().Recovery {
	case domain.RecoveryRetryAnalysis:
		action = "Retry analysis"
	case domain.RecoveryRetryAcquisition:
		action = "Retry acquisition"
	case domain.RecoveryNone:
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

// MyBooksBrowseState carries the server-rendered collection controls and
// result state. Enabled deliberately distinguishes the new read model from
// the zero-value legacy rendering used by lightweight stores.
type MyBooksBrowseState struct {
	Enabled            bool
	Query              string
	Disposition        domain.BookDisposition
	History            bool
	ShowHidden         bool
	HiddenCount        int
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
	return myBooksScopedURL(query, page, needsLanguage, disposition, history, false)
}

// myBooksScopedURL is the one place a My Books location is spelled. Flag
// parameters (needs-language, show-hidden) are bare so the address stays
// readable and ordinary links work without JavaScript.
func myBooksScopedURL(query string, page int, needsLanguage bool, disposition domain.BookDisposition, history, showHidden bool) string {
	values := url.Values{}
	if needsLanguage {
		values.Set("needs-language", "")
	}
	if showHidden {
		values.Set("show-hidden", "")
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
	encoded := values.Encode()
	if encoded == "" {
		return "/library"
	}
	parts := strings.Split(encoded, "&")
	for i, part := range parts {
		if part == "needs-language=" || part == "show-hidden=" {
			parts[i] = strings.TrimSuffix(part, "=")
		}
	}
	encoded = strings.Join(parts, "&")
	return "/library?" + encoded
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

// parseMyBooksShowHidden reports the visibility scope. Presence of the bare
// show-hidden parameter selects it; the default scope omits Hidden Books.
func parseMyBooksShowHidden(rURL *url.URL) bool {
	return rURL.Query().Has("show-hidden")
}

// myBooksReturnFromRequest derives the browse location of an HTMX row refresh
// from the page that issued it, so a row's Hide/Unhide returns to that view.
func myBooksReturnFromRequest(r *http.Request) string {
	current, err := url.Parse(r.Header.Get("Hx-Current-Url"))
	if err != nil || current.Path != "/library" {
		return "/library"
	}
	return myBooksReturnURL(current.Path + "?" + current.RawQuery)
}

// myBooksReturnURL validates a posted return location. Only a local /library
// address is honored, so Hide/Unhide can return to the exact browse state
// without becoming an open redirect.
func myBooksReturnURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.Path != "/library" {
		return "/library"
	}
	values := parsed.Query()
	values.Del("message")
	values.Del("error")
	query, page, needsLanguage, disposition, history := parseMyBooksBrowseRequestWithHistory(&url.URL{RawQuery: values.Encode()})
	return myBooksScopedURL(query, page, needsLanguage, disposition, history, parseMyBooksShowHidden(&url.URL{RawQuery: values.Encode()}))
}

func myBooksResultsURL(browse MyBooksBrowseState, page int) string {
	return myBooksScopedURL(browse.Query, page, browse.NeedsLanguage, browse.Disposition, browse.History, browse.ShowHidden)
}

// myBooksClearSearchURL keeps every control except the search text.
func myBooksClearSearchURL(browse MyBooksBrowseState) string {
	return myBooksScopedURL("", 1, browse.NeedsLanguage, browse.Disposition, browse.History, browse.ShowHidden)
}

func myBooksHistoryFilterURL(browse MyBooksBrowseState, history bool) string {
	return myBooksScopedURL(browse.Query, 1, browse.NeedsLanguage, "", history, browse.ShowHidden)
}

// myBooksShowHiddenURL toggles the visibility scope while keeping the rest of
// the browse state.
func myBooksShowHiddenURL(browse MyBooksBrowseState) string {
	return myBooksScopedURL(browse.Query, 1, browse.NeedsLanguage, browse.Disposition, browse.History, !browse.ShowHidden)
}

func myBooksShowHiddenLabel(browse MyBooksBrowseState) string {
	if browse.ShowHidden {
		return "Stop showing hidden books"
	}
	if browse.HiddenCount > 0 {
		return fmt.Sprintf("Show hidden books (%d)", browse.HiddenCount)
	}
	return "Show hidden books"
}

func myBooksHiddenOmittedLabel(count int) string {
	if count == 1 {
		return "1 hidden book is not shown."
	}
	return fmt.Sprintf("%d hidden books are not shown.", count)
}

// myBookVisibleBucketURL names where a Book is listed. A Hidden Book is only
// listed in the Show hidden books scope, so its location carries that scope.
func myBookVisibleBucketURL(book domain.MyBook, page int) string {
	hidden := book.Hidden
	switch book.WorkflowBucket() {
	case domain.MyBookBucketInbox:
		return myBooksScopedURL("", page, false, domain.BookDispositionInbox, false, hidden)
	case domain.MyBookBucketToRead, domain.MyBookBucketCurrentReading:
		return myBooksScopedURL("", page, false, domain.BookDispositionToRead, false, hidden)
	case domain.MyBookBucketRead:
		return myBooksScopedURL("", page, false, "", true, hidden)
	}
	return "/library"
}

func myBooksDispositionURL(browse MyBooksBrowseState, disposition domain.BookDisposition) string {
	return myBooksScopedURL(browse.Query, 1, browse.NeedsLanguage, disposition, false, browse.ShowHidden)
}

func myBooksDispositionCount(browse MyBooksBrowseState, disposition domain.BookDisposition) int {
	switch disposition {
	case domain.BookDispositionInbox:
		return browse.InboxCount
	case domain.BookDispositionToRead:
		return browse.ToReadCount
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
	default:
		return "All"
	}
}

func myBookDisposition(book domain.MyBook) domain.BookDisposition {
	if book.Disposition != "" {
		return book.Disposition
	}
	if myBookInReading(book) {
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

func myBookVisibilityURL(book domain.MyBook) string {
	action := "hide"
	if book.Hidden {
		action = "unhide"
	}
	return "/library/books/" + url.PathEscape(book.Book.ID) + "/" + action
}

func myBookVisibilityActionLabel(book domain.MyBook) string {
	if book.Hidden {
		return "Unhide"
	}
	return "Hide"
}
