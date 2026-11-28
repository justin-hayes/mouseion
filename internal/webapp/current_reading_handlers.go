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

func (h *Handler) respondCurrentReading(w http.ResponseWriter, r *http.Request, pageError string) {
	if isPartialHTMXRequest(r) {
		section, err := h.currentReadingSection(r.Context(), user(r).ID, "", pageError)
		if err != nil {
			fail(w, err)
			return
		}
		render(w, r, CurrentReadingSection(section.CurrentReading, h.csrf(w, r), section.Message, section.Error, ""))
		return
	}
	query := url.Values{}
	if pageError != "" {
		query.Set("error", pageError)
	}
	location := myBooksPagePath(r.Context())
	if encoded := query.Encode(); encoded != "" {
		location += "?" + encoded
	}
	redirect(w, r, location)
}
