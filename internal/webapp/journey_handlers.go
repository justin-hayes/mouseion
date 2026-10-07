package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const analysisPublicationPendingDescription = "Analysis completed, but publication is pending or failed. This Book is not shown as analyzed until its result is published; retry analysis to finish publication."

type journeyBookView struct {
	Book                   domain.SourceMaterialSummary
	BookID                 string
	Cover                  domain.BookCover
	ReadingSince           time.Time
	Position               int
	PrimaryGoal            bool
	GoalReadingOnly        bool
	GoalUnassessed         bool
	GoalVocabularyEligible int
	GoalSnapshotID         string
	GoalSnapshotSize       int
	GoalPreparation        *domain.DeckPreparation
	GoalDeckMissing        bool
	GoalDeckUnavailable    bool
	CanMoveEarlier         bool
	CanMoveLater           bool
	CanChooseGoal          bool
	GoalEligibilityReason  string
	Coverage               *domain.AnalysisCoverage
	StatisticsUnavailable  bool
}

func journeyBookClass(primary bool) string {
	if primary {
		return "resource-card journey-book journey-book--goal"
	}
	return "resource-card journey-book"
}

func journeyBookTitlePageClass(primary bool) string {
	if primary {
		return "journey-book__title-page journey-book__title-page--current"
	}
	return "journey-book__title-page"
}

func journeyBookAnchorID(bookID string) string {
	if bookID == "" {
		return ""
	}
	return "journey-book-" + bookID
}

func journeyBookID(item journeyBookView) string {
	if item.BookID != "" {
		return item.BookID
	}
	return item.Book.Source.ID
}

func readingReanalyzeURL(bookID string) string {
	return "/reading/books/" + url.PathEscape(bookID) + "/reanalyze"
}

func readingBookURL(bookID string) string {
	return "/reading#" + url.PathEscape(journeyBookAnchorID(bookID))
}

func canonicalBookTitle(book domain.SourceMaterialSummary) string {
	if strings.TrimSpace(book.BookTitle) != "" {
		return book.BookTitle
	}
	if strings.TrimSpace(book.Source.Title) != "" {
		return book.Source.Title
	}
	if book.BookID != "" {
		return book.BookID
	}
	return book.Source.ID
}

func canonicalBookAuthor(book domain.SourceMaterialSummary) string {
	return strings.TrimSpace(book.BookAuthor)
}

func goalSectionFocusID(bookID string) string {
	// On full-page renders no HTMX swap will run, so an empty focus target keeps
	// the section free of a stale data-focus-id that could redirect attention
	// during a later provisional-list swap. Only HTMX responses name a target.
	if bookID == "" {
		return ""
	}
	return journeyBookAnchorID(bookID)
}

func journeyEvidenceState(item journeyBookView) string {
	status := strings.ToLower(strings.TrimSpace(item.Book.AnalysisStatus))
	if strings.Contains(status, "failed") {
		return "failed"
	}
	switch item.Book.EvidenceState() {
	case domain.BookStale:
		return "stale"
	case domain.BookAcquiredUnassessed:
		return "incomplete"
	case domain.BookNotAcquired, domain.BookUnavailable:
		return "unavailable"
	case domain.BookAnalyzed:
		if item.StatisticsUnavailable || item.Coverage == nil {
			return "incomplete"
		}
		return "current"
	default: // Unknown evidence states are not treated as current.
		return "unavailable"
	}
}

func journeyEvidenceLabel(item journeyBookView) string {
	switch journeyEvidenceState(item) {
	case "stale":
		return "Stale evidence"
	case "incomplete":
		return "Incomplete evidence"
	case "failed":
		return "Analysis failed"
	case "unavailable":
		return "Evidence unavailable"
	default:
		return ""
	}
}

func journeyEvidenceDescription(item journeyBookView) string {
	switch journeyEvidenceState(item) {
	case "stale":
		return "The current content no longer matches this analysis. Re-analyze the book to refresh its evidence."
	case "incomplete":
		if strings.EqualFold(strings.TrimSpace(item.Book.AnalysisState), "completed") {
			return analysisPublicationPendingDescription
		}
		return "A completed analysis has not produced usable coverage for this book yet."
	case "failed":
		return "The last analysis failed. Retry analysis to produce current evidence."
	case "unavailable":
		return "Current book content is unavailable, so coverage cannot be calculated."
	default:
		return ""
	}
}

