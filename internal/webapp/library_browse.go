package webapp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const (
	myBooksPageSize = 25
	// notPreparedYetNote is shown while a classical Book has no current
	// analysis, and is the refusal To Read gives for it.
	notPreparedYetNote = "Not prepared yet."
	// myBooksNeedsActionValue is the value of the needs query parameter that
	// shows only the Books with a status line.
	myBooksNeedsActionValue = "action"
)

// myBookEditionLabel is the edition a classical Book shows under its author.
// It is empty for every other Book.
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
	if preparation, ok := classicalPreparationFor(book); ok {
		return preparation.Label + "."
	}
	if acquisition, ok := epubAcquisitionFor(book); ok {
		if acquisition.InProgress {
			return acquisition.Label + "."
		}
		return acquisition.Label
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
		notes = append(notes, analysisFailureSentence(classification.FailureCode))
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
	case domain.RunFailed:
		if reason := analysisFailureReason(classification.FailureCode); reason != "" {
			return "Re-analysis failed. " + reason
		}
		return "Re-analysis failed."
	case domain.RunCancelled:
		return "Re-analysis cancelled."
	case domain.RunNone, domain.RunPublicationPending, domain.RunPublicationFailed, domain.RunCompleted:
	}
	return ""
}

func myBookEvidenceRecovery(book domain.MyBook) string {
	if book.IsCurrentReading {
		return ""
	}
	action := myBookActionableRecovery(book)
	if action == "" {
		return ""
	}
	if book.Disposition == domain.BookDispositionToRead {
		return action
	}
	if book.CompletionCount > 0 {
		return action + " by reading again"
	}
	return action + " by moving to To Read"
}

// myBookActionableRecovery names the recovery the Book's applicable evidence
// offers. The caller adds the disposition-specific suffix.
func myBookActionableRecovery(book domain.MyBook) string {
	if preparation, ok := classicalPreparationFor(book); ok {
		if !preparation.Actionable {
			return ""
		}
		return preparation.Recovery
	}
	if acquisition, ok := epubAcquisitionFor(book); ok {
		if !acquisition.Actionable {
			return ""
		}
		return acquisition.Recovery
	}
	if book.Acquired == nil {
		return ""
	}
	return classicalRecoveryLabel(book.Classification().Recovery)
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
	Enabled       bool
	Query         string
	Disposition   domain.BookDisposition
	History       bool
	NeedsAction   bool
	Language      string
	LanguageLabel string
	AllCount      int
	ScopeTotal    int
	Total         int
	Page          int
	PageCount     int
	TextNoMatch   bool
	ToReadCount   int
	ReadCount     int
}

// myBooksScopedURL is the one place a My Books tab location is spelled.
func myBooksScopedURL(language, query string, page int, disposition domain.BookDisposition, history bool) string {
	return myBooksFilteredURL(language, query, page, disposition, history, false)
}

// myBooksFilteredURL is myBooksScopedURL plus the needs-action filter, which
// is kept in the address so a reload or a shared link shows the same Books.
func myBooksFilteredURL(language, query string, page int, disposition domain.BookDisposition, history, needsAction bool) string {
	values := url.Values{}
	if needsAction {
		values.Set("needs", myBooksNeedsActionValue)
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
		return myBooksPath(language)
	}
	return myBooksPath(language) + "?" + encoded
}

// myBooksPath is the My Books address for a language, unprefixed only when the
// learner has no study language.
func myBooksPath(language string) string {
	return studyLanguagePath(pageKindMyBooks, language)
}

// myBookStudyLanguage is the language a Book belongs to, empty until chosen.
func myBookStudyLanguage(book domain.MyBook) string {
	if book.Book.LanguageState != domain.LanguageChosen {
		return ""
	}
	return canonicalization.NormalizeLanguage(book.Book.LanguageTag)
}

// myBooksPathLanguage reads the language of a My Books path: "/library" has
// none, and "/de/library" is German. Any other path is not My Books.
func myBooksPathLanguage(path string) (language string, ok bool) {
	if path == "/library" {
		return "", true
	}
	prefix, rest, found := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if !found || rest != "library" || !isLanguageSegment(prefix) {
		return "", false
	}
	return canonicalization.NormalizeLanguage(prefix), true
}

