package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type readingChooserBookView struct {
	Book        domain.MyBook
	Coverage    *domain.AnalysisCoverage
	Band        domain.CoverageBand
	State       readingChooserState
	Description string
}

type readingChooserState string

// vocabularyCountsUpdatingDescription explains why a Book shows no coverage
// while its effective vocabulary counts are rebuilt.
const vocabularyCountsUpdatingDescription = "This Book's vocabulary counts are being refreshed. Coverage appears when they are ready."

const (
	readingChooserInProgress     readingChooserState = "in_progress"
	readingChooserNeedsAttention readingChooserState = "needs_attention"
	readingChooserUpdating       readingChooserState = "updating"
)

type readingChooserPageView struct {
	Language, LanguageLabel          string
	CurrentBookID, CurrentSnapshotID string
	At99Plus, At97To99               []readingChooserBookView
	At95To97, Below95                []readingChooserBookView
	NoComparison                     []readingChooserBookView
	InProgress, Attention            []readingChooserBookView
	Updating                         []readingChooserBookView
}

func (h *Handler) reading(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	currentReading, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	// Back to book vocabulary names the commitment it left. Only that exact
	// snapshot may restore controls; anything else is explained before any
	// other reading context opens, and nothing is transferred to it.
	if snapshot := strings.TrimSpace(r.URL.Query().Get("snapshot")); snapshot != "" && (!currentReading.IsActive() || currentReading.SnapshotID != snapshot || r.URL.Query().Get("reading") != currentReading.BookID) {
		h.renderReadingBrowseOriginExpired(w, r, owner, language, currentReading.IsActive())
		return
	}
	if currentReading.IsActive() {
		readingView, buildErr := h.buildReadingView(r.Context(), owner.ID, language)
		if buildErr != nil {
			fail(w, buildErr)
			return
		}
		readingView.Language = language
		readingView.LanguageLabel = languageLabel
		status := http.StatusOK
		if readingView.CurrentReading != nil {
			readingView.Browse, status = h.loadReadingBrowse(r.Context(), r, owner.ID, language, currentReading.BookID)
			readingView.Browse.SnapshotID = currentReading.SnapshotID
		}
		renderStatus(w, r, status, ReadingPage(owner, h.csrf(w, r), readingView, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
		return
	}
	view, err := h.buildReadingChooser(r.Context(), owner.ID, language, languageLabel)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, ReadingChooserPage(owner, h.csrf(w, r), view, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
}

// renderReadingBrowseOriginExpired explains a Back to book vocabulary request
// whose commitment ended or was restarted. The My Books recovery is offered
// only for a Book the owner has in the active study language.
func (h *Handler) renderReadingBrowseOriginExpired(w http.ResponseWriter, r *http.Request, owner domain.User, language string, hasCurrent bool) {
	myBooksURL := ""
	if bookID := strings.TrimSpace(r.URL.Query().Get("reading")); bookID != "" {
		detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
		switch {
		case errors.Is(err, persistence.ErrNotFound):
		case err != nil:
			fail(w, err)
			return
		case detail.Book.LanguageTag == language:
			myBooksURL = "/library?" + url.Values{"q": {detail.Book.Title}}.Encode()
		}
	}
	renderStatus(w, r, http.StatusConflict, ReadingBrowseOriginExpiredPage(owner, h.csrf(w, r), hasCurrent, myBooksURL))
}

func (h *Handler) switchReadingPage(w http.ResponseWriter, r *http.Request) {
	owner := user(r).ID
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	current, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	if !current.IsActive() {
		redirect(w, r, "/reading?error="+url.QueryEscape("There is no current book to switch from. Choose a book from Reading first."))
		return
	}
	view, err := h.buildReadingChooser(r.Context(), owner, language, languageLabel)
	if err != nil {
		fail(w, err)
		return
	}
	view.CurrentBookID = current.BookID
	view.CurrentSnapshotID = current.SnapshotID
	view.At99Plus = withoutReadingBook(view.At99Plus, current.BookID)
	view.At97To99 = withoutReadingBook(view.At97To99, current.BookID)
	view.At95To97 = withoutReadingBook(view.At95To97, current.BookID)
	view.Below95 = withoutReadingBook(view.Below95, current.BookID)
	view.NoComparison = withoutReadingBook(view.NoComparison, current.BookID)
	view.InProgress = withoutReadingBook(view.InProgress, current.BookID)
	view.Attention = withoutReadingBook(view.Attention, current.BookID)
	render(w, r, ReadingChooserPage(user(r), h.csrf(w, r), view, "", ""))
}

func withoutReadingBook(books []readingChooserBookView, bookID string) []readingChooserBookView {
	filtered := books[:0]
	for _, book := range books {
		if book.Book.Book.ID != bookID {
			filtered = append(filtered, book)
		}
	}
	return filtered
}

func (h *Handler) switchReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	language, _ := activeStudyLanguageForContext(r.Context())
	expectedBookID := strings.TrimSpace(r.FormValue("expected_current_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if language == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape("Choose a study language and refresh Reading before switching books."))
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	// Flag detection is a write the freeze depends on, so it runs before the
	// switch. Persistence decides everything else.
	if _, err := h.services.LemmaReview.Assess(r.Context(), owner, detail.Book.ID); err != nil {
		fail(w, err)
		return
	}
	_, err = h.services.Store.Reading.SwitchCurrentReading(r.Context(), owner, language, bookID, expectedBookID, expectedSnapshotID)
	var ineligible persistence.CurrentReadingIneligibleError
	switch {
	case errors.Is(err, persistence.ErrCurrentReadingStale), errors.Is(err, persistence.ErrNotFound):
		redirect(w, r, "/reading?error="+url.QueryEscape("The current book changed while you were choosing. No changes were made; review Reading before trying again."))
	case errors.As(err, &ineligible) && ineligible.Reason.IsBookChoiceRejection():
		redirect(w, r, "/reading?error="+url.QueryEscape("This book is no longer an eligible To Read choice. No changes were made; refresh Reading and try again."))
	case errors.Is(err, persistence.ErrCurrentReadingIneligible):
		redirect(w, r, "/reading?error="+url.QueryEscape("This book no longer has current analyzed content or is no longer To Read. No changes were made; refresh Reading and try again."))
	case errors.Is(err, persistence.ErrUnresolvedLemmaReviewFlags):
		redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review")
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsPending):
		redirect(w, r, "/reading?error="+url.QueryEscape("Across-book vocabulary counts are updating. No reading was changed; try switching again when the counts are ready."))
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsUnavailable):
		redirect(w, r, "/reading?error="+url.QueryEscape("Across-book vocabulary counts are unavailable after repeated rebuild failures. No reading was changed; ask the server operator to restart Mouseion, then retry."))
	case err != nil:
		fail(w, err)
	default:
		redirect(w, r, "/reading?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
	}
}

// endCurrentReading ends the exact expected commitment. It needs no live
// analysis, so End stays available when the Book's analysis is missing or
// stale. Replays are verified by the store against durable facts.
func (h *Handler) endCurrentReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	language, _ := activeStudyLanguageForContext(r.Context())
	expectedBookID := strings.TrimSpace(r.FormValue("expected_current_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if language == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape("Choose a study language and refresh Reading before ending the current reading."))
		return
	}
	if expectedBookID == "" || expectedSnapshotID == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape(endCurrentReadingStaleMessage))
		return
	}
	err := h.services.Store.Reading.EndCurrentReading(r.Context(), owner, language, expectedBookID, expectedSnapshotID)
	if errors.Is(err, persistence.ErrCurrentReadingStale) || errors.Is(err, persistence.ErrNotFound) {
		redirect(w, r, "/reading?error="+url.QueryEscape(endCurrentReadingStaleMessage))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape("Reading ended. "+h.currentReadingBookTitle(r.Context(), owner, expectedBookID)+" stays in To Read; its analysis, decks, and history are preserved. Choose a Book when you are ready."))
}

const endCurrentReadingStaleMessage = "The current reading changed before this action. No changes were made; review Reading and try again."

func (h *Handler) reanalyzeToReadBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	current, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, detail.Book.LanguageTag)
	if err != nil {
		fail(w, err)
		return
	}
	if current.IsActive() && current.BookID == detail.Book.ID {
		rejectCurrentBookRefresh(w, r)
		return
	}
	if detail.Disposition != domain.BookDispositionToRead {
		http.NotFound(w, r)
		return
	}
	handle, _, _, _, err := h.ensureToReadAnalysis(r.Context(), owner, bookID)
	if err != nil {
		if errors.Is(err, analysis.ErrCurrentBook) {
			rejectCurrentBookRefresh(w, r)
			return
		}
		redirect(w, r, "/reading?error="+url.QueryEscape("Acquisition or analysis could not be started. The To Read choice is retained. Review current book content in My Books and try again."))
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(fmt.Sprintf("Analysis job #%d submitted.", handle.DisplayNumber)))
}

