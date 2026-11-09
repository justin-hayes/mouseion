package webapp

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const (
	myBooksPageSize = 25
	// notPreparedYetNote is shown while an operator-registered Book has no
	// current analysis, and is the refusal To Read gives for it.
	notPreparedYetNote = "Not prepared yet."
)

// myBookEditionLabel is the edition an operator-registered Book shows under its
// author. It is empty for every other Book.
func myBookEditionLabel(book domain.MyBook) string {
	if book.ReadingUnit == nil {
		return ""
	}
	return strings.TrimSpace(book.ReadingUnit.EditionLabel)
}

// myBookMarginEvidence summarizes facts for the current source analysis only.
// Empty evidence stays empty; it is never converted into a failure or 0%.
func myBookMarginEvidence(book domain.MyBook) string {
	if book.WorkflowBucket() == domain.MyBookBucketRead && book.LatestCompletionAt != nil {
		return "Finished " + myBooksCompletionDateLabel(book.LatestCompletionAt) + "."
	}
	if book.AwaitingPreparation() {
		return notPreparedYetNote
	}
	if book.Acquired == nil {
		return ""
	}
	notes := myBookAnalysisMarginNotes(book)
	if note := myBookDeckMarginNote(book); note != "" {
		notes = append(notes, note)
	}
	return strings.Join(notes, " ")
}

// myBookAnalysisMarginNotes describes the current analysis. It names the
// re-analysis in flight first, then the published evidence it shadows.
func myBookAnalysisMarginNotes(book domain.MyBook) []string {
	classification := book.Classification()
	var notes []string
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
		if note := myBookCoverageMarginNote(book); note != "" {
			notes = append(notes, note)
		}
	case classification.Phase == domain.PhaseNotAnalyzed:
		if classification.Evidence == domain.BookUnavailable {
			notes = append(notes, "Content unavailable.")
		} else {
			notes = append(notes, "Not analysed yet.")
		}
	}
	return notes
}

// myBookCoverageMarginNote reports the running-word coverage the current
// analysis published. A Book without a token total stays silent rather than
// reporting 0%.
func myBookCoverageMarginNote(book domain.MyBook) string {
	if book.CoverageTotalTokens <= 0 {
		return ""
	}
	known := max(min(book.CoverageKnownTokens, book.CoverageTotalTokens), 0)
	percent := float64(known) * 100 / float64(book.CoverageTotalTokens)
	bandTarget, gap := domain.CoverageGapToNextBand(known, book.CoverageTotalTokens)
	if bandTarget > 0 {
		return fmt.Sprintf("%.1f%% of running words Known. %d more words to reach %d%%.", percent, gap, bandTarget)
	}
	return fmt.Sprintf("%.1f%% of running words Known.", percent)
}

