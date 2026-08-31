package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/library")
}
func (h *Handler) library(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	books, err := h.services.Store.ListSourceMaterials(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, LibraryPage(u, h.csrf(w, r), books, r.URL.Query().Get("message")))
}

type campaignView struct {
	Campaign              domain.LearningCampaign
	Book                  domain.SourceMaterialSummary
	Deck                  domain.DeckPreparation
	Coverage              *domain.AnalysisCoverage
	GraduatableLemmaCount *int
	QueuePosition         int
}

type campaignVocabularyCounter interface {
	CountCampaignVocabularyToGraduate(context.Context, string, string) (int, error)
}

type preparedCampaignOption struct {
	Book domain.SourceMaterialSummary
	Deck domain.DeckPreparation
}

func (h *Handler) campaigns(w http.ResponseWriter, r *http.Request) {
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	u := user(r)
	campaigns, err := h.services.Store.ListLearningCampaigns(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	books, err := h.services.Store.ListSourceMaterials(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	bookByID := make(map[string]domain.SourceMaterialSummary, len(books))
	for _, book := range books {
		bookByID[book.Source.ID] = book
	}
	views := make([]campaignView, 0, len(campaigns))
	queuedPosition := 0
	for _, campaign := range campaigns {
		deck, deckErr := h.services.PreparedDeck.Get(r.Context(), u.ID, campaign.DeckPreparationID)
		if deckErr != nil {
			fail(w, deckErr)
			return
		}
		book := bookByID[campaign.SourceMaterialID]
		view := campaignView{Campaign: campaign, Book: book, Deck: deck}
		if campaign.Status == domain.CampaignQueued {
			queuedPosition++
			view.QueuePosition = queuedPosition
		}
		if campaign.Status == domain.CampaignActive {
			if counter, ok := h.services.Store.(campaignVocabularyCounter); ok {
				if count, countErr := counter.CountCampaignVocabularyToGraduate(r.Context(), u.ID, campaign.ID); countErr == nil {
					view.GraduatableLemmaCount = &count
				}
			}
		}
		if campaign.Status == domain.CampaignQueued && book.AnalysisStatus == "analyzed" && h.services.AnalysisInsights != nil {
			coverage, coverageErr := h.services.AnalysisInsights.Coverage(r.Context(), u.ID, book.CorpusID)
			if coverageErr != nil && !errors.Is(coverageErr, analysisinsights.ErrStatisticsUnavailable) {
				fail(w, coverageErr)
				return
			}
			if coverageErr == nil {
				view.Coverage = &coverage
			}
		}
		views = append(views, view)
	}
	ready, err := h.services.Store.ListUnassignedReadyDeckPreparations(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	options := make([]preparedCampaignOption, 0, len(ready))
	for _, deck := range ready {
		if book, ok := bookByID[deck.SourceMaterialID]; ok {
			options = append(options, preparedCampaignOption{Book: book, Deck: deck})
		}
	}
	activeCampaignID := r.URL.Query().Get("active_campaign_id")
	if activeCampaignID != "" {
		found := false
		for _, item := range views {
			if item.Campaign.ID == activeCampaignID && item.Campaign.Status == domain.CampaignActive {
				found = true
				break
			}
		}
		if !found {
			activeCampaignID = ""
		}
	}
	render(w, r, CampaignsPage(u, h.csrf(w, r), views, options, r.URL.Query().Get("message"), r.URL.Query().Get("error"), activeCampaignID))
}

func (h *Handler) queueCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	u := user(r)
	deck, err := h.services.PreparedDeck.Get(r.Context(), u.ID, r.FormValue("deck_preparation_id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		redirectCampaignMutationFailure(w, r, "The campaign could not be added to the queue. Your prepared deck was not changed; review Learning and try again.")
		return
	}
	if _, err = h.services.Store.CreateLearningCampaign(r.Context(), u.ID, deck.SourceMaterialID, deck.ID); err != nil {
		if errors.Is(err, persistence.ErrInvalidTransition) {
			redirect(w, r, "/campaigns?error="+url.QueryEscape("Only a ready, unassigned deck can be added to the queue."))
			return
		}
		redirect(w, r, "/campaigns?error="+url.QueryEscape("The campaign could not be added to the queue. Your prepared deck was not changed; review Learning and try again."))
		return
	}
	redirect(w, r, "/campaigns?message="+url.QueryEscape("Book and deck added to your learning queue."))
}

func (h *Handler) activateCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	expected, ok := campaignExpectedState(r)
	if !ok {
		redirectCampaignStale(w, r)
		return
	}
	if expected.Status != domain.CampaignQueued || expected.BookProgress != domain.BookQueued || expected.DeckProgress != domain.DeckQueued {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only a queued campaign can be started."))
		return
	}
	u := user(r)
	_, err := h.services.Store.UpdateLearningCampaignProgress(r.Context(), u.ID, r.PathValue("id"), expected, domain.BookReading, domain.DeckStudying)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, persistence.ErrStaleCampaignState) {
		redirectCampaignStale(w, r)
		return
	}
	if errors.Is(err, persistence.ErrActiveCampaign) {
		h.redirectCampaignActivationBlocked(w, r, u.ID)
		return
	}
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only a queued campaign can be started."))
		return
	}
	if err != nil {
		redirectCampaignMutationFailure(w, r, "The campaign could not be started. Your queue was not changed; review Learning and try again.")
		return
	}
	redirect(w, r, "/campaigns?message="+url.QueryEscape("Learning campaign started."))
}

