package webapp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type currentReadingFinishView struct {
	BookTitle string
}

// finishCurrentReading appends a Reading completion for the language's current
// reading and clears the role. It has no vocabulary effect and takes no
// confirmation token (ADR 0136 §4).
func (h *Handler) finishCurrentReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	language := pageLanguage(r)
	if language == "" {
		h.respondCurrentReading(w, r, currentReadingLanguageRequiredMessage)
		return
	}
	result, err := h.services.Store.Reading.FinishCurrentReading(r.Context(), owner, language)
	if errors.Is(err, domain.ErrNotFound) {
		h.respondCurrentReading(w, r, noCurrentReadingMessage)
		return
	}
	if err != nil {
		h.respondCurrentReading(w, r, "Reading could not be marked finished. No changes were made; review My Books and try again.")
		return
	}
	outcome := currentReadingFinishView{BookTitle: h.finishBookTitle(r.Context(), owner, result.Completion.BookID)}
	render(w, r, CurrentReadingFinishPage(user(r), h.csrf(w, r), outcome))
}

func (h *Handler) finishBookTitle(ctx context.Context, owner, bookID string) string {
	book, err := h.services.Store.Reading.GetBook(ctx, owner, bookID)
	if err == nil && strings.TrimSpace(book.Title) != "" {
		return book.Title
	}
	return bookID
}

// currentReadingCompletionConfirmationText says what Finish does: it records
// the reading and has no vocabulary effect (ADR 0136 §4).
const currentReadingCompletionConfirmationText = "Record this Book as read and clear your current reading. This changes no vocabulary."