// parseMyBooksBrowseRequest reads the two-tab My Books request. The retired
// needs-language scope and the Inbox filter are unsupported rather than
// translated.
func parseMyBooksBrowseRequest(rURL *url.URL) (myBooksLibraryRequest, bool) {
	values := rURL.Query()
	if values.Has("needs-language") {
		return myBooksLibraryRequest{}, false
	}
	request := myBooksLibraryRequest{query: strings.TrimSpace(values.Get("q"))}
	request.disposition = domain.BookDisposition(strings.TrimSpace(values.Get("disposition")))
	switch request.disposition {
	case "", domain.BookDispositionToRead:
	default:
		return myBooksLibraryRequest{}, false
	}
	switch values.Get("needs") {
	case "":
	case myBooksNeedsActionValue:
		request.needsAction = true
	default:
		return myBooksLibraryRequest{}, false
	}
	request.history = values.Get("history") == "read"
	if request.history {
		if request.disposition != "" {
			return myBooksLibraryRequest{}, false
		}
	} else if request.disposition == "" {
		request.disposition = domain.BookDispositionToRead
	}
	request.page = 1
	if parsed, err := strconv.Atoi(values.Get("page")); err == nil && parsed >= 1 {
		request.page = parsed
	}
	return request, true
}

// myBooksReturnURL validates a posted return location. Only a local My Books
// address is honored, so an action can return to the exact browse state without
// becoming an open redirect. The result is in language, the Book's own; a
// return location in another language keeps none of its browse state.
func myBooksReturnURL(raw, language string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return myBooksPath(language)
	}
	returnLanguage, ok := myBooksPathLanguage(parsed.Path)
	if !ok {
		return myBooksPath(language)
	}
	if language == "" {
		language = returnLanguage
	}
	if returnLanguage != "" && returnLanguage != language {
		return myBooksPath(language)
	}
	values := parsed.Query()
	values.Del("message")
	values.Del("error")
	request, ok := parseMyBooksBrowseRequest(&url.URL{RawQuery: values.Encode()})
	if !ok {
		return myBooksPath(language)
	}
	return myBooksFilteredURL(language, request.query, request.page, request.disposition, request.history, request.needsAction)
}

// myBookSearchURL searches the tab that lists the Book for its title, so a
// Read Book is found on the Read tab and every other Book on To Read.
func myBookSearchURL(book domain.MyBook) string {
	history := book.WorkflowBucket() == domain.MyBookBucketRead
	disposition := domain.BookDispositionToRead
	if history {
		disposition = ""
	}
	return myBooksScopedURL(myBookStudyLanguage(book), book.Book.Title, 1, disposition, history)
}

func myBooksResultsURL(browse MyBooksBrowseState, page int) string {
	return myBooksFilteredURL(browse.Language, browse.Query, page, browse.Disposition, browse.History, browse.NeedsAction)
}

// myBooksClearSearchURL keeps every control except the search text.
func myBooksClearSearchURL(browse MyBooksBrowseState) string {
	return myBooksFilteredURL(browse.Language, "", 1, browse.Disposition, browse.History, browse.NeedsAction)
}

func myBooksToReadTabURL(browse MyBooksBrowseState) string {
	return myBooksFilteredURL(browse.Language, browse.Query, 1, domain.BookDispositionToRead, false, browse.NeedsAction)
}

func myBooksReadTabURL(browse MyBooksBrowseState) string {
	return myBooksFilteredURL(browse.Language, browse.Query, 1, "", true, browse.NeedsAction)
}

// myBooksNeedsActionToggleURL turns the needs-action filter on or off and keeps
// every other control, starting again at the first page.
func myBooksNeedsActionToggleURL(browse MyBooksBrowseState) string {
	return myBooksFilteredURL(browse.Language, browse.Query, 1, browse.Disposition, browse.History, !browse.NeedsAction)
}

