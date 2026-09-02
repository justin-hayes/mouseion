package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/library")
}
func (h *Handler) library(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	goal, goalErr := h.services.Store.GetPrimaryGoal(r.Context(), u.ID)
	if goalErr != nil {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksPage(u, h.csrf(w, r), nil, "", "My Books could not be loaded. Try refreshing the page.", "", false, MyBooksBrowseState{}))
		return
	}
	query, language, page := parseMyBooksBrowseRequest(r.URL)
	var books []domain.MyBook
	var err error
	var browse MyBooksBrowseState
	if reader, ok := h.services.Store.(interface {
		ListMyBooksBrowse(context.Context, string, string, string, int, int) (persistence.MyBooksBrowseResult, error)
	}); ok {
		result, readErr := reader.ListMyBooksBrowse(r.Context(), u.ID, query, language, myBooksPageOffset(page), myBooksPageSize)
		err = readErr
		books = result.Items
		browse = MyBooksBrowseState{
			Enabled:         true,
			Query:           query,
			Language:        language,
			AllCount:        result.AllCount,
			Total:           result.Total,
			Page:            page,
			PageCount:       myBooksPageCount(result.Total),
			TextNoMatch:     query != "" && result.Total == 0 && language == "",
			CombinedNoMatch: query != "" && language != "" && result.Total == 0,
		}
		for _, count := range result.Counts {
			browse.Counts = append(browse.Counts, MyBooksLanguageCount{Tag: count.Tag, Count: count.Count})
		}
		if err == nil && result.Total > 0 && myBooksPageOffset(page) >= result.Total {
			lastPage := myBooksPageCount(result.Total)
			http.Redirect(w, r, myBooksBrowseURL(query, language, lastPage), http.StatusSeeOther)
			return
		}
	} else if reader, ok := h.services.Store.(interface {
		ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	}); ok {
		books, err = reader.ListMyBooksWithEvidence(r.Context(), u.ID)
	} else {
		// Compatibility for lightweight stores used by older web tests. The
		// production PostgresStore always supplies the complete read model.
		var acquired []domain.SourceMaterialSummary
		acquired, err = h.services.Store.ListSourceMaterials(r.Context(), u.ID)
		for _, source := range acquired {
			books = append(books, domain.MyBook{Book: domain.Book{ID: source.Source.ID, OwnerID: source.Source.OwnerID, Title: source.Source.Title, LanguageState: domain.LanguageChosen, LanguageTag: source.Source.Language}, Acquired: &source, EvidenceState: domain.MyBookAnalyzed})
		}
	}
	if err != nil {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksPage(u, h.csrf(w, r), nil, "", "My Books could not be loaded. Try refreshing the page.", goal.BookID, false, browse))
		return
	}
	connections, err := h.services.Store.ListOpdsConnections(r.Context(), u.ID)
	if err != nil {
		renderStatus(w, r, http.StatusInternalServerError, MyBooksPage(u, h.csrf(w, r), nil, "", "My Books could not be loaded. Try refreshing the page.", goal.BookID, false, browse))
		return
	}
	goalBookID := ""
	if primaryGoalIsActive(goal) {
		goalBookID = goal.BookID
	}
	if isHTMX(r) && browse.Enabled {
		render(w, r, MyBooksResults(h.csrf(w, r), books, goalBookID, browse))
		return
	}
	render(w, r, MyBooksPage(u, h.csrf(w, r), books, r.URL.Query().Get("message"), r.URL.Query().Get("error"), goalBookID, len(connections) > 0, browse))
}

func (h *Handler) createMetadataBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	state := strings.TrimSpace(r.FormValue("language_state"))
	tag := strings.TrimSpace(r.FormValue("language_tag"))
	book, err := domain.NewBook(u.ID, strings.TrimSpace(r.FormValue("title")), domain.MetadataProvenanceManualEntry, state, tag)
	if err != nil {
		redirect(w, r, "/library?error="+url.QueryEscape("The book was not added. Enter a title and choose an explicit language state."))
		return
	}
	created, err := h.services.Store.CreateBook(r.Context(), book)
	if err != nil {
		redirect(w, r, "/library?error="+url.QueryEscape("The book could not be added to My Books. Check the details and try again."))
		return
	}
	if err = h.services.Store.AddBookToMyBooks(r.Context(), u.ID, created.ID); err != nil {
		redirect(w, r, "/library?error="+url.QueryEscape("The book identity was recorded, but My Books membership could not be activated. Refresh and try again."))
		return
	}
	redirect(w, r, "/library?message="+url.QueryEscape("Book added to My Books. It has metadata only until you acquire an EPUB."))
}