func journeyAnalysisAction(item journeyBookView) bookLifecycleAction {
	bookID := journeyBookID(item)
	if !strings.EqualFold(strings.TrimSpace(item.Book.Source.MediaType), opds.EPUBMediaType) || strings.TrimSpace(item.Book.Source.ContentRevisionID) == "" || strings.TrimSpace(item.Book.Source.ContentSnapshotID) == "" {
		return bookLifecycleAction{
			Status:      "Assessment unavailable",
			Description: "No current EPUB content is available for this book in Reading. Retry acquisition when the catalog can provide it.",
			Label:       "Retry acquisition",
			URL:         readingReanalyzeURL(bookID),
			Submit:      true,
			Tone:        StatusWarning,
		}
	}
	action := bookLifecycleActionFor(item.Book)
	evidenceState := journeyEvidenceState(item)
	if evidenceState == "failed" || (evidenceState == "incomplete" && action.Status != "Analysis queued" && action.Status != "Analysis running") {
		action.Status = "Analysis incomplete"
		action.Description = journeyEvidenceDescription(item)
		action.Label = "Retry analysis"
		action.URL = readingReanalyzeURL(bookID)
		action.Submit = true
		action.Tone = StatusWarning
		if journeyEvidenceState(item) == "failed" {
			action.Status = "Analysis failed"
			action.Tone = StatusDanger
		}
	} else if action.Status == "Analysis not started" {
		action.Label = "Retry analysis"
		action.URL = readingReanalyzeURL(bookID)
		action.Submit = true
	} else if action.Status == "Analysis result ready" && bookHasCompletedAnalysis(item.Book) {
		action.URL = readingBookURL(bookID)
		action.Label = "View in Reading"
	}
	return action
}

func journeyGoalEligibility(book domain.SourceMaterialSummary) (bool, string) {
	switch book.GoalEligibility() {
	case domain.GoalNeedsCurrentContent:
		return false, "This book cannot be started until current EPUB content is available."
	case domain.GoalAnalysisInProgress:
		return false, "This book cannot be started while its current analysis is still in progress."
	case domain.GoalFailed:
		return false, "This book cannot be started until its failed analysis is retried successfully."
	case domain.GoalCancelled:
		return false, "This book cannot be started until its cancelled analysis is retried successfully."
	case domain.GoalStale:
		return false, "This book cannot be started until its analysis matches the current content."
	case domain.GoalNoCompletedAnalysis:
		return false, "This book needs a successfully completed current analysis before it can be started."
	case domain.GoalEligible:
		return true, ""
	default:
		return false, "This book's Goal eligibility is unavailable."
	}
}

func journeyCurrentCoverage(item journeyBookView) string {
	if item.Coverage == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f%%", knownCoveragePercent(*item.Coverage))
}

type journeyPageView struct {
	Language            string
	LanguageLabel       string
	ActiveLanguageLabel string
	LanguageHandoff     *journeyLanguageHandoffView
	Goal                *journeyBookView
	Provisional         []journeyBookView
}

type journeyLanguageHandoffView struct {
	BookID        string
	BookTitle     string
	Language      string
	LanguageLabel string
}

func journeyLanguageHandoffURL(bookID, language string) string {
	query := url.Values{}
	query.Set("language", language)
	query.Set("language_handoff_book", bookID)
	query.Set("language_handoff_language", language)
	return "/reading?" + query.Encode()
}

type deckJourneyState string

const (
	deckJourneyUnknown   deckJourneyState = ""
	deckJourneyNotMember deckJourneyState = "not-member"
	deckIsToRead         deckJourneyState = "member"
	deckIsCurrentReading deckJourneyState = "primary-goal"
)

type deckJourneyActionView struct {
	BookID  string
	State   deckJourneyState
	Message string
	Error   string
}

func emptyDeckJourneyAction() deckJourneyActionView {
	return deckJourneyActionView{State: deckJourneyUnknown}
}

func deckJourneyActionID(bookID string) string {
	return "deck-preparation-journey-action-" + bookID
}

func (h *Handler) deckJourneyStatus(ctx context.Context, owner, bookID string) (deckJourneyActionView, error) {
	// Deck preparation surfaces are keyed by source_materials.id while Journey
	// membership and the current reading are keyed by books.id, so every action
	// identity is resolved to its canonical book first. A source material with
	// no book identity cannot join the Journey, so no action is offered.
	resolved, ok, err := h.resolveBookID(ctx, owner, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	if !ok {
		return deckJourneyActionView{}, nil
	}
	bookID = resolved
	language, _ := activeStudyLanguageForContext(ctx)
	goal, err := h.currentReading(ctx, owner, language)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	action := deckJourneyActionView{BookID: bookID, State: deckJourneyNotMember}
	if goal.IsActive() && goal.BookID == bookID {
		action.State = deckIsCurrentReading
		return action, nil
	}
	if h.services.Store.Books == nil {
		return action, nil
	}
	detail, err := h.services.Store.Books.GetBookDetail(ctx, owner, bookID)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return deckJourneyActionView{}, err
	}
	if err == nil && detail.Disposition == domain.BookDispositionToRead {
		action.State = deckIsToRead
	}
	return action, nil
}

