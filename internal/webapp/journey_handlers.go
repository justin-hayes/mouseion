package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type journeyBookView struct {
	Book                   domain.SourceMaterialSummary
	BookID                 string
	Position               int
	JourneyRevision        int64
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
	Forecast               *domain.JourneyForecastEntry
	ForecastHasGoal        bool
	ForecastUnavailable    bool
}

func journeyBookClass(primary bool) string {
	if primary {
		return "resource-card journey-book journey-book--goal"
	}
	return "resource-card journey-book"
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

func journeyMembershipBookID(book domain.SourceMaterialSummary) string {
	if book.BookID != "" {
		return book.BookID
	}
	return book.Source.ID
}

func journeyMoveURL(bookID string, earlier bool) string {
	direction := "move-later"
	if earlier {
		direction = "move-earlier"
	}
	return "/journey/entries/" + bookID + "/" + direction
}

func journeyRemoveURL(bookID string) string {
	return "/journey/books/" + url.PathEscape(bookID) + "/remove"
}

func journeyReanalyzeURL(bookID string) string {
	return "/journey/books/" + url.PathEscape(bookID) + "/reanalyze"
}

func journeyEntryURL(bookID string) string {
	return "/journey/" + url.PathEscape(bookID)
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

func journeyExpectedGoalBookID(journey journeyPageView) string {
	if journey.Goal == nil {
		return ""
	}
	return journeyBookID(*journey.Goal)
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
	if !strings.EqualFold(strings.TrimSpace(item.Book.Source.MediaType), opds.EPUBMediaType) || strings.TrimSpace(item.Book.Source.ContentRevisionID) == "" {
		return bookLifecycleAction{
			Status:      "Assessment unavailable",
			Description: "No current EPUB content is available for this Journey entry. Retry acquisition when the catalog can provide it.",
			Label:       "Retry acquisition",
			URL:         journeyReanalyzeURL(bookID),
			Submit:      true,
			Tone:        StatusWarning,
		}
	}
	action := bookLifecycleActionFor(item.Book)
	if journeyEvidenceState(item) == "failed" || journeyEvidenceState(item) == "incomplete" {
		action.Status = "Analysis incomplete"
		action.Description = journeyEvidenceDescription(item)
		action.Label = "Retry analysis"
		action.URL = journeyReanalyzeURL(bookID)
		action.Submit = true
		action.Tone = StatusWarning
		if journeyEvidenceState(item) == "failed" {
			action.Status = "Analysis failed"
			action.Tone = StatusDanger
		}
	} else if action.Status == "Analysis not started" {
		action.Label = "Retry analysis"
		action.URL = journeyReanalyzeURL(bookID)
		action.Submit = true
	} else if action.Status == "Analysis result ready" && bookHasCompletedAnalysis(item.Book) {
		action.URL = journeyEntryURL(bookID)
		action.Label = "View Journey entry"
	}
	return action
}

func journeyGoalEligibility(book domain.SourceMaterialSummary) (bool, string) {
	switch book.GoalEligibility() {
	case domain.GoalNeedsCurrentContent:
		return false, "This book cannot become a Primary Goal until current EPUB content is available."
	case domain.GoalAnalysisInProgress:
		return false, "This book cannot become a Primary Goal while its current analysis is still in progress."
	case domain.GoalFailed:
		return false, "This book cannot become a Primary Goal until its failed analysis is retried successfully."
	case domain.GoalCancelled:
		return false, "This book cannot become a Primary Goal until its cancelled analysis is retried successfully."
	case domain.GoalStale:
		return false, "This book cannot become a Primary Goal until its analysis matches the current content."
	case domain.GoalNoCompletedAnalysis:
		return false, "This book needs a successfully completed current analysis before it can become a Primary Goal."
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
	Goal                *journeyBookView
	Provisional         []journeyBookView
	Revision            int64
	ForecastUnavailable bool
}

func journeyPageTitle(journey journeyPageView) string {
	label := strings.TrimSpace(journey.LanguageLabel)
	if label == "" {
		label = strings.TrimSpace(journey.Language)
	}
	if label == "" {
		return "Reading Journey"
	}
	return "Reading Journey in " + label
}

type deckJourneyState string

const (
	deckJourneyUnknown   deckJourneyState = ""
	deckJourneyNotMember deckJourneyState = "not-member"
	deckJourneyMember    deckJourneyState = "member"
	deckJourneyGoal      deckJourneyState = "primary-goal"
)

type deckJourneyActionView struct {
	BookID        string
	PreparationID string
	Revision      int64
	State         deckJourneyState
	Message       string
	Error         string
}

func emptyDeckJourneyAction() deckJourneyActionView {
	return deckJourneyActionView{State: deckJourneyUnknown}
}

func deckJourneyActionID(bookID string) string {
	return "deck-preparation-journey-action-" + bookID
}

func journeyAddURL(bookID string) string {
	return "/journey/books/" + url.PathEscape(bookID) + "/add"
}

func (h *Handler) deckJourneyAction(ctx context.Context, owner string, preparationID, bookID string) (deckJourneyActionView, error) {
	// Deck preparation surfaces are keyed by source_materials.id while Journey
	// membership and the Primary Goal are keyed by books.id, so every action
	// identity is resolved to its canonical book first. A source material with
	// no book identity cannot join the Journey, so no action is offered.
	resolved, ok, err := h.services.Store.Journey.ResolveJourneyBookID(ctx, owner, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	if !ok {
		return deckJourneyActionView{}, nil
	}
	bookID = resolved
	language, _ := activeStudyLanguageForContext(ctx)
	journey, err := h.services.Store.Journey.GetReadingJourney(ctx, owner, language)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	goal, err := h.services.Store.Goals.GetPrimaryGoal(ctx, owner, language)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	action := deckJourneyActionView{BookID: bookID, PreparationID: preparationID, Revision: journey.Revision, State: deckJourneyNotMember}
	if goal.IsActive() && goal.BookID == bookID {
		action.State = deckJourneyGoal
		return action, nil
	}
	for _, entry := range journey.Entries {
		if entry.BookID == bookID {
			action.State = deckJourneyMember
			break
		}
	}
	return action, nil
}

func (h *Handler) addBookToReadingJourney(ctx context.Context, owner, preparationID, bookID string, expectedRevision int64) (deckJourneyActionView, error) {
	resolved, ok, err := h.services.Store.Journey.ResolveJourneyBookID(ctx, owner, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	if !ok {
		return deckJourneyActionView{}, nil
	}
	bookID = resolved
	language, _ := activeStudyLanguageForContext(ctx)
	action, err := h.deckJourneyAction(ctx, owner, preparationID, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	// A Primary Goal is intentionally not changed by a ready-deck action. This
	// guard also keeps a forged direct POST from adding or reordering the Goal.
	if action.State == deckJourneyGoal {
		return action, nil
	}
	if _, err = h.services.Store.Journey.AddToReadingJourney(ctx, owner, language, bookID, expectedRevision); err != nil {
		log.Print("mouseion: add book to Reading Journey failed")
		refreshed, refreshErr := h.deckJourneyAction(ctx, owner, preparationID, bookID)
		if refreshErr != nil {
			return deckJourneyActionView{}, refreshErr
		}
		if errors.Is(err, persistence.ErrJourneyStale) {
			refreshed.Message = journeyStaleMessage
			return refreshed, nil
		}
		if errors.Is(err, persistence.ErrBookLanguageRequired) {
			refreshed.Error = "This book needs a language before it can join Reading Journey. Fix the language in the catalog, then re-sync."
			return refreshed, nil
		}
		refreshed.Error = "The book could not be added to Reading Journey. No Journey changes were made; try again."
		return refreshed, nil
	}
	refreshed, err := h.deckJourneyAction(ctx, owner, preparationID, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	if action.State == deckJourneyNotMember && h.services.Analysis != nil {
		handle, target, title, acquisitionFailed, analysisErr := h.ensureJourneyAnalysis(ctx, owner, bookID)
		if analysisErr != nil {
			refreshed.Error = journeyAnalysisError(ctx, h.services.Store.Catalog, owner, bookID, title, target, acquisitionFailed, analysisErr)
		} else {
			refreshed.Message = fmt.Sprintf("Book added to Reading Journey. Analysis job #%d submitted.", handle.DisplayNumber)
		}
		return refreshed, nil
	}
	if refreshed.State == deckJourneyMember {
		if action.State == deckJourneyMember {
			refreshed.Message = "This book is already in your Reading Journey."
		} else {
			refreshed.Message = "Book added to Reading Journey."
		}
	}
	return refreshed, nil
}

func (h *Handler) ensureJourneyAnalysis(ctx context.Context, owner, bookID string) (analysis.Handle, cataloguesync.AcquisitionTarget, string, bool, error) {
	detail, err := h.services.Store.Books.GetBookDetail(ctx, owner, bookID)
	if err != nil {
		return analysis.Handle{}, cataloguesync.AcquisitionTarget{}, "", false, err
	}
	target := cataloguesync.AcquisitionTarget{}
	acquisitionAttempted := false
	if detail.Acquired == nil || detail.Acquired.EvidenceState() == domain.BookUnavailable {
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
	handle, err := h.services.Analysis.SubmitAnalysis(ctx, owner, detail.Acquired.Source.ID)
	return handle, target, detail.Book.Title, false, err
}

func journeyAnalysisError(ctx context.Context, catalog CatalogStore, owner, bookID, title string, target cataloguesync.AcquisitionTarget, acquisitionFailed bool, err error) string {
	if acquisitionFailed {
		return "Book added to Reading Journey, but " + journeyAcquisitionError(ctx, catalog, owner, bookID, title, target, err) + ". The Journey entry is retained; assessment is unavailable until the current EPUB can be acquired."
	}
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) || errors.Is(err, analysis.ErrEPUBRequired) {
		return "Book added to Reading Journey, but the current source has no usable EPUB units. The Journey entry is retained; assessment is unavailable."
	}
	return "Book added to Reading Journey, but analysis could not start. The Journey entry is retained; retry analysis from the Journey entry when ready."
}

func (h *Handler) annotateMyBooksWithJourney(ctx context.Context, owner string, books []domain.MyBook) error {
	language, _ := activeStudyLanguageForContext(ctx)
	journey, err := h.services.Store.Journey.GetReadingJourney(ctx, owner, language)
	if err != nil {
		return err
	}
	goal, err := h.services.Store.Goals.GetPrimaryGoal(ctx, owner, language)
	if err != nil {
		return err
	}
	goalBookID := ""
	if goal.IsActive() {
		goalBookID = goal.BookID
	}
	members := make(map[string]bool, len(journey.Entries))
	for _, entry := range journey.Entries {
		members[entry.BookID] = true
	}
	for i := range books {
		books[i].JourneyMember = members[books[i].Book.ID]
		books[i].JourneyGoal = books[i].Book.ID == goalBookID
		books[i].JourneyRevision = journey.Revision
		if books[i].Acquired != nil {
			books[i].Acquired.JourneyMember = books[i].JourneyMember
			books[i].Acquired.JourneyGoal = books[i].JourneyGoal
			books[i].Acquired.JourneyRevision = journey.Revision
		}
	}
	return nil
}

func (h *Handler) annotateBookWithJourney(ctx context.Context, owner string, book *domain.MyBook) error {
	language, _ := activeStudyLanguageForContext(ctx)
	return h.annotateBookWithJourneyLanguage(ctx, owner, language, book)
}

func (h *Handler) annotateBookWithJourneyLanguage(ctx context.Context, owner, language string, book *domain.MyBook) error {
	journey, err := h.services.Store.Journey.GetReadingJourney(ctx, owner, language)
	if err != nil {
		return err
	}
	goal, err := h.services.Store.Goals.GetPrimaryGoal(ctx, owner, language)
	if err != nil {
		return err
	}
	for _, entry := range journey.Entries {
		if entry.BookID == book.Book.ID {
			book.JourneyMember = true
			break
		}
	}
	book.JourneyGoal = goal.IsActive() && goal.BookID == book.Book.ID
	book.JourneyRevision = journey.Revision
	if book.Acquired != nil {
		book.Acquired.JourneyMember = book.JourneyMember
		book.Acquired.JourneyGoal = book.JourneyGoal
		book.Acquired.JourneyRevision = journey.Revision
	}
	return nil
}

func (h *Handler) addDeckBookToJourney(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	preparationID := strings.TrimSpace(r.FormValue("deck_preparation_id"))
	expectedRevision, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("expected_revision")), 10, 64)
	if err != nil {
		if isHTMX(r) {
			action, actionErr := h.deckJourneyAction(r.Context(), owner, preparationID, bookID)
			if actionErr != nil {
				fail(w, actionErr)
				return
			}
			action.Message = journeyStaleMessage
			render(w, r, DeckJourneyAction(action, h.csrf(w, r)))
			return
		}
		h.redirectDeckJourneyAction(w, r, "", journeyStaleMessage)
		return
	}
	action, err := h.addBookToReadingJourney(r.Context(), owner, preparationID, bookID, expectedRevision)
	if err != nil {
		fail(w, err)
		return
	}
	if isHTMX(r) {
		render(w, r, DeckJourneyAction(action, h.csrf(w, r)))
		return
	}
	h.redirectDeckJourneyAction(w, r, action.Message, action.Error)
}

func (h *Handler) reanalyzeJourneyBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	if err := h.annotateBookWithJourneyLanguage(r.Context(), u.ID, detail.Book.LanguageTag, &detail); err != nil {
		fail(w, err)
		return
	}
	if !detail.JourneyMember || detail.Acquired == nil {
		http.NotFound(w, r)
		return
	}
	recoverable := detail.Acquired.EvidenceState() == domain.BookStale || detail.Acquired.EvidenceState() == domain.BookUnavailable || detail.Acquired.EvidenceState() == domain.BookAcquiredUnassessed
	if detail.Acquired.EvidenceState() == domain.BookAnalyzed {
		item := journeyBookView{Book: *detail.Acquired, BookID: detail.Book.ID}
		if evidenceErr := h.addJourneyEvidence(r.Context(), u.ID, &item); evidenceErr != nil {
			fail(w, evidenceErr)
			return
		}
		recoverable = journeyEvidenceState(item) == "incomplete"
	}
	if !recoverable {
		http.NotFound(w, r)
		return
	}
	handle, target, title, acquisitionFailed, err := h.ensureJourneyAnalysis(r.Context(), u.ID, detail.Book.ID)
	if err != nil {
		message := journeyAnalysisError(r.Context(), h.services.Store.Catalog, u.ID, detail.Book.ID, title, target, acquisitionFailed, err)
		redirect(w, r, "/journey?error="+url.QueryEscape(message))
		return
	}
	redirect(w, r, "/journey?message="+url.QueryEscape(fmt.Sprintf("Analysis job #%d submitted.", handle.DisplayNumber)))
}

func (h *Handler) redirectDeckJourneyAction(w http.ResponseWriter, r *http.Request, message, pageError string) {
	location := "/journey"
	query := url.Values{}
	if message != "" {
		query.Set("message", message)
	}
	if pageError != "" {
		query.Set("error", pageError)
	}
	if encoded := query.Encode(); encoded != "" {
		location += "?" + encoded
	}
	redirect(w, r, location)
}

func (h *Handler) journey(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	view, err := h.buildJourneyView(r.Context(), u.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	view.Language = language
	view.LanguageLabel = languageLabel
	render(w, r, JourneyPage(u, h.csrf(w, r), view, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
}

func (h *Handler) buildJourneyView(ctx context.Context, owner, language string) (journeyPageView, error) {
	journey, err := h.services.Store.Journey.GetReadingJourney(ctx, owner, language)
	if err != nil {
		return journeyPageView{}, err
	}
	goal, err := h.services.Store.Goals.GetPrimaryGoal(ctx, owner, language)
	if err != nil {
		return journeyPageView{}, err
	}
	books, err := h.services.Store.Books.ListSourceMaterials(ctx, owner)
	if err != nil {
		return journeyPageView{}, err
	}
	bookByID := make(map[string]domain.SourceMaterialSummary, len(books))
	for _, book := range books {
		bookByID[book.Source.ID] = book
	}
	if reader, ok := h.services.Store.Books.(interface {
		ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	}); ok {
		if myBooks, readErr := reader.ListMyBooksWithEvidence(ctx, owner); readErr == nil {
			for _, myBook := range myBooks {
				if myBook.Acquired != nil {
					book := *myBook.Acquired
					book.BookTitle = myBook.Book.Title
					bookByID[myBook.Book.ID] = book
					bookByID[book.Source.ID] = book
				} else {
					bookByID[myBook.Book.ID] = domain.SourceMaterialSummary{
						Source:    domain.SourceMaterial{ID: myBook.Book.ID, OwnerID: myBook.Book.OwnerID, Title: myBook.Book.Title, Language: myBook.Book.LanguageTag},
						BookTitle: myBook.Book.Title,
						BookID:    myBook.Book.ID,
					}
				}
			}
		}
	}

	view := journeyPageView{Revision: journey.Revision}
	if goal.IsActive() {
		book, bookErr := h.journeyBook(ctx, owner, goal.BookID, bookByID)
		if bookErr != nil {
			return journeyPageView{}, bookErr
		}
		book.PrimaryGoal = true
		book.JourneyRevision = journey.Revision
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
		if goal.SnapshotSize > 0 {
			book.GoalDeckUnavailable = true
			if reader, ok := h.services.PreparedDeck.(PreparedDeckForGoalSnapshot); ok {
				preparation, preparationErr := reader.GetForGoalSnapshot(ctx, owner, goal.SnapshotID)
				switch {
				case preparationErr == nil:
					book.GoalPreparation = &preparation
					book.GoalDeckUnavailable = false
				case errors.Is(preparationErr, persistence.ErrNotFound):
					// The Goal remains visible while an unavailable artifact is
					// retried through the exact snapshot identity.
					book.GoalDeckMissing = true
				default:
					log.Printf("mouseion: Goal deck unavailable for owner %s snapshot %s: %v", owner, goal.SnapshotID, preparationErr)
				}
			}
		}
		view.Goal = &book
	}
	for _, entry := range journey.Entries {
		// A Goal is anchored in the first section and must not be duplicated as
		// a provisional membership when the two relationships overlap.
		if entry.BookID == goal.BookID {
			continue
		}
		book, bookErr := h.journeyBook(ctx, owner, entry.BookID, bookByID)
		if bookErr != nil {
			return journeyPageView{}, bookErr
		}
		book.Position = entry.Position
		book.JourneyRevision = journey.Revision
		if err = h.addJourneyEvidence(ctx, owner, &book); err != nil {
			return journeyPageView{}, err
		}
		book.CanChooseGoal, book.GoalEligibilityReason = journeyGoalEligibility(book.Book)
		view.Provisional = append(view.Provisional, book)
	}
	for i := range view.Provisional {
		view.Provisional[i].CanMoveEarlier = i > 0
		view.Provisional[i].CanMoveLater = i < len(view.Provisional)-1
		view.Provisional[i].ForecastHasGoal = goal.IsActive()
	}
	if view.Goal != nil {
		view.Goal.ForecastHasGoal = goal.IsActive()
	}
	// A reorder must never announce success when the forecast capability is
	// absent or returns an incomplete read model. Unavailable evidence is still
	// a valid forecast entry; a missing Journey member is not.
	view.ForecastUnavailable = view.Goal != nil || len(view.Provisional) > 0
	if provider, ok := h.services.AnalysisInsights.(journeyForecastProvider); ok {
		forecast, forecastErr := provider.JourneyForecast(ctx, owner, language)
		if forecastErr != nil {
			log.Printf("mouseion: Journey forecast unavailable for owner %s: %v", owner, forecastErr)
		} else {
			forecastByBook := make(map[string]*domain.JourneyForecastEntry, len(forecast.Entries))
			for i := range forecast.Entries {
				forecastByBook[forecast.Entries[i].BookID] = &forecast.Entries[i]
			}
			if view.Goal != nil {
				view.Goal.Forecast = forecastByBook[view.Goal.BookID]
			}
			for i := range view.Provisional {
				view.Provisional[i].Forecast = forecastByBook[view.Provisional[i].BookID]
			}
			view.ForecastUnavailable = false
			if goal.IsActive() && forecast.Goal == nil {
				view.ForecastUnavailable = true
			}
			if view.Goal != nil && view.Goal.Forecast == nil {
				view.ForecastUnavailable = true
			}
			for i := range view.Provisional {
				if view.Provisional[i].Forecast == nil {
					view.ForecastUnavailable = true
					break
				}
			}
		}
	}
	if view.ForecastUnavailable {
		if view.Goal != nil {
			view.Goal.Forecast = nil
			view.Goal.ForecastUnavailable = true
		}
		for i := range view.Provisional {
			view.Provisional[i].Forecast = nil
			view.Provisional[i].ForecastUnavailable = true
		}
	}

	return view, nil
}

func (h *Handler) journeyBook(ctx context.Context, owner, bookID string, bookByID map[string]domain.SourceMaterialSummary) (journeyBookView, error) {
	if book, ok := bookByID[bookID]; ok {
		return journeyBookView{Book: book, BookID: bookID}, nil
	}
	// Goal and Journey membership are allowed to exist before acquisition. Keep
	// that identity visible instead of silently dropping it from the surface.
	book, err := h.services.Store.Books.GetBook(ctx, owner, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: bookID, Title: "Book details unavailable", OwnerID: owner}}, BookID: bookID}, nil
		}
		return journeyBookView{}, err
	}
	return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: book.ID, OwnerID: owner, Title: book.Title, Language: book.LanguageTag}, BookTitle: book.Title}, BookID: book.ID}, nil
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
