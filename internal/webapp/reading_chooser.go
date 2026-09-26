package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
)

type readingChooserBookView struct {
	Book        domain.MyBook
	Coverage    *domain.AnalysisCoverage
	Band        domain.CoverageBand
	State       readingChooserState
	Description string
}

type readingChooserState string

const (
	readingChooserInProgress     readingChooserState = "in_progress"
	readingChooserNeedsAttention readingChooserState = "needs_attention"
)

type transactionalCurrentReadingStarter interface {
	CreatePrimaryGoalWith(context.Context, string, string, string, func(context.Context, pgx.Tx, domain.PrimaryGoal) error) (domain.PrimaryGoal, error)
}

type transactionalGoalDeckPreparer interface {
	SubmitForGoalTx(context.Context, pgx.Tx, string, string, string) (prepareddeck.Handle, error)
}

type readingChooserPageView struct {
	Language, LanguageLabel          string
	ActiveLanguageLabel              string
	LanguageHandoff                  *journeyLanguageHandoffView
	CurrentBookID, CurrentSnapshotID string
	At99Plus, At97To99               []readingChooserBookView
	At95To97, Below95                []readingChooserBookView
	NoComparison                     []readingChooserBookView
	InProgress, Attention            []readingChooserBookView
}

func (h *Handler) reading(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	activeLanguage := language
	activeLanguageLabel := languageLabel
	if requested := strings.TrimSpace(r.URL.Query().Get("language")); requested != "" {
		if view := shellViewFromContext(r.Context()); view != nil {
			for _, option := range view.Options {
				if option.HasBooks && strings.EqualFold(option.Language, requested) {
					language, languageLabel = option.Language, option.DisplayName
					break
				}
			}
		}
	}
	handoff, hasHandoff, handoffErr := h.journeyLanguageHandoff(r.Context(), owner.ID, activeLanguage, r.URL.Query().Get("language_handoff_book"), r.URL.Query().Get("language_handoff_language"))
	if handoffErr != nil {
		fail(w, handoffErr)
		return
	}
	goal, err := h.services.Store.Goals.GetPrimaryGoal(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	if goal.IsActive() {
		journey, buildErr := h.buildJourneyView(r.Context(), owner.ID, language)
		if buildErr != nil {
			fail(w, buildErr)
			return
		}
		journey.Language = language
		journey.LanguageLabel = languageLabel
		journey.ActiveLanguageLabel = activeLanguageLabel
		if hasHandoff {
			journey.LanguageHandoff = &handoff
		}
		render(w, r, JourneyPage(owner, h.csrf(w, r), journey, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
		return
	}
	view, err := h.buildReadingChooser(r.Context(), owner.ID, language, languageLabel)
	if err != nil {
		fail(w, err)
		return
	}
	if hasHandoff {
		view.LanguageHandoff = &handoff
	}
	view.ActiveLanguageLabel = activeLanguageLabel
	render(w, r, ReadingChooserPage(owner, h.csrf(w, r), view, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
}

func (h *Handler) switchReadingPage(w http.ResponseWriter, r *http.Request) {
	owner := user(r).ID
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	current, err := h.services.Store.CurrentReading.GetCurrentReading(r.Context(), owner, language)
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
	if language == "" || expectedBookID == "" || expectedSnapshotID == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape("Choose a study language and refresh Reading before switching books."))
		return
	}
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if detail.Disposition != domain.BookDispositionToRead || detail.Book.LanguageTag != language {
		redirect(w, r, "/reading?error="+url.QueryEscape("This book is no longer an eligible To Read choice. No changes were made; refresh Reading and try again."))
		return
	}
	current, err := h.services.Store.CurrentReading.GetCurrentReading(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	if current.BookID == bookID {
		redirect(w, r, "/reading?message="+url.QueryEscape(h.goalBookTitle(r.Context(), owner, bookID)+" is already your current reading."))
		return
	}
	if current.BookID != expectedBookID || current.SnapshotID != expectedSnapshotID {
		redirect(w, r, "/reading?error="+url.QueryEscape("The current book changed while you were choosing. No changes were made; review Reading before trying again."))
		return
	}
	selected, err := h.services.Store.CurrentReading.SwitchCurrentReading(r.Context(), owner, language, bookID, expectedBookID, expectedSnapshotID)
	if errors.Is(err, persistence.ErrCurrentReadingStale) {
		redirect(w, r, "/reading?error="+url.QueryEscape("The current book changed while you were choosing. No changes were made; review Reading before trying again."))
		return
	}
	if errors.Is(err, persistence.ErrCurrentReadingIneligible) {
		redirect(w, r, "/reading?error="+url.QueryEscape("This book no longer has current analyzed content or is no longer To Read. No changes were made; refresh Reading and try again."))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if selected.AnalysisRunID != "" && selected.SnapshotSize > 0 && h.services.PreparedDeck != nil {
		if _, prepareErr := h.services.PreparedDeck.SubmitForGoal(r.Context(), owner, selected.AnalysisRunID, selected.SnapshotID); prepareErr != nil {
			log.Printf("current reading switch deck preparation owner=%s language=%s book=%s: %v", owner, language, selected.BookID, prepareErr)
		}
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(h.goalBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
}

func (h *Handler) stopReading(w http.ResponseWriter, r *http.Request) {
	h.transitionCurrentReading(w, r, false)
}

func (h *Handler) setAsideCurrentReading(w http.ResponseWriter, r *http.Request) {
	h.transitionCurrentReading(w, r, true)
}

func (h *Handler) transitionCurrentReading(w http.ResponseWriter, r *http.Request, setAside bool) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	language, _ := activeStudyLanguageForContext(r.Context())
	expectedBookID := strings.TrimSpace(r.FormValue("expected_current_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if language == "" || expectedBookID == "" || expectedSnapshotID == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape("Choose a study language and refresh Reading before changing the current book."))
		return
	}
	current, err := h.services.Store.CurrentReading.GetCurrentReading(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	disposition := domain.BookDispositionToRead
	message := "Reading paused. The book remains To Read; its analysis and history are preserved."
	if setAside {
		disposition = domain.BookDispositionSetAside
		message = "Book set aside. Its analysis, deck, and history are preserved."
	}
	if !current.IsActive() {
		actual, dispositionErr := h.services.Store.Books.GetBookDetail(r.Context(), owner, expectedBookID)
		if dispositionErr != nil {
			fail(w, dispositionErr)
			return
		}
		if actual.Disposition == disposition {
			redirect(w, r, "/reading?message="+url.QueryEscape(message))
			return
		}
	}
	if current.BookID != expectedBookID || (current.IsActive() && current.SnapshotID != expectedSnapshotID) {
		redirect(w, r, "/reading?error="+url.QueryEscape("The current book changed before this action. No changes were made; review Reading and try again."))
		return
	}
	if setAside {
		err = h.services.Store.CurrentReading.SetAsideCurrentReading(r.Context(), owner, language, expectedBookID, expectedSnapshotID)
	} else {
		err = h.services.Store.CurrentReading.StopCurrentReading(r.Context(), owner, language, expectedBookID, expectedSnapshotID)
	}
	if errors.Is(err, persistence.ErrCurrentReadingStale) {
		redirect(w, r, "/reading?error="+url.QueryEscape("The current book changed before this action. No changes were made; review Reading and try again."))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(message))
}

func (h *Handler) reanalyzeToReadBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if detail.Disposition != domain.BookDispositionToRead {
		http.NotFound(w, r)
		return
	}
	handle, _, _, _, err := h.ensureJourneyAnalysis(r.Context(), owner, bookID)
	if err != nil {
		redirect(w, r, "/reading?error="+url.QueryEscape("Acquisition or analysis could not be started. The To Read choice is retained. Review current book content in My Books and try again."))
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(fmt.Sprintf("Analysis job #%d submitted.", handle.DisplayNumber)))
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
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if detail.Disposition != domain.BookDispositionToRead || detail.Book.LanguageTag != language {
		redirect(w, r, "/reading?error="+url.QueryEscape("This book is no longer an eligible To Read candidate. Review Reading before trying again."))
		return
	}
	current, err := h.services.Store.Goals.GetPrimaryGoal(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	if current.IsActive() && current.BookID != bookID {
		redirect(w, r, "/reading?error="+url.QueryEscape("A current book is already set for this language. Review it in Reading before starting another."))
		return
	}
	selected := current
	queuedWithSnapshot := false
	if !current.IsActive() {
		if starter, ok := h.services.Store.Goals.(transactionalCurrentReadingStarter); ok {
			if deck, canQueueAtomically := h.services.PreparedDeck.(transactionalGoalDeckPreparer); canQueueAtomically {
				selected, err = starter.CreatePrimaryGoalWith(r.Context(), owner, language, bookID, func(ctx context.Context, tx pgx.Tx, reading domain.PrimaryGoal) error {
					if reading.SnapshotSize == 0 {
						return nil
					}
					_, queueErr := deck.SubmitForGoalTx(ctx, tx, owner, reading.AnalysisRunID, reading.SnapshotID)
					return queueErr
				})
				queuedWithSnapshot = err == nil && selected.SnapshotSize > 0
			} else {
				selected, err = h.services.Store.Goals.CreatePrimaryGoal(r.Context(), owner, language, bookID)
			}
		} else {
			selected, err = h.services.Store.Goals.CreatePrimaryGoal(r.Context(), owner, language, bookID)
		}
		if errors.Is(err, persistence.ErrGoalExists) {
			redirect(w, r, "/reading?error="+url.QueryEscape("Another book became current while you were choosing. Review Reading before trying again."))
			return
		}
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, persistence.ErrGoalIneligible) {
			redirect(w, r, "/reading?error="+url.QueryEscape("This book no longer has trustworthy current analysis or is no longer To Read. No changes were made; refresh Reading and try again."))
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
	}
	if selected.AnalysisRunID != "" && selected.SnapshotSize > 0 && !queuedWithSnapshot {
		if h.services.PreparedDeck == nil {
			redirect(w, r, "/reading?error="+url.QueryEscape("The book is current, but local deck preparation is unavailable. Its snapshot is preserved; retry preparation when the service is available."))
			return
		}
		if _, prepareErr := h.services.PreparedDeck.SubmitForGoal(r.Context(), owner, selected.AnalysisRunID, selected.SnapshotID); prepareErr != nil {
			log.Printf("current reading deck preparation owner=%s language=%s book=%s: %v", owner, language, selected.BookID, prepareErr)
			redirect(w, r, "/reading?error="+url.QueryEscape("The book is current and its snapshot is frozen, but local deck preparation could not be queued. The reading is unchanged; open its deck task to retry."))
			return
		}
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(h.goalBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
}

func (h *Handler) buildReadingChooser(ctx context.Context, owner, language, languageLabel string) (readingChooserPageView, error) {
	reader, ok := h.services.Store.Books.(interface {
		ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	})
	if !ok {
		return readingChooserPageView{}, errors.New("reading chooser is unavailable")
	}
	books, err := reader.ListMyBooksWithEvidence(ctx, owner)
	if err != nil {
		return readingChooserPageView{}, err
	}
	view := readingChooserPageView{Language: language, LanguageLabel: languageLabel}
	for _, book := range books {
		if book.Disposition != domain.BookDispositionToRead || language == "" || book.Book.LanguageTag != language {
			continue
		}
		candidate := readingChooserBookView{Book: book}
		if book.Acquired == nil || book.Acquired.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(*book.Acquired) {
			candidate.State, candidate.Description = readingChooserEvidenceState(book)
			switch candidate.State {
			case readingChooserInProgress:
				view.InProgress = append(view.InProgress, candidate)
			case readingChooserNeedsAttention:
				view.Attention = append(view.Attention, candidate)
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
	for _, group := range [][]readingChooserBookView{view.At99Plus, view.At97To99, view.At95To97, view.Below95, view.NoComparison, view.InProgress, view.Attention} {
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

func readingChooserEvidenceState(book domain.MyBook) (readingChooserState, string) {
	if book.Acquired == nil {
		return readingChooserNeedsAttention, "Book content has not been acquired yet. Return to My Books to review its catalog entry."
	}
	status := strings.ToLower(strings.TrimSpace(book.Acquired.AnalysisStatus))
	if strings.Contains(status, "queued") || strings.Contains(status, "running") || status == "analyzing" {
		return readingChooserInProgress, "Analysis is queued or running. This candidate will appear in a coverage group when current evidence is ready."
	}
	evidenceState := book.Acquired.EvidenceState()
	if evidenceState == domain.BookStale {
		return readingChooserNeedsAttention, "The analysis no longer matches the current book content. Retry analysis to refresh its evidence."
	}
	if evidenceState == domain.BookUnavailable || evidenceState == domain.BookNotAcquired {
		return readingChooserNeedsAttention, "Current book content is unavailable. Retry acquisition or analysis from My Books."
	}
	if strings.Contains(status, "failed") || strings.Contains(status, "cancelled") {
		return readingChooserNeedsAttention, "The last analysis did not complete. Retry analysis to refresh its evidence."
	}
	return readingChooserNeedsAttention, "Current analysis is not complete. Retry analysis to produce usable evidence."
}

func readingChooserStateLabel(state readingChooserState) string {
	if state == readingChooserInProgress {
		return "Analysis in progress"
	}
	return "Needs attention"
}

func readingChooserTitle(view readingChooserPageView) string {
	if strings.TrimSpace(view.LanguageLabel) == "" {
		return "Choose your next book"
	}
	return "Choose your next book in " + view.LanguageLabel
}

func readingChooserPageTitle(view readingChooserPageView) string {
	if view.CurrentBookID != "" {
		return "Switch current reading"
	}
	return readingChooserTitle(view)
}

func readingChooserPageDescription(view readingChooserPageView) string {
	if view.CurrentBookID != "" {
		return "Choose another eligible To Read book in this study language. The switch replaces the current book and its active reservation atomically."
	}
	return "Choose a To Read book when you are ready. Coverage groups describe vocabulary evidence; they are not difficulty ratings or recommendations."
}

func readingChooserCandidateCount(view readingChooserPageView) int {
	return len(view.At99Plus) + len(view.At97To99) + len(view.At95To97) + len(view.Below95) + len(view.NoComparison) + len(view.InProgress) + len(view.Attention)
}

func readingChooserNextMarkerText(coverage domain.AnalysisCoverage, band domain.CoverageBand) string {
	marker, hasNextMarker := readingChooserNextMarkers[band]
	if !hasNextMarker {
		if band == domain.CoverageBand99Plus {
			return "At least 99% of analyzable tokens are already Known."
		}
		if band == domain.CoverageBandNoComparison {
			return "Coverage cannot be compared because there are no analyzable tokens."
		}
		return "Next-marker investment is unavailable."
	}
	for _, threshold := range coverage.Thresholds {
		if threshold.TargetPercent == marker {
			return readingChooserThresholdText(threshold)
		}
	}
	return fmt.Sprintf("Investment to reach %d%% is unavailable.", marker)
}

var readingChooserNextMarkers = map[domain.CoverageBand]int{
	domain.CoverageBandBelow95: 95,
	domain.CoverageBand95To97:  97,
	domain.CoverageBand97To99:  99,
}

func readingChooserThresholdText(threshold domain.CoverageThreshold) string {
	if !threshold.Reachable {
		return fmt.Sprintf("%d%%: not reachable from currently eligible vocabulary.", threshold.TargetPercent)
	}
	return fmt.Sprintf("%d%%: %d additional eligible vocabulary identities.", threshold.TargetPercent, threshold.LemmaCount)
}

func readingChooserAnalysisJobURL(jobID int64) string {
	if jobID <= 0 {
		return "/library?disposition=to_read"
	}
	return fmt.Sprintf("/jobs/%d", jobID)
}

func readingChooserCoverageDisplay(coverage domain.AnalysisCoverage) string {
	if coverage.AnalyzableTokenCount <= 0 {
		return "No analyzable tokens"
	}
	return fmt.Sprintf("%.1f%%", min(float64(coverage.KnownTokenCount)*100/float64(coverage.AnalyzableTokenCount), 100))
}
