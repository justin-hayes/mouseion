package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const analysisPublicationPendingDescription = "Analysis completed, but publication is pending or failed. This Book is not shown as analyzed until its result is published; retry analysis to finish publication."

type readingBookView struct {
	Book                             domain.SourceMaterialSummary
	BookID                           string
	Cover                            domain.BookCover
	ReadingSince                     time.Time
	Position                         int
	CurrentReading                   bool
	ReadingOnly                      bool
	CurrentReadingUnassessed         bool
	CurrentReadingVocabularyEligible int
	CurrentReadingSnapshotID         string
	CurrentReadingSnapshotSize       int
	CurrentReadingPreparation        *domain.DeckPreparation
	CurrentReadingDeckMissing        bool
	CurrentReadingDeckUnavailable    bool
	CanMoveEarlier                   bool
	CanMoveLater                     bool
	CanChooseCurrentReading          bool
	CurrentReadingEligibilityReason  string
	Coverage                         *domain.AnalysisCoverage
	StatisticsUnavailable            bool
}

func readingBookAnchorID(bookID string) string {
	if bookID == "" {
		return ""
	}
	return "journey-book-" + bookID
}

func readingBookID(item readingBookView) string {
	if item.BookID != "" {
		return item.BookID
	}
	return item.Book.Source.ID
}

func readingReanalyzeURL(bookID string) string {
	return "/reading/books/" + url.PathEscape(bookID) + "/reanalyze"
}

