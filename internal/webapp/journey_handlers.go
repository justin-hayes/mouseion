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

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type journeyBookView struct {
	Book                  domain.SourceMaterialSummary
	BookID                string
	Position              int
	PrimaryGoal           bool
	GoalReadingOnly       bool
	GoalUnassessed        bool
	GoalDeckAvailable     bool
	GoalResidual          *goalResidualView
	CanMoveEarlier        bool
	CanMoveLater          bool
	Coverage              *domain.AnalysisCoverage
	StatisticsUnavailable bool
}

func primaryGoalIsActive(goal domain.PrimaryGoal) bool {
	return goal.BookID != "" && goal.ReadingFinishedAt == nil
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

func journeyMoveURL(bookID string, earlier bool) string {
	direction := "move-later"
	if earlier {
		direction = "move-earlier"
	}
	return "/journey/entries/" + bookID + "/" + direction
}

func journeyBookTitle(book domain.SourceMaterialSummary) string {
	if strings.TrimSpace(book.Source.Title) != "" {
		return book.Source.Title
	}
	return book.Source.ID
}

func journeyExpectedGoalBookID(journey journeyPageView) string {
	if journey.Goal == nil {
		return ""
	}
	return journeyBookID(*journey.Goal)
}

func goalCardView(goal *journeyBookView, residual *goalResidualView) journeyBookView {
	if goal == nil {
		return journeyBookView{}
	}
	view := *goal
	if residual != nil {
		view.GoalResidual = residual
	}
	return view
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
	status := strings.ToLower(item.Book.AnalysisStatus + " " + item.Book.AnalysisState)
	if strings.Contains(status, "stale") || strings.Contains(status, "needs review") {
		return "stale"
	}
	if item.StatisticsUnavailable || item.Coverage == nil {
		return "unavailable"
	}
	return "current"
}

func journeyEvidenceLabel(item journeyBookView) string {
	switch journeyEvidenceState(item) {
	case "stale":
		return "Stale evidence"
	case "unavailable":
		return "Coverage unavailable"
	default:
		return "Current evidence"
	}
}

func journeyEvidenceDescription(item journeyBookView) string {
	switch journeyEvidenceState(item) {
	case "stale":
		return "Current and projected coverage are unavailable until this book's analysis is reviewed."
	case "unavailable":
		return "This book is not currently assessed or its statistics are unavailable."
	default:
		return "Coverage is based on the current analyzed evidence."
	}
}

func journeyCurrentCoverage(item journeyBookView) string {
	if item.Coverage == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f%%", knownCoveragePercent(*item.Coverage))
}

func journeyProjectedCoverage(item journeyBookView) string {
	if item.Coverage == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f%% if active-campaign vocabulary graduates; %s", activeCampaignCoveragePercent(*item.Coverage), journeyProjectionText(*item.Coverage))
}

func journeyProjectionText(coverage domain.AnalysisCoverage) string {
	if len(coverage.Projections) == 0 {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f%% after the top %d deck-eligible lemmas", projectedCoveragePercent(coverage.Projections[0], coverage.AnalyzableTokenCount), coverage.Projections[0].TopLemmaCount)
}

type journeyPageView struct {
	Goal            *journeyBookView
	Residual        *goalResidualView
	Provisional     []journeyBookView
	Revision        int64
	Campaigns       []campaignView
	Prepared        []preparedCampaignOption
	RouteComparison *routeComparisonView
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

func deckJourneyAddURL(bookID string) string {
	return "/journey/books/" + url.PathEscape(bookID) + "/add"
}

func (h *Handler) deckJourneyAction(ctx context.Context, owner string, preparationID, bookID string) (deckJourneyActionView, error) {
	// Deck preparation surfaces are keyed by source_materials.id while Journey
	// membership and the Primary Goal are keyed by books.id, so every action
	// identity is resolved to its canonical book first. A source material with
	// no book identity cannot join the Journey, so no action is offered.
	resolved, ok, err := h.services.Store.ResolveJourneyBookID(ctx, owner, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	if !ok {
		return deckJourneyActionView{}, nil
	}
	bookID = resolved
	journey, err := h.services.Store.GetReadingJourney(ctx, owner)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	goal, err := h.services.Store.GetPrimaryGoal(ctx, owner)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	action := deckJourneyActionView{BookID: bookID, PreparationID: preparationID, Revision: journey.Revision, State: deckJourneyNotMember}
	if primaryGoalIsActive(goal) && goal.BookID == bookID {
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
	resolved, ok, err := h.services.Store.ResolveJourneyBookID(ctx, owner, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	if !ok {
		return deckJourneyActionView{}, nil
	}
	bookID = resolved
	action, err := h.deckJourneyAction(ctx, owner, preparationID, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
	}
	// A Primary Goal is intentionally not changed by a ready-deck action. This
	// guard also keeps a forged direct POST from adding or reordering the Goal.
	if action.State == deckJourneyGoal {
		return action, nil
	}
	if _, err = h.services.Store.AddToReadingJourney(ctx, owner, bookID, expectedRevision); err != nil {
		log.Printf("mouseion: add book %s to Reading Journey failed: %v", bookID, err)
		refreshed, refreshErr := h.deckJourneyAction(ctx, owner, preparationID, bookID)
		if refreshErr != nil {
			return deckJourneyActionView{}, refreshErr
		}
		if errors.Is(err, persistence.ErrJourneyStale) {
			refreshed.Message = journeyStaleMessage
			return refreshed, nil
		}
		refreshed.Error = "The book could not be added to Reading Journey. No Journey changes were made; try again."
		return refreshed, nil
	}
	refreshed, err := h.deckJourneyAction(ctx, owner, preparationID, bookID)
	if err != nil {
		return deckJourneyActionView{}, err
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
	view, err := h.buildJourneyView(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	activeCampaignID := r.URL.Query().Get("active_campaign_id")
	if !journeyHasActiveCampaign(view.Campaigns, activeCampaignID) {
		activeCampaignID = ""
	}
	if h.services.AnalysisInsights != nil && (len(view.Provisional) >= 2 || view.Goal != nil) {
		if provider, ok := h.services.AnalysisInsights.(journeyProjectionProvider); ok {
			titles := make(map[string]string)
			if view.Goal != nil {
				titles[view.Goal.Book.Source.ID] = journeyBookTitle(view.Goal.Book)
			}
			for _, item := range view.Provisional {
				titles[item.Book.Source.ID] = journeyBookTitle(item.Book)
			}
			comparison, projectionErr := journeyRouteComparison(r.Context(), provider, u.ID, titles)
			if projectionErr != nil {
				log.Printf("mouseion: Journey comparison unavailable for owner %s: %v", u.ID, projectionErr)
				view.RouteComparison = &routeComparisonView{ComparisonUnavailable: true}
			} else {
				view.RouteComparison = comparison
			}
		}
	}
	render(w, r, JourneyPage(u, h.csrf(w, r), view, r.URL.Query().Get("message"), r.URL.Query().Get("error"), activeCampaignID))
}

func (h *Handler) buildJourneyView(ctx context.Context, owner string) (journeyPageView, error) {
	journey, err := h.services.Store.GetReadingJourney(ctx, owner)
	if err != nil {
		return journeyPageView{}, err
	}
	goal, err := h.services.Store.GetPrimaryGoal(ctx, owner)
	if err != nil {
		return journeyPageView{}, err
	}
	books, err := h.services.Store.ListSourceMaterials(ctx, owner)
	if err != nil {
		return journeyPageView{}, err
	}
	bookByID := make(map[string]domain.SourceMaterialSummary, len(books))
	for _, book := range books {
		bookByID[book.Source.ID] = book
	}
	if reader, ok := h.services.Store.(interface {
		ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	}); ok {
		if myBooks, readErr := reader.ListMyBooksWithEvidence(ctx, owner); readErr == nil {
			for _, myBook := range myBooks {
				if myBook.Acquired != nil {
					bookByID[myBook.Book.ID] = *myBook.Acquired
				}
			}
		}
	}

	view := journeyPageView{Revision: journey.Revision}
	if primaryGoalIsActive(goal) {
		book, bookErr := h.journeyBook(ctx, owner, goal.BookID, bookByID)
		if bookErr != nil {
			return journeyPageView{}, bookErr
		}
		book.PrimaryGoal = true
		if err = h.addJourneyEvidence(ctx, owner, &book); err != nil {
			return journeyPageView{}, err
		}
		campaigns, _ := h.services.Store.ListLearningCampaigns(ctx, owner)
		preparations, _ := h.services.Store.ListUnassignedReadyDeckPreparations(ctx, owner)
		book.GoalResidual = h.goalResidual(ctx, owner, book.Book.Source.ID, journeyBookTitle(book.Book), campaigns)
		book.GoalDeckAvailable = goalBookHasDeck(book.Book.Source.ID, campaigns, preparations)
		book.GoalUnassessed = !strings.EqualFold(strings.TrimSpace(book.Book.AnalysisStatus), "analyzed")
		book.GoalReadingOnly = !book.GoalDeckAvailable
		view.Residual = book.GoalResidual
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
		if err = h.addJourneyEvidence(ctx, owner, &book); err != nil {
			return journeyPageView{}, err
		}
		view.Provisional = append(view.Provisional, book)
	}
	for i := range view.Provisional {
		view.Provisional[i].CanMoveEarlier = i > 0
		view.Provisional[i].CanMoveLater = i < len(view.Provisional)-1
	}

	if h.services.PreparedDeck != nil {
		view.Campaigns, view.Prepared, err = h.campaignOperations(ctx, owner, bookByID)
		if err != nil {
			return journeyPageView{}, err
		}
	}
	return view, nil
}

func journeyHasActiveCampaign(campaigns []campaignView, id string) bool {
	if id == "" {
		return false
	}
	for _, campaign := range campaigns {
		if campaign.Campaign.ID == id && campaign.Campaign.Status == domain.CampaignActive {
			return true
		}
	}
	return false
}

func (h *Handler) journeyBook(ctx context.Context, owner, bookID string, bookByID map[string]domain.SourceMaterialSummary) (journeyBookView, error) {
	if book, ok := bookByID[bookID]; ok {
		return journeyBookView{Book: book, BookID: bookID}, nil
	}
	// Goal and Journey membership are allowed to exist before acquisition. Keep
	// that identity visible instead of silently dropping it from the surface.
	book, err := h.services.Store.GetBook(ctx, owner, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: bookID, Title: "Book details unavailable", OwnerID: owner}}, BookID: bookID}, nil
		}
		return journeyBookView{}, err
	}
	return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: book.ID, OwnerID: owner, Title: book.Title, Language: book.LanguageTag}}, BookID: book.ID}, nil
}

func (h *Handler) addJourneyEvidence(ctx context.Context, owner string, book *journeyBookView) error {
	status := strings.ToLower(book.Book.AnalysisStatus + " " + book.Book.AnalysisState)
	if strings.Contains(status, "stale") || strings.Contains(status, "needs review") {
		return nil
	}
	if book.Book.AnalysisStatus != "analyzed" || book.Book.CorpusID == "" || h.services.AnalysisInsights == nil {
		book.StatisticsUnavailable = book.Book.AnalysisStatus == "analyzed"
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

func (h *Handler) campaignOperations(ctx context.Context, owner string, bookByID map[string]domain.SourceMaterialSummary) ([]campaignView, []preparedCampaignOption, error) {
	campaigns, err := h.services.Store.ListLearningCampaigns(ctx, owner)
	if err != nil {
		return nil, nil, err
	}
	views := make([]campaignView, 0, len(campaigns))
	for _, campaign := range campaigns {
		deck, deckErr := h.services.PreparedDeck.Get(ctx, owner, campaign.DeckPreparationID)
		if deckErr != nil {
			return nil, nil, deckErr
		}
		book := bookByID[campaign.SourceMaterialID]
		if book.Source.ID == "" {
			bookView, bookErr := h.journeyBook(ctx, owner, campaign.SourceMaterialID, bookByID)
			if bookErr != nil {
				return nil, nil, bookErr
			}
			book = bookView.Book
		}
		view := campaignView{Campaign: campaign, Book: book, Deck: deck}
		if campaign.Status == domain.CampaignActive {
			if counter, ok := h.services.Store.(campaignVocabularyCounter); ok {
				if count, countErr := counter.CountCampaignVocabularyToGraduate(ctx, owner, campaign.ID); countErr == nil {
					view.GraduatableLemmaCount = &count
				}
			}
		}
		views = append(views, view)
	}
	ready, err := h.services.Store.ListUnassignedReadyDeckPreparations(ctx, owner)
	if err != nil {
		return nil, nil, err
	}
	prepared := make([]preparedCampaignOption, 0, len(ready))
	for _, deck := range ready {
		if book, ok := bookByID[deck.SourceMaterialID]; ok {
			prepared = append(prepared, preparedCampaignOption{Book: book, Deck: deck})
		}
	}
	return views, prepared, nil
}