func (h *Handler) finishCampaignBook(w http.ResponseWriter, r *http.Request) {
	h.updateActiveCampaignProgress(w, r, true)
}

func (h *Handler) reviewCampaignDeck(w http.ResponseWriter, r *http.Request) {
	h.updateActiveCampaignProgress(w, r, false)
}

func (h *Handler) abandonCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	expected, ok := campaignExpectedState(r)
	if !ok {
		redirectCampaignStale(w, r)
		return
	}
	if expected.Status != domain.CampaignActive && expected.Status != domain.CampaignQueued {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only an active or queued campaign can be abandoned."))
		return
	}
	u := user(r)
	_, err := h.services.Store.AbandonLearningCampaign(r.Context(), u.ID, r.PathValue("id"), expected)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, persistence.ErrStaleCampaignState) {
		redirectCampaignStale(w, r)
		return
	}
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only an active or queued campaign can be abandoned."))
		return
	}
	if err != nil {
		redirectCampaignMutationFailure(w, r, "The campaign could not be abandoned. Its book, deck, and history are unchanged; review Learning and try again.")
		return
	}
	redirect(w, r, "/campaigns?message="+url.QueryEscape("Campaign abandoned. Its ungraduated vocabulary is available again."))
}

func (h *Handler) updateActiveCampaignProgress(w http.ResponseWriter, r *http.Request, finishBook bool) {
	if !h.checkCSRF(w, r) {
		return
	}
	expected, ok := campaignExpectedState(r)
	if !ok {
		redirectCampaignStale(w, r)
		return
	}
	if expected.Status != domain.CampaignActive {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only the active campaign can be updated."))
		return
	}
	u := user(r)
	nextBook, nextDeck := expected.BookProgress, expected.DeckProgress
	message := "Deck marked reviewed."
	if finishBook {
		nextBook = domain.BookFinished
		message = "Book marked finished."
	} else {
		nextDeck = domain.DeckReviewed
	}
	updated, err := h.services.Store.UpdateLearningCampaignProgress(r.Context(), u.ID, r.PathValue("id"), expected, nextBook, nextDeck)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, persistence.ErrStaleCampaignState) {
		redirectCampaignStale(w, r)
		return
	}
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only the active campaign can be updated."))
		return
	}
	if err != nil {
		redirectCampaignMutationFailure(w, r, "Campaign progress could not be updated. No progress was changed; review Learning and try again.")
		return
	}
	if updated.Status == domain.CampaignComplete {
		message = "Campaign complete. Its vocabulary is now known."
	}
	redirect(w, r, "/campaigns?message="+url.QueryEscape(message))
}

func campaignExpectedState(r *http.Request) (persistence.LearningCampaignExpectedState, bool) {
	expected := persistence.LearningCampaignExpectedState{
		Status:       domain.CampaignStatus(r.FormValue("expected_campaign_status")),
		BookProgress: domain.BookProgress(r.FormValue("expected_book_progress")),
		DeckProgress: domain.DeckProgress(r.FormValue("expected_deck_progress")),
	}
	return expected, expected.Status != "" && expected.BookProgress != "" && expected.DeckProgress != ""
}