func readingBookURL(bookID string) string {
	return "/reading#" + url.PathEscape(readingBookAnchorID(bookID))
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

func currentReadingSectionFocusID(bookID string) string {
	// On full-page renders no HTMX swap will run, so an empty focus target keeps
	// the section free of a stale data-focus-id that could redirect attention
	// during a later provisional-list swap. Only HTMX responses name a target.
	if bookID == "" {
		return ""
	}
	return readingBookAnchorID(bookID)
}

func readingEvidenceState(item readingBookView) string {
	classification := item.Book.EvidenceClassification()
	if classification.Phase == domain.PhaseFailed {
		return "failed"
	}
	switch classification.Evidence {
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

func readingEvidenceLabel(item readingBookView) string {
	switch readingEvidenceState(item) {
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

func readingEvidenceDescription(item readingBookView) string {
	switch readingEvidenceState(item) {
	case "stale":
		return "The current content no longer matches this analysis. Re-analyze the book to refresh its evidence."
	case "incomplete":
		if item.Book.EvidenceClassification().RunFinished() {
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

func reservedVocabularySummary(count int) string {
	if count == 1 {
		return "1 lemma is set aside from vocabulary selection while you read this Book."
	}
	return fmt.Sprintf("%d lemmas are set aside from vocabulary selection while you read this Book.", count)
}

func readingAnalysisAction(item readingBookView) bookLifecycleAction {
	bookID := readingBookID(item)
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
	evidenceState := readingEvidenceState(item)
	if evidenceState == "failed" || (evidenceState == "incomplete" && action.Status != "Analysis queued" && action.Status != "Analysis running") {
		action.Status = "Analysis incomplete"
		action.Description = readingEvidenceDescription(item)
		action.Label = "Retry analysis"
		action.URL = readingReanalyzeURL(bookID)
		action.Submit = true
		action.Tone = StatusWarning
		if readingEvidenceState(item) == "failed" {
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

func readingCurrentReadingEligibility(book domain.SourceMaterialSummary) (bool, string) {
	switch book.EvidenceClassification().Eligibility {
	case domain.CurrentReadingNotToRead:
		return false, "This book cannot be started until it is in To Read."
	case domain.CurrentReadingNoChosenLanguage:
		return false, "This book cannot be started until its language is chosen."
	case domain.CurrentReadingNeedsCurrentContent:
		return false, "This book cannot be started until current EPUB content is available."
	case domain.CurrentReadingAnalysisInProgress:
		return false, "This book cannot be started while its current analysis is still in progress."
	case domain.CurrentReadingFailed:
		return false, "This book cannot be started until its failed analysis is retried successfully."
	case domain.CurrentReadingCancelled:
		return false, "This book cannot be started until its cancelled analysis is retried successfully."
	case domain.CurrentReadingStale:
		return false, "This book cannot be started until its analysis matches the current content."
	case domain.CurrentReadingNoCompletedAnalysis:
		return false, "This book needs a successfully completed current analysis before it can be started."
	case domain.CurrentReadingEligible:
		return true, ""
	default:
		return false, "This book's Goal eligibility is unavailable."
	}
}

type readingPageView struct {
	Language       string
	LanguageLabel  string
	CurrentReading *readingBookView
	Browse         readingBrowseView
}

type deckReadingState string

const (
	deckReadingUnknown   deckReadingState = ""
	deckReadingNotMember deckReadingState = "not-member"
	deckIsToRead         deckReadingState = "member"
	deckIsCurrentReading deckReadingState = "primary-goal"
)

type deckReadingActionView struct {
	BookID  string
	State   deckReadingState
	Message string
	Error   string
}

func emptyDeckReadingAction() deckReadingActionView {
	return deckReadingActionView{State: deckReadingUnknown}
}

func deckReadingActionID(bookID string) string {
	return "deck-preparation-journey-action-" + bookID
}

func (h *Handler) deckReadingStatus(ctx context.Context, owner, bookID string) (deckReadingActionView, error) {
	// Deck preparation surfaces are keyed by source_materials.id while Reading
	// membership and the current reading are keyed by books.id, so every action
	// identity is resolved to its canonical book first. A source material with
	// no book identity cannot join Reading, so no action is offered.
	resolved, ok, err := h.resolveBookID(ctx, owner, bookID)
	if err != nil {
		return deckReadingActionView{}, err
	}
	if !ok {
		return deckReadingActionView{}, nil
	}
	bookID = resolved
	language, _ := activeStudyLanguageForContext(ctx)
	currentReading, err := h.services.Store.Reading.GetCurrentReading(ctx, owner, language)
	if err != nil {
		return deckReadingActionView{}, err
	}
	action := deckReadingActionView{BookID: bookID, State: deckReadingNotMember}
	if currentReading.IsActive() && currentReading.BookID == bookID {
		action.State = deckIsCurrentReading
		return action, nil
	}
	detail, err := h.services.Store.Reading.GetBookDetail(ctx, owner, bookID)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return deckReadingActionView{}, err
	}
	if err == nil && detail.Disposition == domain.BookDispositionToRead {
		action.State = deckIsToRead
	}
	return action, nil
}

func (h *Handler) resolveBookID(ctx context.Context, owner, id string) (string, bool, error) {
	return h.services.Store.Reading.ResolveBookID(ctx, owner, id)
}

func (h *Handler) ensureToReadAnalysis(ctx context.Context, owner, bookID string) (analysis.Handle, cataloguesync.AcquisitionTarget, string, bool, error) {
	detail, err := h.services.Store.Reading.GetBookDetail(ctx, owner, bookID)
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
		target, err = h.acquireBookForReadingContext(ctx, owner, bookID)
		if err != nil {
			return analysis.Handle{}, target, detail.Book.Title, true, err
		}
		detail, err = h.services.Store.Reading.GetBookDetail(ctx, owner, bookID)
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

func toReadAnalysisError(ctx context.Context, catalog CatalogsStore, owner, bookID, title string, target cataloguesync.AcquisitionTarget, acquisitionFailed bool, err error) string {
	if acquisitionFailed {
		return "Book moved to To Read, but " + readingAcquisitionError(ctx, catalog, owner, bookID, title, target, err) + ". The To Read status is retained; analysis is unavailable until the current EPUB can be acquired."
	}
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) || errors.Is(err, analysis.ErrEPUBRequired) {
		return "Book moved to To Read, but the current source has no usable EPUB units. The To Read status is retained; analysis is unavailable."
	}
	return "Book moved to To Read, but analysis could not start. Retry analysis from Reading when ready."
}

func (h *Handler) annotateMyBooksWithDisposition(ctx context.Context, owner string, books []domain.MyBook) error {
	language, _ := activeStudyLanguageForContext(ctx)
	currentReading, err := h.services.Store.Reading.GetCurrentReading(ctx, owner, language)
	if err != nil {
		return err
	}
	currentReadingBookID := ""
	if currentReading.IsActive() {
		currentReadingBookID = currentReading.BookID
	}
	for i := range books {
		books[i].IsToRead = books[i].Disposition == domain.BookDispositionToRead
		books[i].IsCurrentReading = books[i].Book.ID == currentReadingBookID
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
	currentReading, err := h.services.Store.Reading.GetCurrentReading(ctx, owner, language)
	if err != nil {
		return err
	}
	book.IsToRead = book.Disposition == domain.BookDispositionToRead
	book.IsCurrentReading = currentReading.IsActive() && currentReading.BookID == book.Book.ID
	if book.Acquired != nil {
		book.Acquired.IsToRead = book.IsToRead
		book.Acquired.IsCurrentReading = book.IsCurrentReading
	}
	return nil
}

func (h *Handler) buildReadingView(ctx context.Context, owner, language string) (readingPageView, error) {
	currentReading, err := h.services.Store.Reading.GetCurrentReading(ctx, owner, language)
	if err != nil {
		return readingPageView{}, err
	}
	books, err := h.services.Store.Reading.ListSourceMaterials(ctx, owner)
	if err != nil {
		return readingPageView{}, err
	}
	bookByID := make(map[string]domain.SourceMaterialSummary, len(books))
	coverByBookID := make(map[string]domain.BookCover)
	for _, book := range books {
		bookByID[book.Source.ID] = book
	}
	myBooks, readErr := h.services.Store.Reading.ListMyBooksWithEvidence(ctx, owner)
	if readErr != nil {
		return readingPageView{}, readErr
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

	view := readingPageView{}
	if currentReading.IsActive() {
		book, bookErr := h.readingBook(ctx, owner, currentReading.BookID, bookByID)
		if bookErr != nil {
			return readingPageView{}, bookErr
		}
		book.CurrentReading = true
		book.Cover = coverByBookID[currentReading.BookID]
		book.ReadingSince = currentReading.CreatedAt
		if err = h.addReadingEvidence(ctx, owner, &book); err != nil {
			return readingPageView{}, err
		}
		book.CurrentReadingUnassessed = book.Book.EvidenceState() != domain.BookAnalyzed
		book.CurrentReadingVocabularyEligible, err = h.services.Store.Reading.CountCurrentReadingVocabularyToAccept(ctx, owner, language)
		if err != nil {
			return readingPageView{}, err
		}
		book.ReadingOnly = book.CurrentReadingVocabularyEligible == 0
		book.CurrentReadingSnapshotID = currentReading.SnapshotID
		book.CurrentReadingSnapshotSize = currentReading.SnapshotSize
		h.addCurrentReadingDeckPreparation(ctx, owner, &book, currentReading)
		view.CurrentReading = &book
	}
	return view, nil
}

func (h *Handler) addCurrentReadingDeckPreparation(ctx context.Context, owner string, book *readingBookView, currentReading domain.CurrentReading) {
	if currentReading.SnapshotSize == 0 {
		return
	}
	book.CurrentReadingDeckUnavailable = true
	preparation, err := h.services.PreparedDeck.GetForGoalSnapshot(ctx, owner, currentReading.SnapshotID)
	switch {
	case err == nil:
		if !currentReadingPreparationMatches(preparation, owner, currentReading) {
			log.Printf("mouseion: Goal deck provenance mismatch for owner %s snapshot %s", owner, currentReading.SnapshotID)
			return
		}
		book.CurrentReadingPreparation = &preparation
		book.CurrentReadingDeckUnavailable = false
	case errors.Is(err, persistence.ErrNotFound):
		// The Current reading remains visible while an unavailable artifact is retried through
		// the exact snapshot identity.
		book.CurrentReadingDeckMissing = true
	default:
		log.Printf("mouseion: Goal deck unavailable for owner %s snapshot %s: %v", owner, currentReading.SnapshotID, err)
	}
}

func (h *Handler) readingBook(ctx context.Context, owner, bookID string, bookByID map[string]domain.SourceMaterialSummary) (readingBookView, error) {
	if book, ok := bookByID[bookID]; ok {
		return readingBookView{Book: book, BookID: bookID}, nil
	}
	// Current reading and To Read status are allowed to exist before acquisition. Keep
	// that identity visible instead of silently dropping it from the surface.
	book, err := h.services.Store.Reading.GetBook(ctx, owner, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return readingBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: bookID, Title: "Book details unavailable", OwnerID: owner}}, BookID: bookID}, nil
		}
		return readingBookView{}, err
	}
	return readingBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: book.ID, OwnerID: owner, Title: book.Title, Language: book.LanguageTag}, BookTitle: book.Title, BookAuthor: book.Author}, BookID: book.ID}, nil
}

func (h *Handler) addReadingEvidence(ctx context.Context, owner string, book *readingBookView) error {
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
