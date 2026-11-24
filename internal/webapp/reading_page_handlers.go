package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
)

type readingBookView struct {
	Book                             domain.SourceMaterialSummary
	BookID                           string
	ReadingUnit                      *domain.BookReadingUnit
	ClassicalPreparation             *domain.ClassicalPreparationStatus
	Cover                            domain.BookCover
	ReadingSince                     time.Time
	CurrentReading                   bool
	ReadingOnly                      bool
	CurrentReadingUnassessed         bool
	CurrentReadingVocabularyEligible int
	CurrentReadingSnapshotID         string
	CurrentReadingSnapshotSize       int
	CurrentReadingPreparation        *domain.DeckPreparation
	CurrentReadingActions            deckPreparationActions
	CurrentReadingDeckMissing        bool
	CurrentReadingDeckUnavailable    bool
	CanMoveEarlier                   bool
	CanMoveLater                     bool
	CanChooseCurrentReading          bool
	Coverage                         *domain.AnalysisCoverage
	StatisticsUnavailable            bool
	CountsUpdating                   bool
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

// classification classifies the Reading view's Book with its preparation
// evidence, so a failure applies only while no analysis is in effect.
func (item readingBookView) classification() domain.BookEvidenceClassification {
	return item.Book.EvidenceClassificationWithPreparation(
		domain.ClassicalPreparationSignalFor(item.ClassicalPreparation, item.ReadingUnit))
}

// readingEvidenceState combines the Book's evidence classification with whether
// its coverage statistics are available to present.
func readingEvidenceState(item readingBookView) readingEvidenceStatus {
	classification := item.classification()
	if classification.Preparation.Applies() {
		return readingEvidenceStatusFor(classification, false)
	}
	if item.CountsUpdating && classification.Evidence == domain.BookAnalyzed {
		return readingEvidenceUpdating
	}
	return readingEvidenceStatusFor(classification, item.Coverage != nil && !item.StatisticsUnavailable)
}

func readingEvidenceLabel(item readingBookView) string {
	classification := item.classification()
	if classification.Preparation.Applies() {
		return classicalPreparationViewFor(classification).Label
	}
	switch readingEvidenceState(item) {
	case readingEvidenceStale:
		return "Stale evidence"
	case readingEvidenceIncomplete:
		return "Incomplete evidence"
	case readingEvidenceFailed:
		return "Analysis failed"
	case readingEvidenceUnavailable:
		return "Evidence unavailable"
	case readingEvidencePublishing:
		return analysisPublicationPendingLabel
	case readingEvidenceUpdating:
		return "Updating"
	case readingEvidenceCurrent:
		return ""
	default:
		return ""
	}
}

func readingEvidenceDescription(item readingBookView) string {
	classification := item.classification()
	if classification.Preparation.Applies() {
		return classicalPreparationViewFor(classification).Description
	}
	switch readingEvidenceState(item) {
	case readingEvidenceStale:
		return "The current content no longer matches this analysis. Re-analyze the book to refresh its evidence."
	case readingEvidenceIncomplete:
		if item.Book.EvidenceClassification().RunFinished() {
			return analysisPublicationPendingDescription
		}
		return "A completed analysis has not produced usable coverage for this book yet."
	case readingEvidenceFailed:
		return "The last analysis failed. Retry analysis to produce current evidence."
	case readingEvidenceUnavailable:
		return "Current book content is unavailable, so coverage cannot be calculated."
	case readingEvidencePublishing:
		return analysisPublicationPendingDescription
	case readingEvidenceUpdating:
		return vocabularyCountsUpdatingDescription
	case readingEvidenceCurrent:
		return ""
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
	classification := item.classification()
	if classification.Preparation.Applies() {
		view := classicalPreparationViewFor(classification)
		action := bookLifecycleAction{Status: view.Label, Description: view.Description, Tone: classicalPreparationTone(classification)}
		if view.Actionable {
			action.Label = view.Recovery
			action.URL = readingReanalyzeURL(bookID)
			action.Submit = true
		}
		return action
	}
	if !classification.Content.IsCurrentContent() {
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
	switch readingEvidenceState(item) {
	case readingEvidencePublishing:
		// A finished run awaiting publication has no recovery action; retrying
		// would only rerun analysis the publisher is already completing.
		return bookLifecycleAction{Status: analysisPublicationPendingLabel, Description: analysisPublicationPendingDescription, Tone: StatusInfo}
	case readingEvidenceFailed:
		action.Status = "Analysis failed"
		action.Description = readingEvidenceDescription(item)
		action.Label = "Retry analysis"
		action.URL = readingReanalyzeURL(bookID)
		action.Submit = true
		action.Tone = StatusDanger
		return action
	case readingEvidenceIncomplete:
		// A newer run shadowed by a published analysis is secondary information;
		// it keeps its own status and Retry rather than the coverage message.
		if action.Status != "Analysis queued" && action.Status != "Analysis running" && !classification.ReAnalysisShadowed() {
			action.Status = "Analysis incomplete"
			action.Description = readingEvidenceDescription(item)
			action.Label = "Retry analysis"
			action.URL = readingReanalyzeURL(bookID)
			action.Submit = true
			action.Tone = StatusWarning
			return action
		}
	case readingEvidenceCurrent, readingEvidenceStale, readingEvidenceUnavailable, readingEvidenceUpdating:
	}
	if action.Status == "Analysis not started" {
		action.Label = "Retry analysis"
		action.URL = readingReanalyzeURL(bookID)
		action.Submit = true
	} else if action.Status == "Analysis result ready" && analysisReadyForReading(item.Book) {
		action.URL = readingBookURL(bookID)
		action.Label = "View in Reading"
	}
	return action
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
	// Actions are decided by the prepared-deck admission. Templates render them
	// and never compare Current reading fields themselves.
	Actions deckPreparationActions
}

// deckPreparationActions are the Book deck actions a surface may offer.
type deckPreparationActions struct {
	BookID string
	// OnCurrentReading reports that the Book is the current reading, so the
	// deck is still Reading's to act on, even if no action is admitted now.
	OnCurrentReading bool
	// Submit prepares, or retries, the deck for the current snapshot.
	Submit bool
	// Reprepare rolls a ready deck forward to a new generation.
	Reprepare bool
}

func deckActionsFor(bookID string, admissions prepareddeck.PreparationAdmissions) deckPreparationActions {
	return deckPreparationActions{
		BookID:           bookID,
		OnCurrentReading: admissions.Retry != domain.DeckAdmissionNotCurrentReading,
		Submit:           admissions.Retry.Admitted(),
		Reprepare:        admissions.Reprepare.Admitted(),
	}
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
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
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
	readingUnitByID := make(map[string]*domain.BookReadingUnit, len(books))
	classicalPreparationByID := make(map[string]*domain.ClassicalPreparationStatus, len(books))
	for _, book := range books {
		bookByID[book.Source.ID] = book
	}
	myBooks, readErr := h.services.Store.Reading.ListMyBooksWithEvidence(ctx, owner)
	if readErr != nil {
		return readingPageView{}, readErr
	}
	for _, myBook := range myBooks {
		coverByBookID[myBook.Book.ID] = myBook.Cover
		readingUnitByID[myBook.Book.ID] = myBook.ReadingUnit
		classicalPreparationByID[myBook.Book.ID] = myBook.ClassicalPreparation
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
		book.Book.IsCurrentReading = true
		book.ReadingUnit = readingUnitByID[currentReading.BookID]
		book.ClassicalPreparation = classicalPreparationByID[currentReading.BookID]
		book.Cover = coverByBookID[currentReading.BookID]
		book.ReadingSince = currentReading.CreatedAt
		if err := h.addReadingEvidence(ctx, owner, &book); err != nil {
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
	deck, err := h.services.PreparedDeck.CurrentReadingDeck(ctx, owner, currentReading.BookID)
	if err != nil {
		// The Current reading remains visible; the retry form re-decides on submit.
		log.Printf("mouseion: Current reading deck unavailable for owner %s snapshot %s: %v", owner, currentReading.SnapshotID, err)
		return
	}
	if deck.Admission == domain.DeckAdmissionNotCurrentReading {
		log.Printf("mouseion: Current reading deck provenance mismatch for owner %s snapshot %s", owner, currentReading.SnapshotID)
		return
	}
	book.CurrentReadingDeckUnavailable = false
	if deck.Preparation == nil {
		book.CurrentReadingDeckMissing = true
		book.CurrentReadingActions = deckPreparationActions{BookID: currentReading.BookID, OnCurrentReading: true, Submit: deck.Admission.Admitted()}
		return
	}
	admissions, err := h.services.PreparedDeck.PreparationAdmissions(ctx, owner, deck.Preparation.ID)
	if err != nil {
		log.Printf("mouseion: Current reading deck admissions unavailable for owner %s snapshot %s: %v", owner, currentReading.SnapshotID, err)
		book.CurrentReadingDeckUnavailable = true
		return
	}
	book.CurrentReadingPreparation = deck.Preparation
	book.CurrentReadingActions = deckActionsFor(currentReading.BookID, admissions)
}

func (h *Handler) readingBook(ctx context.Context, owner, bookID string, bookByID map[string]domain.SourceMaterialSummary) (readingBookView, error) {
	if book, ok := bookByID[bookID]; ok {
		return readingBookView{Book: book, BookID: bookID}, nil
	}
	// Current reading and To Read status are allowed to exist before acquisition. Keep
	// that identity visible instead of silently dropping it from the surface.
	book, err := h.services.Store.Reading.GetBook(ctx, owner, bookID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
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
	if errors.Is(err, analysisinsights.ErrCountsUpdating) {
		book.CountsUpdating = true
		return nil
	}
	if err != nil {
		return err
	}
	book.Coverage = &coverage
	return nil
}
