package webapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
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

func readingAcquisitionError(ctx context.Context, catalog CatalogsStore, owner, bookID, bookTitle string, target cataloguesync.AcquisitionTarget, err error) string {
	connectionName, entryTitle := "catalog connection", "this book"
	if strings.TrimSpace(target.Entry.Title) != "" {
		entryTitle = target.Entry.Title
	} else if strings.TrimSpace(bookTitle) != "" {
		entryTitle = bookTitle
	}
	if strings.TrimSpace(target.ConnectionID) != "" {
		connectionName = target.ConnectionID
	}
	if alias, aliasErr := catalog.GetBookCatalogEntryAlias(ctx, owner, bookID); aliasErr == nil {
		if connection, connectionErr := catalog.GetOpdsConnection(ctx, owner, alias.ConnectionID); connectionErr == nil {
			if strings.TrimSpace(connection.Name) != "" {
				connectionName = connection.Name
			}
		}
	}
	return fmt.Sprintf("Could not acquire %q from connection %s: %s", entryTitle, connectionName, opdsErrorMessage(err))
}
