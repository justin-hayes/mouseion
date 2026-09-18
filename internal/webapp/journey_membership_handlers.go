package webapp

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) removeBookFromReadingJourney(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	expectedRevision, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("expected_revision")), 10, 64)
	if err != nil {
		redirect(w, r, "/journey?error="+url.QueryEscape(journeyStaleMessage))
		return
	}
	owner := user(r).ID
	bookID, ok, err := h.services.Store.Journey.ResolveJourneyBookID(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	language := strings.TrimSpace(detail.Book.LanguageTag)
	if language == "" {
		http.NotFound(w, r)
		return
	}
	if _, err = h.services.Store.Journey.RemoveFromReadingJourney(r.Context(), owner, language, bookID, expectedRevision); err != nil {
		if errors.Is(err, persistence.ErrJourneyStale) {
			redirect(w, r, "/journey?error="+url.QueryEscape(journeyStaleMessage))
			return
		}
		fail(w, err)
		return
	}
	title := bookID
	if book, bookErr := h.services.Store.Books.GetBook(r.Context(), owner, bookID); bookErr == nil && strings.TrimSpace(book.Title) != "" {
		title = book.Title
	}
	redirect(w, r, "/journey?message="+url.QueryEscape(title+" removed from Reading Journey. Analysis and acquired content were retained."))
}