func rejectCurrentBookRefresh(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/reading?error="+url.QueryEscape("The current book cannot be refreshed. Its frozen evidence remains unchanged."))
}

func (h *Handler) startReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape("Choose a study language before starting a book."))
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	// Flag detection is a write the freeze depends on, so it runs before the
	// Start. Persistence decides everything else.
	if _, err := h.services.LemmaReview.Assess(r.Context(), owner, detail.Book.ID); err != nil {
		fail(w, err)
		return
	}
	start, err := h.services.Store.Reading.StartCurrentReadingResult(r.Context(), owner, language, bookID)
	var ineligible persistence.CurrentReadingIneligibleError
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		http.NotFound(w, r)
		return
	case err == nil && start.Replayed:
		// Starting the Book that is already current is a repeat, not a conflict.
		redirect(w, r, "/reading?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is already your current reading."))
		return
	case errors.Is(err, persistence.ErrCurrentReadingExists):
		redirect(w, r, "/reading?error="+url.QueryEscape("A current book is already set for this language. Review it in Reading before starting another."))
		return
	case errors.As(err, &ineligible) && ineligible.Reason.IsBookChoiceRejection():
		redirect(w, r, "/reading?error="+url.QueryEscape("This book is no longer an eligible To Read candidate. Review Reading before trying again."))
		return
	case errors.Is(err, persistence.ErrCurrentReadingIneligible):
		redirect(w, r, "/reading?error="+url.QueryEscape("This book no longer has trustworthy current analysis or is no longer To Read. No changes were made; refresh Reading and try again."))
		return
	case errors.Is(err, persistence.ErrUnresolvedLemmaReviewFlags):
		redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review")
		return
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsPending):
		redirect(w, r, "/reading?error="+url.QueryEscape("Across-book vocabulary counts are updating. No reading was changed; try starting again when the counts are ready."))
		return
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsUnavailable):
		redirect(w, r, "/reading?error="+url.QueryEscape("Across-book vocabulary counts are unavailable after repeated rebuild failures. No reading was changed; ask the server operator to restart Mouseion, then retry."))
		return
	case err != nil:
		fail(w, err)
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
}