func (h *Handler) removeBookFromMyBooks(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	if err := h.services.Store.RemoveBookFromMyBooks(r.Context(), u.ID, r.PathValue("id")); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/library?message="+url.QueryEscape("Book removed from My Books. Acquired content and history remain."))
}

type campaignView struct {
	Campaign              domain.LearningCampaign
	Book                  domain.SourceMaterialSummary
	Deck                  domain.DeckPreparation
	Coverage              *domain.AnalysisCoverage
	GraduatableLemmaCount *int
}

type campaignVocabularyCounter interface {
	CountCampaignVocabularyToGraduate(context.Context, string, string) (int, error)
}

type preparedCampaignOption struct {
	Book domain.SourceMaterialSummary
	Deck domain.DeckPreparation
}

func (h *Handler) campaigns(w http.ResponseWriter, r *http.Request) {
	// /campaigns is retained as a compatibility endpoint for old bookmarks and
	// action links. The learner-facing plan now has one canonical destination.
	location := "/journey"
	if r.URL.RawQuery != "" {
		location += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, location, http.StatusMovedPermanently)
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
		redirect(w, r, "/journey?error="+url.QueryEscape("Only a prepared campaign can be started."))
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
		redirect(w, r, "/journey?error="+url.QueryEscape("Only a prepared campaign can be started."))
		return
	}
	if err != nil {
		redirectCampaignMutationFailure(w, r, "The campaign could not be started. Its state was not changed; review Reading Journey and try again.")
		return
	}
	redirect(w, r, "/journey?message="+url.QueryEscape("Learning campaign started."))
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
		redirect(w, r, "/journey?error="+url.QueryEscape("Only an active or prepared campaign can be abandoned."))
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
		redirect(w, r, "/journey?error="+url.QueryEscape("Only an active or prepared campaign can be abandoned."))
		return
	}
	if err != nil {
		redirectCampaignMutationFailure(w, r, "The campaign could not be abandoned. Its book, deck, and history are unchanged; review Reading Journey and try again.")
		return
	}
	redirect(w, r, "/journey?message="+url.QueryEscape("Campaign abandoned. Its ungraduated vocabulary is available again."))
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
		redirect(w, r, "/journey?error="+url.QueryEscape("Only the active campaign can be updated."))
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
		redirect(w, r, "/journey?error="+url.QueryEscape("Only the active campaign can be updated."))
		return
	}
	if err != nil {
		redirectCampaignMutationFailure(w, r, "Campaign progress could not be updated. No progress was changed; review Reading Journey and try again.")
		return
	}
	if updated.Status == domain.CampaignComplete {
		message = "Campaign complete. Its vocabulary is now known."
	}
	redirect(w, r, "/journey?message="+url.QueryEscape(message))
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
	redirect(w, r, "/journey?error="+url.QueryEscape("This campaign changed since this page was loaded. Its current state is unchanged by this request; review Reading Journey before trying again."))
}

func redirectCampaignMutationFailure(w http.ResponseWriter, r *http.Request, message string) {
	redirect(w, r, "/journey?error="+url.QueryEscape(message))
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
	location := "/journey?error=" + url.QueryEscape("Finish or abandon the active campaign before starting another.")
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
		return "This prepared campaign has no active reservation to release; its assigned vocabulary remains eligible for future decks unless independently known."
	}
	return "Any active reservation is released, so ungraduated vocabulary can be eligible for future decks again unless independently known."
}

func hasCampaignHistory(campaigns []campaignView) bool {
	return hasCampaignStatus(campaigns, domain.CampaignComplete) || hasCampaignStatus(campaigns, domain.CampaignAbandoned)
}
func campaignStatusLabel(status domain.CampaignStatus) string {
	switch status {
	case domain.CampaignQueued:
		return "Prepared"
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
		return "Prepared"
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
		return "Prepared"
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
