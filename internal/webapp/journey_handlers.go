package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type journeyBookView struct {
	Book                  domain.SourceMaterialSummary
	Position              int
	PrimaryGoal           bool
	CanMoveEarlier        bool
	CanMoveLater          bool
	Coverage              *domain.AnalysisCoverage
	StatisticsUnavailable bool
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
	Goal        *journeyBookView
	Provisional []journeyBookView
	Revision    int64
	Campaigns   []campaignView
	Prepared    []preparedCampaignOption
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

	view := journeyPageView{Revision: journey.Revision}
	if goal.BookID != "" {
		book, bookErr := h.journeyBook(ctx, owner, goal.BookID, bookByID)
		if bookErr != nil {
			return journeyPageView{}, bookErr
		}
		book.PrimaryGoal = true
		if err = h.addJourneyEvidence(ctx, owner, &book); err != nil {
			return journeyPageView{}, err
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
		return journeyBookView{Book: book}, nil
	}
	// Goal and Journey membership are allowed to exist before acquisition. Keep
	// that identity visible instead of silently dropping it from the surface.
	book, err := h.services.Store.GetBook(ctx, owner, bookID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: bookID, Title: "Book details unavailable", OwnerID: owner}}}, nil
		}
		return journeyBookView{}, err
	}
	return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: book.ID, OwnerID: owner, Title: book.Title, Language: book.LanguageTag}}}, nil
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
