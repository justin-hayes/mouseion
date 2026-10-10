package webapp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// currentReadingSectionView is the server-truth fragment returned after an HTMX Current reading
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

// expectedCurrentSnapshotID is the Current reading snapshot the request names.
// An empty value names no snapshot, which admission refuses as stale.
func expectedCurrentSnapshotID(r *http.Request) string {
	return strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
}

// retryCurrentReadingDeck is the Reading-page entry for preparing or retrying the
// Book deck. The service admits or refuses the exact snapshot the request names.
func (h *Handler) retryCurrentReadingDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	if !h.requireStudyLanguage(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, bookID)
		return
	}
	_, err := h.services.PreparedDeck.PrepareCurrentReadingDeck(r.Context(), owner, bookID, expectedCurrentSnapshotID(r))
	switch {
	case err == nil:
		h.respondCurrentReading(w, r, currentReadingDeckRetryMessage, "", bookID)
	case errors.Is(err, domain.ErrDeckPreparationNotRequired):
		h.respondCurrentReading(w, r, "No deck is required for this empty Current reading snapshot.", "", bookID)
	case errors.Is(err, domain.ErrDeckPreparationStale), errors.Is(err, domain.ErrDeckPreparationNotCurrentReading):
		h.respondCurrentReading(w, r, "", currentReadingStaleMessage, bookID)
	default:
		log.Printf("current reading deck retry: %v", err)
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, bookID)
	}
}

// cancelCurrentReadingDeck cancels the live preparation of the Current reading.
// Cancellation checks only ownership and the preparation's active state, so the
// live preparation is resolved by ID rather than by the expected snapshot.
func (h *Handler) cancelCurrentReadingDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	if !h.requireStudyLanguage(w, r) {
		return
	}
	deck, err := h.services.PreparedDeck.CurrentReadingDeck(r.Context(), owner, bookID)
	if err != nil {
		log.Printf("current reading deck cancel lookup: %v", err)
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, bookID)
		return
	}
	if deck.Admission == domain.DeckAdmissionNotCurrentReading {
		h.respondCurrentReading(w, r, "", currentReadingStaleMessage, bookID)
		return
	}
	if deck.Preparation == nil {
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, bookID)
		return
	}
	if _, err = h.services.PreparedDeck.Cancel(r.Context(), owner, deck.Preparation.ID); err != nil {
		log.Printf("current reading deck cancel: %v", err)
		h.respondCurrentReading(w, r, "", currentReadingDeckUnavailableMessage, bookID)
		return
	}
	h.respondCurrentReading(w, r, currentReadingDeckCancelledMessage, "", bookID)
}

// requireStudyLanguage answers the request and reports false when no study
// language is active.
func (h *Handler) requireStudyLanguage(w http.ResponseWriter, r *http.Request) bool {
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		h.respondCurrentReading(w, r, "", currentReadingLanguageRequiredMessage, "")
		return false
	}
	return true
}