func redirectCampaignStale(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/campaigns?error="+url.QueryEscape("This campaign changed since this page was loaded. Its current state is unchanged by this request; review Learning before trying again."))
}

func redirectCampaignMutationFailure(w http.ResponseWriter, r *http.Request, message string) {
	redirect(w, r, "/campaigns?error="+url.QueryEscape(message))
}

func (h *Handler) redirectCampaignActivationBlocked(w http.ResponseWriter, r *http.Request, owner string) {
	activeCampaignID := ""
	if campaigns, err := h.services.Store.ListLearningCampaigns(r.Context(), owner); err == nil {
		for _, campaign := range campaigns {
			if campaign.Status == domain.CampaignActive {
				activeCampaignID = campaign.ID
				break
			}
		}
	}
	location := "/campaigns?error=" + url.QueryEscape("Finish or abandon the active campaign before starting another.")
	if activeCampaignID != "" {
		location += "&active_campaign_id=" + url.QueryEscape(activeCampaignID)
	}
	redirect(w, r, location)
}
func hasCampaignStatus(campaigns []campaignView, status domain.CampaignStatus) bool {
	for _, item := range campaigns {
		if item.Campaign.Status == status {
			return true
		}
	}
	return false
}
func campaignAnchorID(id string) string { return "campaign-" + id }

func campaignCardClass(status domain.CampaignStatus) string {
	if status == domain.CampaignActive {
		return "resource-card campaign-card campaign-card--active"
	}
	return "resource-card campaign-card"
}

func campaignRemainingBookText(status domain.BookProgress) string {
	if status == domain.BookFinished {
		return ""
	}
	return " — finish the book to complete this condition."
}

func campaignRemainingDeckText(status domain.DeckProgress) string {
	if status == domain.DeckReviewed {
		return ""
	}
	return " — review the prepared deck to complete this condition."
}

func campaignCompletionLabel(item campaignView) string {
	if item.GraduatableLemmaCount == nil {
		return "Complete campaign and add its vocabulary to known"
	}
	return fmt.Sprintf("Complete campaign and add %d lemmas to known vocabulary", *item.GraduatableLemmaCount)
}

func campaignGraduationOutcome(item campaignView) string {
	if item.GraduatableLemmaCount == nil {
		return "The assigned campaign vocabulary will be added to known vocabulary when this campaign completes."
	}
	return fmt.Sprintf("%d assigned lemmas not already independently known will be added to known vocabulary when this campaign completes.", *item.GraduatableLemmaCount)
}

func campaignAbandonmentOutcome(status domain.CampaignStatus) string {
	if status == domain.CampaignQueued {
		return "This queued campaign has no active reservation to release; its assigned vocabulary remains eligible for future decks unless independently known."
	}
	return "Any active reservation is released, so ungraduated vocabulary can be eligible for future decks again unless independently known."
}

func hasCampaignHistory(campaigns []campaignView) bool {
	return hasCampaignStatus(campaigns, domain.CampaignComplete) || hasCampaignStatus(campaigns, domain.CampaignAbandoned)
}
func campaignStatusLabel(status domain.CampaignStatus) string {
	switch status {
	case domain.CampaignQueued:
		return "Queued"
	case domain.CampaignActive:
		return "Active"
	case domain.CampaignComplete:
		return "Complete"
	case domain.CampaignAbandoned:
		return "Abandoned"
	}
	return string(status)
}
func campaignBookLabel(status domain.BookProgress) string {
	switch status {
	case domain.BookQueued:
		return "Queued"
	case domain.BookReading:
		return "Reading"
	case domain.BookFinished:
		return "Finished"
	case domain.BookAbandoned:
		return "Abandoned"
	}
	return string(status)
}
func campaignDeckLabel(status domain.DeckProgress) string {
	switch status {
	case domain.DeckQueued:
		return "Queued"
	case domain.DeckStudying:
		return "Studying"
	case domain.DeckReviewed:
		return "Reviewed"
	case domain.DeckAbandoned:
		return "Abandoned"
	}
	return string(status)
}

func campaignProgressTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return " · " + value.UTC().Format("2006-01-02 15:04 UTC")
}
