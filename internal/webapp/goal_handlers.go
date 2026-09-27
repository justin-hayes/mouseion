package webapp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
)

// goalSectionView is the server-truth fragment returned after an HTMX Goal
// mutation.
type goalSectionView struct {
	Goal    *journeyBookView
	Message string
	Error   string
}

const (
	goalStaleMessage            = "This current reading changed since this page was loaded. No changes were made; review Reading before trying again."
	goalLanguageRequiredMessage = "Choose a study language before starting a book."
	goalDeckRetryMessage        = "Deck preparation retry queued."
	goalDeckCancelledMessage    = "Deck preparation cancelled."
	goalDeckUnavailableMessage  = "Deck preparation is unavailable. The current reading and its frozen snapshot remain unchanged; retry preparation when ready."
)

func (h *Handler) goalBookTitle(ctx context.Context, owner, bookID string) string {
	books, err := h.services.Store.Books.ListSourceMaterials(ctx, owner)
	if err == nil {
		for _, book := range books {
			if book.Source.ID == bookID {
				return canonicalBookTitle(book)
			}
		}
	}
	if book, err := h.services.Store.Books.GetBook(ctx, owner, bookID); err == nil && strings.TrimSpace(book.Title) != "" {
		return book.Title
	}
	return bookID
}

func (h *Handler) goalSection(ctx context.Context, owner, message, pageError string) (goalSectionView, error) {
	language, _ := activeStudyLanguageForContext(ctx)
	journey, err := h.buildJourneyView(ctx, owner, language)
	if err != nil {
		return goalSectionView{}, err
	}
	return goalSectionView{Goal: journey.Goal, Message: message, Error: pageError}, nil
}

func (h *Handler) respondGoal(w http.ResponseWriter, r *http.Request, message, pageError, focusBookID string) {
	if isHTMX(r) {
		section, err := h.goalSection(r.Context(), user(r).ID, message, pageError)
		if err != nil {
			fail(w, err)
			return
		}
		render(w, r, GoalSection(section.Goal, h.csrf(w, r), section.Message, section.Error, focusBookID))
		return
	}
	query := url.Values{}
	if message != "" {
		query.Set("message", message)
	}
	if pageError != "" {
		query.Set("error", pageError)
	}
	location := "/reading"
	if r.FormValue("return_to") == "/reading" {
		location = "/reading"
	}
	if encoded := query.Encode(); encoded != "" {
		location += "?" + encoded
	}
	redirect(w, r, location)
}

func (h *Handler) retryCurrentReadingDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	goal, ok := h.currentReadingForDeckAction(w, r, owner)
	if !ok {
		return
	}
	if goal.SnapshotSize == 0 {
		h.respondGoal(w, r, "No deck is required for this empty Goal snapshot.", "", goal.BookID)
		return
	}
	if h.services.PreparedDeck == nil {
		h.respondGoal(w, r, "", goalDeckUnavailableMessage, goal.BookID)
		return
	}
	_, err := h.submitOrRetryCurrentReadingDeck(r.Context(), owner, goal)
	if err != nil {
		log.Printf("primary goal deck retry owner=%s book=%s: %v", owner, goal.BookID, err)
		h.respondGoal(w, r, "", goalDeckUnavailableMessage, goal.BookID)
		return
	}
	h.respondGoal(w, r, goalDeckRetryMessage, "", goal.BookID)
}

func (h *Handler) cancelCurrentReadingDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	goal, ok := h.currentReadingForDeckAction(w, r, owner)
	if !ok {
		return
	}
	reader, ok := h.services.PreparedDeck.(PreparedDeckForGoalSnapshot)
	if !ok {
		h.respondGoal(w, r, "", goalDeckUnavailableMessage, goal.BookID)
		return
	}
	preparation, err := reader.GetForGoalSnapshot(r.Context(), owner, goal.SnapshotID)
	if err != nil || !currentReadingPreparationMatches(preparation, owner, goal) {
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			log.Printf("primary goal deck cancel lookup owner=%s book=%s: %v", owner, goal.BookID, err)
		}
		h.respondGoal(w, r, "", goalDeckUnavailableMessage, goal.BookID)
		return
	}
	if _, err = h.services.PreparedDeck.Cancel(r.Context(), owner, preparation.ID); err != nil {
		log.Printf("primary goal deck cancel owner=%s book=%s: %v", owner, goal.BookID, err)
		h.respondGoal(w, r, "", goalDeckUnavailableMessage, goal.BookID)
		return
	}
	h.respondGoal(w, r, goalDeckCancelledMessage, "", goal.BookID)
}

func (h *Handler) currentReadingForDeckAction(w http.ResponseWriter, r *http.Request, owner string) (domain.CurrentReading, bool) {
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		h.respondGoal(w, r, "", goalLanguageRequiredMessage, "")
		return domain.CurrentReading{}, false
	}
	goal, err := h.services.Store.CurrentReading.GetCurrentReading(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return domain.CurrentReading{}, false
	}
	if !goal.IsActive() || goal.BookID != strings.TrimSpace(r.PathValue("id")) {
		h.respondGoal(w, r, "", goalStaleMessage, goal.BookID)
		return domain.CurrentReading{}, false
	}
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if expectedSnapshotID == "" {
		// Old focused-deck bookmarks remain safe: this identifies the same
		// immutable snapshot and cannot mutate the current-reading lifecycle.
		expectedSnapshotID = strings.TrimSpace(r.FormValue("expected_goal_snapshot_id"))
	}
	if expectedSnapshotID == "" || goal.SnapshotID != expectedSnapshotID {
		h.respondGoal(w, r, "", goalStaleMessage, goal.BookID)
		return domain.CurrentReading{}, false
	}
	return goal, true
}

func (h *Handler) submitOrRetryCurrentReadingDeck(ctx context.Context, owner string, goal domain.CurrentReading) (prepareddeck.Handle, error) {
	if reader, ok := h.services.PreparedDeck.(PreparedDeckForGoalSnapshot); ok {
		preparation, err := reader.GetForGoalSnapshot(ctx, owner, goal.SnapshotID)
		switch {
		case err == nil:
			if !currentReadingPreparationMatches(preparation, owner, goal) {
				return prepareddeck.Handle{}, persistence.ErrInvalidTransition
			}
			return h.services.PreparedDeck.Retry(ctx, owner, preparation.ID)
		case !errors.Is(err, persistence.ErrNotFound):
			return prepareddeck.Handle{}, err
		}
	}
	return h.services.PreparedDeck.SubmitForGoal(ctx, owner, goal.AnalysisRunID, goal.SnapshotID)
}

func currentReadingPreparationMatches(preparation domain.DeckPreparation, owner string, goal domain.CurrentReading) bool {
	return (preparation.OwnerID == "" || preparation.OwnerID == owner) &&
		preparation.SourceMaterialID == goal.SourceMaterialID &&
		preparation.AnalysisRunID == goal.AnalysisRunID &&
		preparation.GoalSnapshotID == goal.SnapshotID
}