func (h *Handler) resolveBookID(ctx context.Context, owner, id string) (string, bool, error) {
	resolver, ok := h.services.Store.Books.(interface {
		ResolveBookID(context.Context, string, string) (string, bool, error)
	})
	if !ok {
		return "", false, nil
	}
	return resolver.ResolveBookID(ctx, owner, id)
}

func (h *Handler) ensureToReadAnalysis(ctx context.Context, owner, bookID string) (analysis.Handle, cataloguesync.AcquisitionTarget, string, bool, error) {
	detail, err := h.services.Store.Books.GetBookDetail(ctx, owner, bookID)
	if err != nil {
		return analysis.Handle{}, cataloguesync.AcquisitionTarget{}, "", false, err
	}
	target := cataloguesync.AcquisitionTarget{}
	acquisitionAttempted := false
	if detail.Acquired == nil || detail.Acquired.EvidenceState() == domain.BookUnavailable {
		// A missing or unavailable source cannot meet current-reading eligibility,
		// which requires a completed current analysis. It cannot become current
		// during acquisition; the guarded submission below serializes analyzable
		// books against a concurrent start.
		acquisitionAttempted = true
		target, err = h.acquireBookForJourneyContext(ctx, owner, bookID)
		if err != nil {
			return analysis.Handle{}, target, detail.Book.Title, true, err
		}
		detail, err = h.services.Store.Books.GetBookDetail(ctx, owner, bookID)
		if err != nil {
			return analysis.Handle{}, target, detail.Book.Title, true, err
		}
	}
	if detail.Acquired == nil {
		return analysis.Handle{}, target, detail.Book.Title, acquisitionAttempted, errors.New("the current EPUB could not be loaded after acquisition")
	}
	handle, err := h.services.Analysis.SubmitToReadBookAnalysis(ctx, owner, bookID, detail.Acquired.Source.ID)
	return handle, target, detail.Book.Title, false, err
}

func toReadAnalysisError(ctx context.Context, catalog CatalogStore, owner, bookID, title string, target cataloguesync.AcquisitionTarget, acquisitionFailed bool, err error) string {
	if acquisitionFailed {
		return "Book moved to To Read, but " + journeyAcquisitionError(ctx, catalog, owner, bookID, title, target, err) + ". The To Read status is retained; analysis is unavailable until the current EPUB can be acquired."
	}
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) || errors.Is(err, analysis.ErrEPUBRequired) {
		return "Book moved to To Read, but the current source has no usable EPUB units. The To Read status is retained; analysis is unavailable."
	}
	return "Book moved to To Read, but analysis could not start. Retry analysis from Reading when ready."
}

func (h *Handler) annotateMyBooksWithDisposition(ctx context.Context, owner string, books []domain.MyBook) error {
	language, _ := activeStudyLanguageForContext(ctx)
	goal, err := h.currentReading(ctx, owner, language)
	if err != nil {
		return err
	}
	goalBookID := ""
	if goal.IsActive() {
		goalBookID = goal.BookID
	}
	for i := range books {
		books[i].IsToRead = books[i].Disposition == domain.BookDispositionToRead
		books[i].IsCurrentReading = books[i].Book.ID == goalBookID
		if books[i].Acquired != nil {
			books[i].Acquired.IsToRead = books[i].IsToRead
			books[i].Acquired.IsCurrentReading = books[i].IsCurrentReading
		}
	}
	return nil
}

func (h *Handler) annotateBookToRead(ctx context.Context, owner string, book *domain.MyBook) error {
	language, _ := activeStudyLanguageForContext(ctx)
	return h.annotateBookToReadLanguage(ctx, owner, language, book)
}

func (h *Handler) annotateBookToReadLanguage(ctx context.Context, owner, language string, book *domain.MyBook) error {
	goal, err := h.currentReading(ctx, owner, language)
	if err != nil {
		return err
	}
	book.IsToRead = book.Disposition == domain.BookDispositionToRead
	book.IsCurrentReading = goal.IsActive() && goal.BookID == book.Book.ID
	if book.Acquired != nil {
		book.Acquired.IsToRead = book.IsToRead
		book.Acquired.IsCurrentReading = book.IsCurrentReading
	}
	return nil
}

