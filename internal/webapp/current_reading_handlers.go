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
type currentReadingSectionView struct {
	CurrentReading *readingBookView
	Message        string
	Error          string
}

const (
	currentReadingStaleMessage            = "This current reading changed since this page was loaded. No changes were made; review Reading before trying again."
	currentReadingLanguageRequiredMessage = "Choose a study language before starting a book."
	currentReadingDeckRetryMessage        = "Deck preparation retry queued."
	currentReadingDeckCancelledMessage    = "Deck preparation cancelled."
	currentReadingDeckUnavailableMessage  = "Deck preparation is unavailable. The current reading and its frozen snapshot remain unchanged; retry preparation when ready."
)

func (h *Handler) currentReadingBookTitle(ctx context.Context, owner, bookID string) string {
	books, err := h.services.Store.Reading.ListSourceMaterials(ctx, owner)
	if err == nil {
		for _, book := range books {
			if book.Source.ID == bookID {
				return canonicalBookTitle(book)
			}
		}
	}
	if book, err := h.services.Store.Reading.GetBook(ctx, owner, bookID); err == nil && strings.TrimSpace(book.Title) != "" {
		return book.Title
	}
	return bookID
}

func (h *Handler) currentReadingSection(ctx context.Context, owner, message, pageError string) (currentReadingSectionView, error) {
	language, _ := activeStudyLanguageForContext(ctx)
	readingView, err := h.buildReadingView(ctx, owner, language)
	if err != nil {
		return currentReadingSectionView{}, err
	}
	return currentReadingSectionView{CurrentReading: readingView.CurrentReading, Message: message, Error: pageError}, nil
}

func (h *Handler) respondCurrentReading(w http.ResponseWriter, r *http.Request, message, pageError, focusBookID string) {
	if isPartialHTMXRequest(r) {
		section, err := h.currentReadingSection(r.Context(), user(r).ID, message, pageError)
		if err != nil {
			fail(w, err)
			return
		}
		render(w, r, CurrentReadingSection(section.CurrentReading, h.csrf(w, r), section.Message, section.Error, focusBookID))
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

// expectedCommitmentMatches reports whether the request names the exact
// commitment snapshot; a missing identity never matches.
func expectedCommitmentMatches(r *http.Request, snapshotID string) bool {
	expected := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	return expected != "" && snapshotID != "" && expected == snapshotID
}

func (h *Handler) retryCurrentReadingDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	currentReading, ok := h.currentReadingForDeckAction(w, r, owner)
	if !ok {
		return
	}
	if currentReading.SnapshotSize == 0 {
		h.respondCurrentReading(w, r, "No deck is required for this empty Goal snapshot.", "", currentReading.BookID)
		return
	}
	if h.services.PreparedDeck == nil {
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, currentReading.BookID)
		return
	}
	_, err := h.submitOrRetryCurrentReadingDeck(r.Context(), owner, currentReading)
	if err != nil {
		log.Printf("primary goal deck retry owner=%s book=%s: %v", owner, currentReading.BookID, err)
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, currentReading.BookID)
		return
	}
	h.respondCurrentReading(w, r, currentReadingDeckRetryMessage, "", currentReading.BookID)
}

func (h *Handler) cancelCurrentReadingDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	currentReading, ok := h.currentReadingForDeckAction(w, r, owner)
	if !ok {
		return
	}
	preparation, err := h.services.PreparedDeck.GetForGoalSnapshot(r.Context(), owner, currentReading.SnapshotID)
	if err != nil || !currentReadingPreparationMatches(preparation, owner, currentReading) {
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			log.Printf("primary goal deck cancel lookup owner=%s book=%s: %v", owner, currentReading.BookID, err)
		}
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, currentReading.BookID)
		return
	}
	if _, err = h.services.PreparedDeck.Cancel(r.Context(), owner, preparation.ID); err != nil {
		log.Printf("primary goal deck cancel owner=%s book=%s: %v", owner, currentReading.BookID, err)
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, currentReading.BookID)
		return
	}
	h.respondCurrentReading(w, r, currentReadingDeckCancelledMessage, "", currentReading.BookID)
}

func (h *Handler) currentReadingForDeckAction(w http.ResponseWriter, r *http.Request, owner string) (domain.CurrentReading, bool) {
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		h.respondCurrentReading(w, r, "", currentReadingLanguageRequiredMessage, "")
		return domain.CurrentReading{}, false
	}
	currentReading, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return domain.CurrentReading{}, false
	}
	if !currentReading.IsActive() || currentReading.BookID != strings.TrimSpace(r.PathValue("id")) {
		h.respondCurrentReading(w, r, "", currentReadingStaleMessage, currentReading.BookID)
		return domain.CurrentReading{}, false
	}
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if expectedSnapshotID == "" {
		// Old focused-deck bookmarks remain safe: this identifies the same
		// immutable snapshot and cannot mutate the current-reading lifecycle.
		expectedSnapshotID = strings.TrimSpace(r.FormValue("expected_goal_snapshot_id"))
	}
	if expectedSnapshotID == "" || currentReading.SnapshotID != expectedSnapshotID {
		h.respondCurrentReading(w, r, "", currentReadingStaleMessage, currentReading.BookID)
		return domain.CurrentReading{}, false
	}
	return currentReading, true
}

func (h *Handler) submitOrRetryCurrentReadingDeck(ctx context.Context, owner string, currentReading domain.CurrentReading) (prepareddeck.Handle, error) {
	preparation, err := h.services.PreparedDeck.GetForGoalSnapshot(ctx, owner, currentReading.SnapshotID)
	switch {
	case err == nil:
		if !currentReadingPreparationMatches(preparation, owner, currentReading) {
			return prepareddeck.Handle{}, persistence.ErrInvalidTransition
		}
		return h.services.PreparedDeck.Retry(ctx, owner, preparation.ID)
	case !errors.Is(err, persistence.ErrNotFound):
		return prepareddeck.Handle{}, err
	}
	return h.services.PreparedDeck.SubmitForGoal(ctx, owner, currentReading.AnalysisRunID, currentReading.SnapshotID)
}

func currentReadingPreparationMatches(preparation domain.DeckPreparation, owner string, currentReading domain.CurrentReading) bool {
	return (preparation.OwnerID == "" || preparation.OwnerID == owner) &&
		preparation.SourceMaterialID == currentReading.SourceMaterialID &&
		preparation.AnalysisRunID == currentReading.AnalysisRunID &&
		preparation.SnapshotID == currentReading.SnapshotID
}