// myBookBucketURL names where a Book is listed.
func myBookBucketURL(book domain.MyBook, page int) string {
	language := myBookStudyLanguage(book)
	switch book.WorkflowBucket() {
	case domain.MyBookBucketToRead, domain.MyBookBucketCurrentReading:
		return myBooksScopedURL(language, "", page, domain.BookDispositionToRead, false)
	case domain.MyBookBucketRead:
		return myBooksScopedURL(language, "", page, "", true)
	case domain.MyBookBucketNone:
		return myBooksPath(language)
	}
	return myBooksPath(language)
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

func myBookBucketLabel(bucket domain.MyBookBucket) string {
	switch bucket {
	case domain.MyBookBucketCurrentReading:
		return "Currently reading"
	case domain.MyBookBucketToRead:
		return "To Read"
	case domain.MyBookBucketRead:
		return "Read"
	default:
		return ""
	}
}

func myBookWorkflowLabel(book domain.MyBook) string {
	return myBookBucketLabel(book.WorkflowBucket())
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

func myBookTitlePlate(book domain.MyBook) titlePlate {
	return newTitlePlate(myBookDisplayTitle(book), myBookOriginalTitle(book), book.Book.LanguageTag, book.Book.Author)
}

func myBookOriginalTitle(book domain.MyBook) string {
	return readingUnitOriginalTitle(book.ReadingUnit)
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
	return browse.Query != "" || browse.History || browse.NeedsAction
}

func myBooksResultAnnouncementAttributes() templ.Attributes {
	return templ.Attributes{"role": "status", "aria-live": "polite", "aria-atomic": "true"}
}

func myBooksTabLabel(browse MyBooksBrowseState) string {
	if browse.History {
		return "Read"
	}
	return "To Read"
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

// myBooksBrowseBooks is the page's Books in the active language. Store-backed
// browsing already supplies exactly the page; lightweight callers such as the
// in-memory fixture store and legacy LibraryPage pass an unfiltered slice.
func myBooksBrowseBooks(browse MyBooksBrowseState, books []domain.MyBook) []domain.MyBook {
	visible := make([]domain.MyBook, 0, len(books))
	for _, book := range books {
		if myBooksBookInLanguage(browse, book) {
			visible = append(visible, book)
		}
	}
	return visible
}

// myBookNeedsAction is the status line of a Book that needs the learner's
// action, derived only from evidence the Book already carries. A Book with
// nothing to do has no status line and is never listed by the needs-action
// filter. The first applicable state wins, most basic first.
func myBookNeedsAction(book domain.MyBook) (string, bool) {
	if book.Book.LanguageState != domain.LanguageChosen {
		return "Choose a Book language.", true
	}
	if status, ok := myBookEvidenceNeedsAction(book); ok {
		return status, true
	}
	if _, candidate := myBookAnalysisUpdate(book); candidate != nil {
		return "Analysis update available.", true
	}
	if myBookSourceStructureUpdate(book) != nil {
		return "Main-text selection update available.", true
	}
	return "", false
}

// myBookEvidenceNeedsAction reports a failed acquisition or preparation, and a
// failed or stale analysis.
func myBookEvidenceNeedsAction(book domain.MyBook) (string, bool) {
	classification := book.Classification()
	switch classification.Acquisition.State {
	case domain.AcquisitionNone, domain.AcquisitionInProgress:
	default:
		return "Acquisition failed.", true
	}
	switch classification.Preparation.State {
	case domain.PreparationFetchFailed, domain.PreparationImportFailed, domain.PreparationSourceRefused,
		domain.PreparationWithdrawn, domain.PreparationCatalogUnavailable:
		return "Preparation failed.", true
	case domain.PreparationNone, domain.PreparationQueued, domain.PreparationRunning, domain.PreparationCleared:
	}
	if book.Acquired == nil {
		return "", false
	}
	if classification.Phase == domain.PhaseFailed {
		return "Analysis failed.", true
	}
	if classification.ReAnalysisShadowed() && classification.Run == domain.RunFailed {
		return "Re-analysis failed.", true
	}
	if classification.Phase == domain.PhaseStale {
		return "Analysis out of date.", true
	}
	return "", false
}