func (h *Handler) currentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error) {
	if h.services.Store.CurrentReading != nil {
		return h.services.Store.CurrentReading.GetCurrentReading(ctx, owner, language)
	}
	return h.services.Store.Goals.GetPrimaryGoal(ctx, owner, language)
}

func (h *Handler) journeyLanguageHandoff(ctx context.Context, owner, activeLanguage, bookID, targetLanguage string) (journeyLanguageHandoffView, bool, error) {
	bookID = strings.TrimSpace(bookID)
	targetLanguage = canonicalization.NormalizeLanguage(targetLanguage)
	if bookID == "" || targetLanguage == "" || canonicalization.NormalizeLanguage(activeLanguage) == targetLanguage {
		return journeyLanguageHandoffView{}, false, nil
	}
	detail, err := h.services.Store.Books.GetBookDetail(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return journeyLanguageHandoffView{}, false, nil
	}
	if err != nil {
		return journeyLanguageHandoffView{}, false, err
	}
	if detail.Acquired == nil || detail.Acquired.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(*detail.Acquired) {
		return journeyLanguageHandoffView{}, false, nil
	}
	if canonicalization.NormalizeLanguage(bookStudyLanguage(detail)) != targetLanguage {
		return journeyLanguageHandoffView{}, false, nil
	}
	if detail.Disposition != domain.BookDispositionToRead {
		return journeyLanguageHandoffView{}, false, nil
	}
	label := targetLanguage
	if shell := shellViewFromContext(ctx); shell != nil {
		for _, option := range shell.Options {
			if option.Language == targetLanguage {
				label = option.DisplayName
				break
			}
		}
	}
	title := strings.TrimSpace(detail.Book.Title)
	if title == "" && detail.Acquired != nil {
		title = strings.TrimSpace(detail.Acquired.Source.Title)
	}
	return journeyLanguageHandoffView{BookID: detail.Book.ID, BookTitle: title, Language: targetLanguage, LanguageLabel: label}, true, nil
}

func (h *Handler) buildJourneyView(ctx context.Context, owner, language string) (journeyPageView, error) {
	goal, err := h.services.Store.Goals.GetPrimaryGoal(ctx, owner, language)
	if err != nil {
		return journeyPageView{}, err
	}
	books, err := h.services.Store.Books.ListSourceMaterials(ctx, owner)
	if err != nil {
		return journeyPageView{}, err
	}
	bookByID := make(map[string]domain.SourceMaterialSummary, len(books))
	coverByBookID := make(map[string]domain.BookCover)
	for _, book := range books {
		bookByID[book.Source.ID] = book
	}
	if reader, ok := h.services.Store.Books.(interface {
		ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	}); ok {
		myBooks, readErr := reader.ListMyBooksWithEvidence(ctx, owner)
		if readErr != nil {
			return journeyPageView{}, readErr
		}
		for _, myBook := range myBooks {
			coverByBookID[myBook.Book.ID] = myBook.Cover
			if myBook.Acquired != nil {
				book := *myBook.Acquired
				book.BookTitle = myBook.Book.Title
				book.BookAuthor = myBook.Book.Author
				bookByID[myBook.Book.ID] = book
				bookByID[book.Source.ID] = book
				coverByBookID[book.Source.ID] = myBook.Cover
			} else {
				bookByID[myBook.Book.ID] = domain.SourceMaterialSummary{
					Source:     domain.SourceMaterial{ID: myBook.Book.ID, OwnerID: myBook.Book.OwnerID, Title: myBook.Book.Title, Language: myBook.Book.LanguageTag},
					BookTitle:  myBook.Book.Title,
					BookAuthor: myBook.Book.Author,
					BookID:     myBook.Book.ID,
				}
			}
		}
	}

	view := journeyPageView{}
	if goal.IsActive() {
		book, bookErr := h.journeyBook(ctx, owner, goal.BookID, bookByID)
		if bookErr != nil {
			return journeyPageView{}, bookErr
		}
		book.PrimaryGoal = true
		book.Cover = coverByBookID[goal.BookID]
		book.ReadingSince = goal.CreatedAt
		if err = h.addJourneyEvidence(ctx, owner, &book); err != nil {
			return journeyPageView{}, err
		}
		book.GoalUnassessed = book.Book.EvidenceState() != domain.BookAnalyzed
		book.GoalVocabularyEligible, err = h.services.Store.Goals.CountPrimaryGoalVocabularyToGraduate(ctx, owner, language)
		if err != nil {
			return journeyPageView{}, err
		}
		book.GoalReadingOnly = book.GoalVocabularyEligible == 0
		book.GoalSnapshotID = goal.SnapshotID
		book.GoalSnapshotSize = goal.SnapshotSize
		h.addGoalDeckPreparation(ctx, owner, &book, goal)
		view.Goal = &book
	}
	toReadIDs := make([]string, 0, len(coverByBookID))
	if reader, ok := h.services.Store.Books.(interface {
		ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	}); ok {
		myBooks, readErr := reader.ListMyBooksWithEvidence(ctx, owner)
		if readErr != nil {
			return journeyPageView{}, readErr
		}
		for _, myBook := range myBooks {
			if myBook.Disposition == domain.BookDispositionToRead && myBook.Book.LanguageTag == language && myBook.Book.ID != goal.BookID {
				toReadIDs = append(toReadIDs, myBook.Book.ID)
			}
		}
	}
	sort.Slice(toReadIDs, func(i, j int) bool {
		left, right := canonicalBookTitle(bookByID[toReadIDs[i]]), canonicalBookTitle(bookByID[toReadIDs[j]])
		if left == right {
			return toReadIDs[i] < toReadIDs[j]
		}
		return left < right
	})
	for _, bookID := range toReadIDs {
		if bookID == goal.BookID {
			continue
		}
		book, bookErr := h.journeyBook(ctx, owner, bookID, bookByID)
		if bookErr != nil {
			return journeyPageView{}, bookErr
		}
		book.Cover = coverByBookID[bookID]
		if err = h.addJourneyEvidence(ctx, owner, &book); err != nil {
			return journeyPageView{}, err
		}
		book.CanChooseGoal, book.GoalEligibilityReason = journeyGoalEligibility(book.Book)
		view.Provisional = append(view.Provisional, book)
	}
	return view, nil
}