func (h *Handler) buildReadingChooser(ctx context.Context, owner, language, languageLabel string) (readingChooserPageView, error) {
	books, err := h.services.Store.Reading.ListMyBooksWithEvidence(ctx, owner)
	if err != nil {
		return readingChooserPageView{}, err
	}
	view := readingChooserPageView{Language: language, LanguageLabel: languageLabel}
	for _, book := range books {
		if book.Disposition != domain.BookDispositionToRead || language == "" || book.Book.LanguageTag != language {
			continue
		}
		// Hidden only keeps a Book out of the default chooser; it never makes
		// the Book ineligible. Recovery is My Books → Show hidden books →
		// Unhide → choose it here.
		if book.Hidden {
			continue
		}
		candidate := readingChooserBookView{Book: book}
		if book.Acquired == nil || !analysisReadyForReading(*book.Acquired) {
			candidate.State, candidate.Description = readingChooserStatusFor(book.Classification())
			switch candidate.State {
			case readingChooserInProgress:
				view.InProgress = append(view.InProgress, candidate)
			case readingChooserNeedsAttention:
				view.Attention = append(view.Attention, candidate)
			case readingChooserUpdating: // set only once coverage is read, below
			}
			continue
		}
		if h.services.AnalysisInsights == nil || strings.TrimSpace(book.Acquired.CorpusID) == "" {
			candidate.State = readingChooserNeedsAttention
			candidate.Description = "Completed analysis statistics are unavailable. Refresh the page or retry analysis."
			view.Attention = append(view.Attention, candidate)
			continue
		}
		coverage, coverageErr := h.services.AnalysisInsights.Coverage(ctx, owner, book.Acquired.CorpusID)
		if errors.Is(coverageErr, analysisinsights.ErrStatisticsUnavailable) {
			candidate.State = readingChooserNeedsAttention
			candidate.Description = "Completed analysis statistics are unavailable. Retry analysis to refresh the evidence."
			view.Attention = append(view.Attention, candidate)
			continue
		}
		if errors.Is(coverageErr, analysisinsights.ErrCountsUpdating) {
			candidate.State = readingChooserUpdating
			candidate.Description = vocabularyCountsUpdatingDescription
			view.Updating = append(view.Updating, candidate)
			continue
		}
		if coverageErr != nil {
			return readingChooserPageView{}, coverageErr
		}
		candidate.Coverage = &coverage
		candidate.Band = domain.CoverageBandFor(coverage.KnownTokenCount, coverage.AnalyzableTokenCount)
		switch candidate.Band {
		case domain.CoverageBand99Plus:
			view.At99Plus = append(view.At99Plus, candidate)
		case domain.CoverageBand97To99:
			view.At97To99 = append(view.At97To99, candidate)
		case domain.CoverageBand95To97:
			view.At95To97 = append(view.At95To97, candidate)
		case domain.CoverageBandBelow95:
			view.Below95 = append(view.Below95, candidate)
		case domain.CoverageBandNoComparison:
			view.NoComparison = append(view.NoComparison, candidate)
		}
	}
	for _, group := range [][]readingChooserBookView{view.At99Plus, view.At97To99, view.At95To97, view.Below95, view.NoComparison, view.Updating, view.InProgress, view.Attention} {
		sort.Slice(group, func(i, j int) bool { return readingChooserLess(group[i], group[j]) })
	}
	return view, nil
}

