package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) connections(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	c, e := h.services.Store.ListOpdsConnections(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	statuses := make(map[string]domain.CatalogueSyncStatus)
	if reader, ok := h.services.CatalogueSync.(interface {
		ListCatalogueSyncStatuses(context.Context, string) ([]domain.CatalogueSyncStatus, error)
	}); ok {
		items, statusErr := reader.ListCatalogueSyncStatuses(r.Context(), u.ID)
		if statusErr != nil {
			fail(w, statusErr)
			return
		}
		for _, status := range items {
			statuses[status.ConnectionID] = status
		}
	} else if reader, ok := h.services.Store.(interface {
		ListCatalogueSyncStatuses(context.Context, string) ([]domain.CatalogueSyncStatus, error)
	}); ok {
		items, statusErr := reader.ListCatalogueSyncStatuses(r.Context(), u.ID)
		if statusErr != nil {
			fail(w, statusErr)
			return
		}
		for _, status := range items {
			statuses[status.ConnectionID] = status
		}
	}
	render(w, r, ConnectionsPageForBook(u, h.csrf(w, r), c, r.URL.Query().Get("message"), r.URL.Query().Get("error"), r.URL.Query().Get("book_id"), statuses))
}

func (h *Handler) legacyConnections(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	for _, key := range []string{"book_id", "message", "error"} {
		if values, ok := r.URL.Query()[key]; ok {
			query[key] = append([]string(nil), values...)
		}
	}
	location := "/catalogs"
	if encoded := query.Encode(); encoded != "" {
		location += "?" + encoded
	}
	http.Redirect(w, r, location, http.StatusMovedPermanently)
}

func (h *Handler) syncConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	service, ok := h.services.CatalogueSync.(interface {
		Enqueue(context.Context, string, string) (cataloguesync.Handle, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, err := service.Enqueue(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, cataloguesync.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		redirect(w, r, "/catalogs?error="+url.QueryEscape("The catalog sync could not be started. Try again."))
		return
	}
		redirect(w, r, "/catalogs?message="+url.QueryEscape("Catalog sync submitted."))
}
func (h *Handler) createConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	connectionURL, err := opds.NormalizeCatalogURL(r.FormValue("url"))
	if err != nil {
		http.Error(w, "catalog URL must use HTTP or HTTPS", http.StatusBadRequest)
		return
	}
	created, e := h.services.Store.CreateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{Name: strings.TrimSpace(r.FormValue("name")), URL: connectionURL, Username: r.FormValue("username"), Password: r.FormValue("password")})
	if e != nil {
		fail(w, e)
		return
	}
	if h.services.CatalogueSync != nil {
		if e = h.services.CatalogueSync.RegisterConnection(r.Context(), u.ID, created.ID); e != nil {
			fail(w, e)
			return
		}
	}
	location := "/catalogs?message=Catalog+added"
	if bookID := strings.TrimSpace(r.FormValue("book_id")); bookID != "" {
		location += "&book_id=" + url.QueryEscape(bookID)
	}
	redirect(w, r, location)
}
func (h *Handler) updateConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	current, e := h.services.Store.GetOpdsConnection(r.Context(), u.ID, r.PathValue("id"))
	if e != nil {
		http.NotFound(w, r)
		return
	}
	password := r.FormValue("password")
	if password == "" {
		password = current.Password
	}
	connectionURL := strings.TrimSpace(r.FormValue("url"))
	if connectionURL == "" {
		connectionURL = current.URL
	}
	connectionURL, e = opds.NormalizeCatalogURL(connectionURL)
	if e != nil {
		http.Error(w, "catalog URL must use HTTP or HTTPS", http.StatusBadRequest)
		return
	}
	_, e = h.services.Store.UpdateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{ID: current.ID, Name: strings.TrimSpace(r.FormValue("name")), URL: connectionURL, Username: r.FormValue("username"), Password: password})
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/catalogs?message=Catalog+updated")
}
func (h *Handler) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	if e := h.services.Store.DeleteOpdsConnection(r.Context(), u.ID, r.PathValue("id")); e != nil {
		if errors.Is(e, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, e)
		return
	}
	if h.services.CatalogueSync != nil {
		if e := h.services.CatalogueSync.UnregisterConnection(u.ID, r.PathValue("id")); e != nil {
			fail(w, e)
			return
		}
	}
	redirect(w, r, "/catalogs?message=Catalog+deleted")
}
func opdsErrorMessage(err error) string {
	message := "The catalog request failed. Check the connection and try again."
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, opds.ErrNoEPUB), strings.Contains(lower, "incompatible media type"):
		message = "This book is not available as an EPUB. Choose another edition or format."
	case strings.Contains(lower, "401"), strings.Contains(lower, "403"):
		message = "The catalog rejected the credentials. Update the connection username and password."
	case strings.Contains(lower, "parse atom"), strings.Contains(lower, "html"):
		message = "The catalog returned a web page instead of an OPDS feed. Check the catalog URL."
	case errors.Is(err, epub.ErrInvalidEPUB), strings.Contains(lower, "invalid epub"):
		message = "The catalog item was not a valid EPUB. No book was added; choose another item or try again."
	case strings.Contains(lower, "ingest downloaded epub"), strings.Contains(lower, "validate epub"):
		message = "The downloaded EPUB could not be added. Choose another book or try again."
	case strings.Contains(lower, "invalid acquisition"):
		message = "The catalog could not acquire this book. Check the connection and try again."
	case strings.Contains(lower, "fetch feed"), strings.Contains(lower, "download epub"):
		message = "The catalog could not be reached. Check its URL and network availability, then try again."
	}
	return message
}

func credentialSummary(connection domain.OpdsConnection) string {
	if connection.Username != "" {
		return "Credentials saved securely for " + connection.Username + "."
	}
	return "No credentials configured."
}