func (h *Handler) addGoalDeckPreparation(ctx context.Context, owner string, book *journeyBookView, goal domain.PrimaryGoal) {
	if goal.SnapshotSize == 0 {
		return
	}
	book.GoalDeckUnavailable = true
	reader, ok := h.services.PreparedDeck.(PreparedDeckForGoalSnapshot)
	if !ok {
		return
	}
	preparation, err := reader.GetForGoalSnapshot(ctx, owner, goal.SnapshotID)
	switch {
	case err == nil:
		if !currentReadingPreparationMatches(preparation, owner, goal) {
			log.Printf("mouseion: Goal deck provenance mismatch for owner %s snapshot %s", owner, goal.SnapshotID)
			return
		}
		book.GoalPreparation = &preparation
		book.GoalDeckUnavailable = false
	case errors.Is(err, persistence.ErrNotFound):
		// The Goal remains visible while an unavailable artifact is retried through
		// the exact snapshot identity.
		book.GoalDeckMissing = true
	default:
		log.Printf("mouseion: Goal deck unavailable for owner %s snapshot %s: %v", owner, goal.SnapshotID, err)
	}
}

func (h *Handler) journeyBook(ctx context.Context, owner, bookID string, bookByID map[string]domain.SourceMaterialSummary) (journeyBookView, error) {
	if book, ok := bookByID[bookID]; ok {
		return journeyBookView{Book: book, BookID: bookID}, nil
	}
	// Goal and To Read status are allowed to exist before acquisition. Keep
	// that identity visible instead of silently dropping it from the surface.
	book, err := h.services.Store.Books.GetBook(ctx, owner, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: bookID, Title: "Book details unavailable", OwnerID: owner}}, BookID: bookID}, nil
		}
		return journeyBookView{}, err
	}
	return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: book.ID, OwnerID: owner, Title: book.Title, Language: book.LanguageTag}, BookTitle: book.Title, BookAuthor: book.Author}, BookID: book.ID}, nil
}

func (h *Handler) addJourneyEvidence(ctx context.Context, owner string, book *journeyBookView) error {
	state := book.Book.EvidenceState()
	if state == domain.BookStale {
		return nil
	}
	if state != domain.BookAnalyzed || book.Book.CorpusID == "" || h.services.AnalysisInsights == nil {
		book.StatisticsUnavailable = state == domain.BookAnalyzed
		return nil
	}
	coverage, err := h.services.AnalysisInsights.Coverage(ctx, owner, book.Book.CorpusID)
	if errors.Is(err, analysisinsights.ErrStatisticsUnavailable) {
		book.StatisticsUnavailable = true
		return nil
	}
	if err != nil {
		return err
	}
	book.Coverage = &coverage
	return nil
}
