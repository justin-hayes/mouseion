package webapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) journeyEntry(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("bookID"))
	if !ok {
		return
	}
	if detail.Acquired == nil {
		http.NotFound(w, r)
		return
	}

	if err := h.annotateBookWithJourneyLanguage(r.Context(), u.ID, journeyBookLanguage(detail), &detail); err != nil {
		fail(w, err)
		return
	}
	if !detail.JourneyMember || detail.Acquired.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(*detail.Acquired) {
		http.NotFound(w, r)
		return
	}
	redirect(w, r, journeyEntryOrLanguageHandoffURL(r.Context(), detail))
}

func (h *Handler) bookCover(w http.ResponseWriter, r *http.Request) {
	reader := h.services.Store.Covers
	bookID := strings.TrimSpace(r.PathValue("id"))
	if reader == nil || bookID == "" {
		http.NotFound(w, r)
		return
	}
	resource, err := reader.GetActiveBookCoverResource(r.Context(), user(r).ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
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
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner, id)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return domain.MyBook{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.MyBook{}, false
	}
	return detail, true
}

type catalogueAliasReader interface {
	GetBookCatalogEntryAlias(context.Context, string, string) (domain.BookAlias, error)
}

func (h *Handler) bookRefreshEligible(ctx context.Context, owner, bookID string) (bool, error) {
	reader, ok := h.services.Store.Catalog.(catalogueAliasReader)
	if !ok {
		return false, nil
	}
	alias, err := reader.GetBookCatalogEntryAlias(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if alias.AliasType != domain.AliasCatalogEntry || alias.Namespace != domain.NamespaceSourceIdentifier || strings.TrimSpace(alias.Value) == "" {
		return false, nil
	}
	if strings.TrimSpace(alias.ConnectionID) == "" {
		return false, nil
	}
	_, err = h.services.Store.Catalog.GetOpdsConnection(ctx, owner, alias.ConnectionID)
	if errors.Is(err, persistence.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (h *Handler) refreshableMyBookIDs(ctx context.Context, owner string, books []domain.MyBook) (map[string]bool, error) {
	refreshable := make(map[string]bool)
	for _, book := range books {
		eligible, err := h.bookRefreshEligible(ctx, owner, book.Book.ID)
		if err != nil {
			return nil, err
		}
		if eligible {
			refreshable[book.Book.ID] = true
		}
	}
	return refreshable, nil
}

func (h *Handler) refreshBookMetadata(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	refresher, ok := h.services.CatalogueSync.(CatalogueMetadataRefresher)
	if !ok {
		h.renderBookRefreshFailure(w, r, u, r.PathValue("id"))
		return
	}
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	result, err := refresher.RefreshEntry(r.Context(), u.ID, detail.Book.ID)
	if errors.Is(err, cataloguesync.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		result.Failed = true
	}
	if result.Book.ID == "" {
		result.Book = detail.Book
	}
	message := refreshMessage(result)
	if isHTMX(r) {
		row := domain.MyBook{Book: result.Book}
		if refreshed, readErr := h.services.Store.Books.GetBookDetail(r.Context(), u.ID, result.Book.ID); readErr == nil {
			row = refreshed
			row.Book = result.Book
		} else if !errors.Is(readErr, persistence.ErrNotFound) {
			fail(w, readErr)
			return
		}
		if err := h.annotateBookWithJourney(r.Context(), u.ID, &row); err != nil {
			fail(w, err)
			return
		}
		refreshEligible, err := h.bookRefreshEligible(r.Context(), u.ID, row.Book.ID)
		if err != nil {
			fail(w, err)
			return
		}
		render(w, r, MyBookRow(h.csrf(w, r), row, refreshEligible, message))
		return
	}
	redirect(w, r, "/library?message="+url.QueryEscape(message))
}

func (h *Handler) renderBookRefreshFailure(w http.ResponseWriter, r *http.Request, u domain.User, bookID string) {
	message := "Metadata could not be refreshed. Check the connection and try again."
	if isHTMX(r) {
		book, ok := h.bookDetail(w, r, u.ID, bookID)
		if !ok {
			return
		}
		if err := h.annotateBookWithJourney(r.Context(), u.ID, &book); err != nil {
			fail(w, err)
			return
		}
		render(w, r, MyBookRow(h.csrf(w, r), book, false, message))
		return
	}
	redirect(w, r, "/library?message="+url.QueryEscape(message))
}

func refreshMessage(result cataloguesync.RefreshResult) string {
	switch {
	case result.CoverPending:
		return "Metadata refreshed. Cover pending."
	case result.Updated:
		return "Metadata refreshed."
	case result.Missing:
		return "The catalog entry is no longer available. Your book and its metadata are unchanged."
	case result.Failed:
		return "Metadata could not be refreshed. Check the connection and try again."
	default:
		return "Metadata is already up to date."
	}
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
	if !bookHasCompletedAnalysis(book) {
		http.NotFound(w, r)
		return
	}
	runID := r.PathValue("runID")
	if book.AnalysisRunID != runID {
		http.NotFound(w, r)
		return
	}
	if err := h.annotateBookWithJourneyLanguage(r.Context(), u.ID, journeyBookLanguage(detail), &detail); err != nil {
		fail(w, err)
		return
	}
	if !detail.JourneyMember {
		http.NotFound(w, r)
		return
	}
	redirect(w, r, journeyEntryOrLanguageHandoffURL(r.Context(), detail))
}

func (h *Handler) acquireBookForJourneyContext(ctx context.Context, owner, bookID string) (cataloguesync.AcquisitionTarget, error) {
	provider, ok := h.services.CatalogueSync.(CatalogueAcquisitionTargetProvider)
	if !ok {
		return cataloguesync.AcquisitionTarget{}, errors.New("catalogue acquisition is unavailable")
	}
	target, err := provider.FindAcquisitionTarget(ctx, owner, bookID)
	if err != nil {
		return target, err
	}
	if h.services.OPDS == nil {
		return target, errors.New("catalogue acquisition cannot promote this book")
	}
	_, err = h.services.OPDS.AcquireForBook(ctx, owner, target.ConnectionID, target.Language, bookID, target.Entry)
	return target, err
}

func journeyAcquisitionError(ctx context.Context, catalog CatalogStore, owner, bookID, bookTitle string, target cataloguesync.AcquisitionTarget, err error) string {
	connectionName, entryTitle := "catalog connection", "this book"
	if strings.TrimSpace(target.Entry.Title) != "" {
		entryTitle = target.Entry.Title
	} else if strings.TrimSpace(bookTitle) != "" {
		entryTitle = bookTitle
	}
	if strings.TrimSpace(target.ConnectionID) != "" {
		connectionName = target.ConnectionID
	}
	if provider, ok := catalog.(catalogueAliasReader); ok {
		if alias, aliasErr := provider.GetBookCatalogEntryAlias(ctx, owner, bookID); aliasErr == nil {
			if connection, connectionErr := catalog.GetOpdsConnection(ctx, owner, alias.ConnectionID); connectionErr == nil {
				if strings.TrimSpace(connection.Name) != "" {
					connectionName = connection.Name
				}
			}
		}
	}
	return fmt.Sprintf("Could not acquire %q from connection %s: %s", entryTitle, connectionName, opdsErrorMessage(err))
}
