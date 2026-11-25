package webapp

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// catalogEntryCover serves one retained Catalog entry image to its owner. The
// entry identity is a query value because OPDS identifiers can contain "/" and
// ":". Missing, unavailable, and cross-owner references are all not found, and
// the validator is evaluated only after the owner-scoped read.
func (h *Handler) catalogEntryCover(w http.ResponseWriter, r *http.Request) {
	connectionID := strings.TrimSpace(r.PathValue("id"))
	entry := strings.TrimSpace(r.URL.Query().Get("entry"))
	if connectionID == "" || entry == "" {
		http.NotFound(w, r)
		return
	}
	resource, err := h.services.Store.CatalogCovers.GetCatalogEntryCoverResource(r.Context(), domain.CatalogEntryKey{OwnerID: user(r).ID, ConnectionID: connectionID, EntryIdentifier: entry})
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	serveCoverImage(w, r, "catalog-entry-cover", resource.MediaType, resource.ContentHash, resource.Bytes)
}

// serveCoverImage writes a retained cover after the owner-scoped read: a
// private revalidated response with a content-hash validator.
func serveCoverImage(w http.ResponseWriter, r *http.Request, name, mediaType, contentHash string, image []byte) {
	etag := `"` + strings.ReplaceAll(contentHash, `"`, "") + `"`
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", mediaType)
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(image))
}

// catalogEntryCoverURL is the owner-scoped local image address for one entry.
func catalogEntryCoverURL(connectionID, entryIdentifier string) string {
	return "/catalogs/sources/" + url.PathEscape(connectionID) + "/cover?entry=" + url.QueryEscape(entryIdentifier)
}
