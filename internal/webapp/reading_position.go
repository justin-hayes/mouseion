package webapp

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// readingPositionRefusalCopy maps each stable refusal reason to learner copy.
var readingPositionRefusalCopy = map[domain.PositionRefusalError]string{
	domain.ErrPositionMissing:    "Enter a percentage from 0 to 100 to record your Reading position.",
	domain.ErrPositionNotANumber: "Enter the percentage as a number from 0 to 100 with at most one decimal place, such as 35 or 35.5.",
	domain.ErrPositionOutOfRange: "A percentage must be from 0 to 100.",
}

// recordReadingPosition records the learner's percentage as a manual Position
// report for a Book in their Reading. A refusal returns to the Book detail with
// its reason and the entered percentage, so the form is refilled.
func (h *Handler) recordReadingPosition(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	book, ok := h.bookDetail(w, r, owner, r.PathValue("id"))
	if !ok {
		return
	}
	bookID := book.Book.ID
	input := r.FormValue("percent")
	fraction, err := domain.PositionFromPercentage(input)
	if refusal, ok := errors.AsType[domain.PositionRefusalError](err); ok {
		query := url.Values{"error": {readingPositionRefusalCopy[refusal]}, "percent": {input}}
		redirect(w, r, bookDetailURL(bookID)+"?"+query.Encode())
		return
	}
	err = h.services.Store.Positions.RecordManualPosition(r.Context(), owner, bookID, fraction, h.now())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		http.NotFound(w, r)
	case err != nil:
		fail(w, err)
	default:
		bookActionRedirect(w, r, bookID, "message", "Reading position recorded.")
	}
}