func readingChooserLess(left, right readingChooserBookView) bool {
	leftTitle, rightTitle := strings.ToLower(readingChooserBookTitle(left.Book)), strings.ToLower(readingChooserBookTitle(right.Book))
	if leftTitle != rightTitle {
		return leftTitle < rightTitle
	}
	leftAuthor, rightAuthor := strings.ToLower(strings.TrimSpace(left.Book.Book.Author)), strings.ToLower(strings.TrimSpace(right.Book.Book.Author))
	if leftAuthor != rightAuthor {
		return leftAuthor < rightAuthor
	}
	return left.Book.Book.ID < right.Book.Book.ID
}

func readingChooserBookTitle(book domain.MyBook) string {
	if title := strings.TrimSpace(book.Book.Title); title != "" {
		return title
	}
	if book.Acquired != nil {
		if title := strings.TrimSpace(book.Acquired.BookTitle); title != "" {
			return title
		}
	}
	return book.Book.ID
}

// readingChooserStatusFor places a To Read Book without completed analysis in
// the chooser's in-progress or needs-attention group.
func readingChooserStatusFor(c domain.BookEvidenceClassification) (readingChooserState, string) {
	if c.Content == domain.ContentNotAcquired {
		return readingChooserNeedsAttention, "Book content has not been acquired yet. Return to My Books to review its catalog entry."
	}
	if c.PublicationPending() {
		return readingChooserInProgress, analysisPublicationPendingDescription
	}
	if c.Phase == domain.PhaseAnalyzing {
		return readingChooserInProgress, "Analysis is queued or running. This candidate will appear in a coverage group when current evidence is ready."
	}
	if c.Evidence == domain.BookStale {
		return readingChooserNeedsAttention, "The analysis no longer matches the current book content. Retry analysis to refresh its evidence."
	}
	if c.Evidence == domain.BookUnavailable || c.Evidence == domain.BookNotAcquired {
		return readingChooserNeedsAttention, "Current book content is unavailable. Retry acquisition or analysis from My Books."
	}
	if c.Phase == domain.PhaseFailed || c.Phase == domain.PhaseCancelled {
		return readingChooserNeedsAttention, "The last analysis did not complete. Retry analysis to refresh its evidence."
	}
	return readingChooserNeedsAttention, "Current analysis is not complete. Retry analysis to produce usable evidence."
}

