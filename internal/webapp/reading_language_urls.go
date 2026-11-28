package webapp

import (
	"context"
	"errors"
	"net/http"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// pageLanguagePath is a page kind in the request's page language.
func pageLanguagePath(ctx context.Context, kind studyLanguagePageKind) string {
	view := shellViewFromContext(ctx)
	if view == nil {
		return studyLanguagePath(kind, "")
	}
	return studyLanguagePath(kind, view.pageLanguage())
}

// myBooksPagePath is Reading in the request's page language.
func myBooksPagePath(ctx context.Context) string {
	return pageLanguagePath(ctx, pageKindReading)
}

// adoptBookLanguage makes a Book's study language the page language of a
// Book-addressed request (ADR 0131): the action runs in, and returns to, the
// Book's language whatever the Last study language is. A missing Book or one in
// a language the learner does not study leaves the page language unchanged.
func (h *Handler) adoptBookLanguage(ctx context.Context, owner, bookID string) error {
	view := shellViewFromContext(ctx)
	if view == nil || bookID == "" {
		return nil
	}
	detail, err := h.services.Store.Reading.GetBookDetail(ctx, owner, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	adoptDetailLanguage(ctx, detail)
	return nil
}

// adoptDetailLanguage is adoptBookLanguage for a Book the handler already loaded.
func adoptDetailLanguage(ctx context.Context, detail domain.MyBook) {
	view := shellViewFromContext(ctx)
	if view == nil {
		return
	}
	if language := canonicalization.NormalizeLanguage(bookStudyLanguage(detail)); view.hasLanguage(language) {
		view.PageLanguage = language
	}
}

// adoptBookLanguageOrFail adopts the Book's language, answering the request
// with a server error when the Book cannot be read.
func (h *Handler) adoptBookLanguageOrFail(w http.ResponseWriter, r *http.Request, owner, bookID string) bool {
	if err := h.adoptBookLanguage(r.Context(), owner, bookID); err != nil {
		fail(w, err)
		return false
	}
	return true
}