// myBookDeckMarginNote reports deck preparation for the Current reading only.
// It is empty for a Book that is not being read and for an unknown deck state.
func myBookDeckMarginNote(book domain.MyBook) string {
	if !book.IsCurrentReading {
		return ""
	}
	switch book.DeckState {
	case "ready":
		if book.DeckPreparedAt != nil {
			return fmt.Sprintf("Deck ready: %d cards, prepared %s.", book.DeckCardCount, book.DeckPreparedAt.Local().Format("2 January 2006"))
		}
		return fmt.Sprintf("Deck ready: %d cards.", book.DeckCardCount)
	case "preparing", "queued":
		return "Deck preparation running."
	case "failed":
		return "Deck preparation failed."
	case "cancelled":
		return "Deck preparation cancelled."
	}
	return ""
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
	BookTotal          int
	WorkTotal          int
	RowCount           int
	HasWorks           bool
	Rows               []domain.MyBooksBrowseRow
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

// myBooksWorkBucketOrder is the ordinary bucket precedence a grouped Work row
// uses for its one leading badge.
var myBooksWorkBucketOrder = []domain.MyBookBucket{
	domain.MyBookBucketCurrentReading,
	domain.MyBookBucketToRead,
	domain.MyBookBucketRead,
	domain.MyBookBucketInbox,
}

func myBookBucketLabel(bucket domain.MyBookBucket) string {
	switch bucket {
	case domain.MyBookBucketCurrentReading:
		return "Currently reading"
	case domain.MyBookBucketToRead:
		return "To Read"
	case domain.MyBookBucketRead:
		return "Read"
	default:
		return "Inbox"
	}
}

func myBookWorkflowLabel(book domain.MyBook) string {
	return myBookBucketLabel(book.WorkflowBucket())
}

// myBookWorkBucketShape gives a grouped Work's cells and leading badge four
// distinct shapes. Ordinary rows keep the shared StatusBadge shape vocabulary.
func myBookWorkBucketShape(bucket domain.MyBookBucket) string {
	switch bucket {
	case domain.MyBookBucketCurrentReading:
		return "status-badge__shape status-badge__shape--dot"
	case domain.MyBookBucketToRead:
		return "status-badge__shape status-badge__shape--ring"
	case domain.MyBookBucketRead:
		return "status-badge__shape status-badge__shape--bar"
	default:
		return "status-badge__shape status-badge__shape--square"
	}
}

func myBookWorkflowTone(book domain.MyBook) StatusTone {
	if book.WorkflowBucket() == domain.MyBookBucketCurrentReading {
		return StatusInfo
	}
	return StatusNeutral
}

// myBookDisplayTitle names a classical constituent Book by its Work and place,
// such as "Histories, Book 1". A whole-work unit keeps the Work title.
func myBookDisplayTitle(book domain.MyBook) string {
	title := book.Book.Title
	if unit := book.ReadingUnit; unit != nil {
		if strings.TrimSpace(unit.WorkTitle) != "" {
			title = unit.WorkTitle
		}
		if number, _, ok := unit.BookPosition(); ok {
			return fmt.Sprintf("%s, Book %d", title, number)
		}
	}
	return title
}

// myBookPositionLabel is a constituent Book's reviewed place in its Work. A
// whole-work unit has no position.
func myBookPositionLabel(book domain.MyBook) string {
	if unit := book.ReadingUnit; unit != nil {
		if number, total, ok := unit.BookPosition(); ok {
			return fmt.Sprintf("Book %d of %d", number, total)
		}
	}
	return ""
}

func myBookGreekTitle(book domain.MyBook) string {
	if unit := book.ReadingUnit; unit != nil {
		return strings.TrimSpace(unit.GreekTitle)
	}
	return ""
}

func myBooksWorkRowID(workKey string) string {
	return "work-row-" + url.PathEscape(workKey)
}

// myBooksWorkSizeLabel states the Work's reviewed size and how much of it the
// learner owns: "all 24 books" or "2 of 24 books".
func myBooksWorkSizeLabel(row domain.MyBooksBrowseRow) string {
	owned := row.OwnedBooks
	if owned <= 0 {
		owned = len(row.Books)
	}
	total := row.ConstituentBooks
	if total <= 0 {
		total = owned
	}
	if owned > total {
		owned = total
	}
	switch {
	case total == 1:
		return "1 book"
	case owned >= total:
		return fmt.Sprintf("all %d books", total)
	default:
		return fmt.Sprintf("%d of %d books", owned, total)
	}
}

// myBooksWorkLeadingBook applies the ordinary bucket precedence: current
// reading, To Read, Read, Inbox.
func myBooksWorkLeadingBook(books []domain.MyBook) (domain.MyBookBucket, domain.MyBook) {
	for _, bucket := range myBooksWorkBucketOrder {
		for _, book := range books {
			if book.WorkflowBucket() == bucket {
				return bucket, book
			}
		}
	}
	return domain.MyBookBucketInbox, domain.MyBook{}
}

// myBooksWorkLeadingBadge is the one StatusBadge a grouped Work row shows.
func myBooksWorkLeadingBadge(books []domain.MyBook) (label string, tone StatusTone, shape string) {
	bucket, book := myBooksWorkLeadingBook(books)
	shape = myBookWorkBucketShape(bucket)
	if bucket == domain.MyBookBucketCurrentReading {
		if unit := book.ReadingUnit; unit != nil && unit.ConstituentBook > 0 {
			return fmt.Sprintf("Currently reading Book %d", unit.ConstituentBook), StatusInfo, shape
		}
		return "Currently reading", StatusInfo, shape
	}
	return myBookBucketLabel(bucket), StatusNeutral, shape
}

// myBooksWorkCountsLabel states the rest of the Work's shape after the leading
// badge, such as "22 more To Read · 1 Read".
func myBooksWorkCountsLabel(books []domain.MyBook) string {
	if len(books) == 0 {
		return ""
	}
	leading, _ := myBooksWorkLeadingBook(books)
	counts := map[domain.MyBookBucket]int{}
	for _, book := range books {
		counts[book.WorkflowBucket()]++
	}
	var parts []string
	for _, bucket := range myBooksWorkBucketOrder {
		count := counts[bucket]
		if bucket == leading {
			count--
		}
		if count <= 0 {
			continue
		}
		label := myBookBucketLabel(bucket)
		if bucket == leading {
			parts = append(parts, fmt.Sprintf("%d more %s", count, label))
		} else {
			parts = append(parts, fmt.Sprintf("%d %s", count, label))
		}
	}
	return strings.Join(parts, " · ")
}

// myBooksWorkCellLabel is the full accessible label of one Book cell.
func myBooksWorkCellLabel(row domain.MyBooksBrowseRow, book domain.MyBook) string {
	label := row.WorkTitle
	if strings.TrimSpace(label) == "" {
		label = book.Book.Title
	}
	if unit := book.ReadingUnit; unit != nil {
		if number, total, ok := unit.BookPosition(); ok {
			label = fmt.Sprintf("%s, Book %d of %d", label, number, total)
		}
	}
	return fmt.Sprintf("%s, %s", label, myBookWorkflowLabel(book))
}

func myBooksWorkCellNumber(book domain.MyBook) string {
	if unit := book.ReadingUnit; unit != nil && unit.ConstituentBook > 0 {
		return strconv.Itoa(unit.ConstituentBook)
	}
	return "1"
}

func myBooksWorkCellClass(book domain.MyBook) string {
	return "library-work__cell library-work__cell--" + string(book.WorkflowBucket())
}

// myBooksShapeLabel states the filtered collection's shape in Books and works.
func myBooksShapeLabel(browse MyBooksBrowseState) string {
	books := browse.BookTotal
	if books == 0 {
		books = browse.Total
	}
	works := browse.WorkTotal
	if works == 0 {
		works = browse.RowCount
	}
	if works == 0 {
		works = books
	}
	switch {
	case books == 1 && works == 1:
		return "1 Book in 1 work"
	case books == 1:
		return fmt.Sprintf("1 Book in %d works", works)
	case works == 1:
		return fmt.Sprintf("%d Books in 1 work", books)
	default:
		return fmt.Sprintf("%d Books in %d works", books, works)
	}
}

// myBooksBrowseRows is the page as rendered. Store-backed browsing supplies
// rows directly; lightweight callers fall back to grouping their Book slice.
func myBooksBrowseRows(browse MyBooksBrowseState, books []domain.MyBook) []domain.MyBooksBrowseRow {
	if len(browse.Rows) > 0 {
		return browse.Rows
	}
	return myBooksFallbackRows(browse, books)
}

func myBooksHasGroupedWork(rows []domain.MyBooksBrowseRow) bool {
	for _, row := range rows {
		if row.Grouped {
			return true
		}
	}
	return false
}

// myBooksFallbackRows groups an already-selected Book page for callers that do
// not supply rows, such as the in-memory fixture store and legacy LibraryPage.
func myBooksFallbackRows(browse MyBooksBrowseState, books []domain.MyBook) []domain.MyBooksBrowseRow {
	visible, owned := myBooksFallbackVisible(browse, books)
	rows := myBooksFallbackGroups(visible, owned)
	for i := range rows {
		myBooksFinalizeFallbackRow(&rows[i])
	}
	return rows
}

func myBooksFallbackVisible(browse MyBooksBrowseState, books []domain.MyBook) ([]domain.MyBook, map[string]int) {
	visible := make([]domain.MyBook, 0, len(books))
	owned := map[string]int{}
	for _, book := range books {
		if !myBooksBookInLanguage(browse, book) {
			continue
		}
		visible = append(visible, book)
		if unit := book.ReadingUnit; unit != nil && unit.WorkKey != "" {
			owned[unit.WorkKey]++
		}
	}
	return visible, owned
}

func myBooksFallbackGroups(visible []domain.MyBook, owned map[string]int) []domain.MyBooksBrowseRow {
	rows := make([]domain.MyBooksBrowseRow, 0, len(visible))
	index := map[string]int{}
	for _, book := range visible {
		key := domain.MyBookWorkGroupKey(book)
		if i, ok := index[key]; ok {
			rows[i].Books = append(rows[i].Books, book)
			continue
		}
		row := domain.MyBooksBrowseRow{Key: key, Books: []domain.MyBook{book}}
		if unit := book.ReadingUnit; unit != nil && unit.WorkKey != "" {
			row.WorkKey = unit.WorkKey
			row.WorkTitle = unit.WorkTitle
			row.WorkAuthor = unit.WorkAuthor
			row.GreekTitle = unit.GreekTitle
			row.ConstituentBooks = unit.ConstituentBooks
			row.OwnedBooks = owned[unit.WorkKey]
		}
		index[key] = len(rows)
		rows = append(rows, row)
	}
	return rows
}

func myBooksFinalizeFallbackRow(row *domain.MyBooksBrowseRow) {
	if row.WorkKey == "" {
		return
	}
	row.Grouped = len(row.Books) > 1 || row.OwnedBooks > 1
	sort.SliceStable(row.Books, func(i, j int) bool {
		return domain.MyBookWorkOrderLess(row.Books[i], row.Books[j])
	})
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