func readingChooserStateLabel(state readingChooserState) string {
	switch state {
	case readingChooserInProgress:
		return "Analysis in progress"
	case readingChooserUpdating:
		return "Updating"
	case readingChooserNeedsAttention:
	}
	return "Not assessed"
}

func readingChooserTitle(view readingChooserPageView) string {
	if strings.TrimSpace(view.LanguageLabel) == "" {
		return "Choose a To Read book"
	}
	return "Choose a To Read book in " + view.LanguageLabel
}

func readingChooserPageTitle(view readingChooserPageView) string {
	if view.CurrentBookID != "" {
		return "Switch current reading"
	}
	return readingChooserTitle(view)
}

func readingChooserPageDescription(view readingChooserPageView) string {
	if view.CurrentBookID != "" {
		return "Choose another To Read Book. Your current Book will return to To Read, so you can come back to it later. Coverage groups describe your current vocabulary, not what you ought to read next."
	}
	return "Choose a To Read book when you are ready. Coverage groups describe current vocabulary evidence, not difficulty or reading recommendations."
}

func readingChooserCandidateCount(view readingChooserPageView) int {
	return len(view.At99Plus) + len(view.At97To99) + len(view.At95To97) + len(view.Below95) + len(view.NoComparison) + len(view.Updating) + len(view.InProgress) + len(view.Attention)
}

func readingChooserNextMarkerText(coverage domain.AnalysisCoverage, band domain.CoverageBand) string {
	marker, hasNextMarker := readingChooserNextMarkers[band]
	if !hasNextMarker {
		if band == domain.CoverageBand99Plus {
			return "At least 99% of the words in this Book are already Known."
		}
		if band == domain.CoverageBandNoComparison {
			return "No words to compare for coverage."
		}
		return "The number of words to the next band is unavailable."
	}
	for _, threshold := range coverage.Thresholds {
		if threshold.TargetPercent == marker {
			return readingChooserThresholdText(threshold)
		}
	}
	return fmt.Sprintf("The number of words to reach %d%% is unavailable.", marker)
}

var readingChooserNextMarkers = map[domain.CoverageBand]int{
	domain.CoverageBandBelow95: 95,
	domain.CoverageBand95To97:  97,
	domain.CoverageBand97To99:  99,
}

func readingChooserThresholdText(threshold domain.CoverageThreshold) string {
	if !threshold.Reachable {
		return fmt.Sprintf("To reach %d%%: not possible with the words available for review.", threshold.TargetPercent)
	}
	return fmt.Sprintf("To reach %d%%: %s more distinct words marked Known.", threshold.TargetPercent, readingChooserCount(threshold.LemmaCount))
}

func readingChooserAnalysisJobURL(jobID int64) string {
	if jobID <= 0 {
		return "/library?disposition=to_read"
	}
	return fmt.Sprintf("/jobs/%d", jobID)
}

func readingChooserCoverageDisplay(coverage domain.AnalysisCoverage) string {
	if coverage.AnalyzableTokenCount <= 0 {
		return "No words to compare"
	}
	return fmt.Sprintf("%.1f%%", min(float64(coverage.KnownTokenCount)*100/float64(coverage.AnalyzableTokenCount), 100))
}

// The learner-facing interface is English; group evidence counts in that locale.
func readingChooserCount(count int64) string {
	return message.NewPrinter(language.English).Sprintf("%d", count)
}
