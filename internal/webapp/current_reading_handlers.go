package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
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
	// A Book-addressed action answers in its Book's language.
	if err := h.adoptBookLanguage(r.Context(), user(r).ID, focusBookID); err != nil {
		fail(w, err)
		return
	}
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
	location := readingPagePath(r.Context())
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

func (h *Handler) requireStudyLanguage(w http.ResponseWriter, r *http.Request) bool {
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		h.respondCurrentReading(w, r, "", currentReadingLanguageRequiredMessage, "")
		return false
	}
	return true
}
