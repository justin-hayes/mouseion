package webapp

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) setAsideReadingBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID, ok, err := h.resolveBookID(r.Context(), owner, r.PathValue("id"))
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
	dispositions, supported := h.services.Store.Books.(persistence.BookDispositionStore)
	if !supported {
		fail(w, errors.New("book dispositions are unavailable"))
		return
	}
	if err = dispositions.SetBookAside(r.Context(), owner, language, bookID); err != nil {
		if errors.Is(err, persistence.ErrBookIsPrimaryGoal) {
			redirect(w, r, "/reading?error="+url.QueryEscape("Current reading cannot be set aside here. Use its current-reading controls."))
			return
		}
		fail(w, err)
		return
	}
	title := bookID
	if book, bookErr := h.services.Store.Books.GetBook(r.Context(), owner, bookID); bookErr == nil && strings.TrimSpace(book.Title) != "" {
		title = book.Title
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(title+" moved to Set Aside. Analysis and acquired content were retained."))
}
