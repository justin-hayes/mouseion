package webapp

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func (h *Handler) bookCover(w http.ResponseWriter, r *http.Request) {
	bookID := strings.TrimSpace(r.PathValue("id"))
	if bookID == "" {
		http.NotFound(w, r)
		return
	}
	resource, err := h.services.Store.Shell.GetActiveBookCoverResource(r.Context(), user(r).ID, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	etag := `"` + strings.ReplaceAll(resource.ContentHash, `"`, "") + `"`
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", resource.MediaType)
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(w, r, "book-cover", time.Time{}, bytes.NewReader(resource.Bytes))
}

func (h *Handler) bookDetail(w http.ResponseWriter, r *http.Request, owner, id string) (domain.MyBook, bool) {
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return domain.MyBook{}, false
	}
	detail, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, id)
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return domain.MyBook{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.MyBook{}, false
	}
	return detail, true
}

func (h *Handler) analysisResult(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	if detail.Acquired == nil {
		http.NotFound(w, r)
		return
	}
	book := *detail.Acquired
	if !analysisReadyForReading(book) {
		http.NotFound(w, r)
		return
	}
	runID := r.PathValue("runID")
	if book.AnalysisRunID != runID {
		http.NotFound(w, r)
		return
	}
	if err := h.annotateBookToReadLanguage(r.Context(), u.ID, bookStudyLanguage(detail), &detail); err != nil {
		fail(w, err)
		return
	}
	if !detail.IsToRead {
		http.NotFound(w, r)
		return
	}
	target := readingBookURLInActiveLanguage(r.Context(), detail)
	if target == "" {
		redirectLanguageChanged(w, r)
		return
	}
	redirect(w, r, target)
}
