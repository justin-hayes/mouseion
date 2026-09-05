package webapp

import (
	"context"
	"errors"
	"log"
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
	render(w, r, ConnectionsPageForBook(u, h.csrf(w, r), c, r.URL.Query().Get("message"), r.URL.Query().Get("book_id"), statuses))
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
		redirect(w, r, "/connections?error="+url.QueryEscape("The catalogue sync could not be started. Try again."))
		return
	}
	redirect(w, r, "/connections?message="+url.QueryEscape("Catalogue sync submitted."))
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
	location := "/connections?message=Catalog+added"
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
	redirect(w, r, "/connections?message=Catalog+updated")
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
	redirect(w, r, "/connections?message=Catalog+deleted")
}
func (h *Handler) acquire(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	target, decodeErr := decodeAcquisitionTarget(h.targetKey, r.FormValue("acquisition"))
	if decodeErr != nil {
		h.acquisitionFailure(w, r, r.FormValue("connection"), "", errors.New("invalid acquisition entry"), h.acquisitionReturnPath(r.FormValue("return_to")), opds.Entry{}, "")
		return
	}
	language := target.Language
	connectionID := target.Connection
	if r.FormValue("connection") != "" && r.FormValue("connection") != connectionID || r.FormValue("language") != "" && r.FormValue("language") != language {
		h.acquisitionFailure(w, r, connectionID, language, errors.New("invalid acquisition entry: catalog context changed"), h.acquisitionReturnPath(r.FormValue("return_to")), target.Entry, target.Href)
		return
	}
	connection, e := h.services.Store.GetOpdsConnection(r.Context(), u.ID, connectionID)
	if e != nil {
		opdsFail(w, e)
		return
	}
	if target.Entry.ID == "" || strings.TrimSpace(target.Entry.Title) == "" {
		h.acquisitionFailure(w, r, connectionID, language, errors.New("invalid acquisition entry"), h.acquisitionReturnPath(r.FormValue("return_to")), target.Entry, target.Href)
		return
	}
	target.Entry.Links = []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: target.Href}}
	href := target.Href
	entry := target.Entry
	bookID := bookIDFromReturnPath(r.FormValue("return_to"))
	var result epub.ImportResult
	var acquireErr error
	if bookID != "" {
		if _, bookErr := h.services.Store.GetBook(r.Context(), u.ID, bookID); bookErr != nil {
			if errors.Is(bookErr, persistence.ErrNotFound) {
				redirect(w, r, "/library?error="+url.QueryEscape("That My Books entry is no longer available. Refresh My Books and choose an active entry."))
				return
			}
			fail(w, bookErr)
			return
		}
		acquirer, ok := h.services.OPDS.(interface {
			AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
		})
		if !ok {
			h.acquisitionFailure(w, r, connectionID, language, errors.New("the selected My Books entry cannot be promoted by this catalog service"), h.acquisitionReturnPath(r.FormValue("return_to")), entry, href)
			return
		}
		result, acquireErr = acquirer.AcquireForBook(r.Context(), u.ID, connectionID, language, bookID, entry)
	} else {
		result, acquireErr = h.services.OPDS.Acquire(r.Context(), u.ID, connectionID, language, entry)
	}
	e = acquireErr
	if acquireErr == nil {
		h.rememberAcquisition(w, r, connectionID, language, entry, href, result.Source.ID)
		if isHTMX(r) {
			render(w, r, AcquisitionSuccessCardWithReturn(entry, result.Source.ID, result.AlreadyPresent, h.acquisitionReturnPath(r.FormValue("return_to"))))
			return
		}
		returnTo := h.acquisitionReturnPath(r.FormValue("return_to"))
		message := "Added to My Books. Continue browsing or open the owned book; analysis starts separately."
		if result.AlreadyPresent {
			message = "That book is already in My Books. Continue browsing or open the existing book."
		}
		redirect(w, r, addQueryMessage(returnTo, message))
		return
	}
	if e != nil {
		message := opdsErrorMessage(e)
		clientEntry, clientHref := h.acquisitionEntryForClient(connectionID, language, entry, href)
		if isHTMX(r) && !errors.Is(e, persistence.ErrNotFound) {
			renderStatus(w, r, catalogFailureStatus(e), AcquisitionFailureCard(h.csrf(w, r), connection.ID, connection.Name, language, r.FormValue("return_to"), message, clientEntry, clientHref))
			return
		}
		if errors.Is(e, persistence.ErrNotFound) {
			opdsFail(w, e)
			return
		}
		renderStatus(w, r, catalogFailureStatus(e), AcquisitionFailurePage(user(r), h.csrf(w, r), connection, language, h.acquisitionReturnPath(r.FormValue("return_to")), message, clientEntry, clientHref))
		return
	}
}

func (h *Handler) acquisitionFailure(w http.ResponseWriter, r *http.Request, connectionID, language string, err error, retryURL string, entry opds.Entry, href string) {
	if errors.Is(err, persistence.ErrNotFound) {
		opdsFail(w, err)
		return
	}
	connection, connectionErr := h.services.Store.GetOpdsConnection(r.Context(), user(r).ID, connectionID)
	if connectionErr != nil {
		opdsFail(w, connectionErr)
		return
	}
	message := opdsErrorMessage(err)
	if isHTMX(r) {
		renderStatus(w, r, catalogFailureStatus(err), AcquisitionFailureCard(h.csrf(w, r), connection.ID, connection.Name, language, retryURL, message, entry, href))
		return
	}
	renderStatus(w, r, catalogFailureStatus(err), AcquisitionFailurePage(user(r), h.csrf(w, r), connection, language, retryURL, message, entry, href))
}

func catalogFailureStatus(err error) int {
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "discovery") || strings.Contains(lower, "temporarily unavailable"):
		return http.StatusServiceUnavailable
	case strings.Contains(lower, "unsupported"), strings.Contains(lower, "choose a language"), strings.Contains(lower, "does not advertise"), strings.Contains(lower, "invalid acquisition"), strings.Contains(lower, "invalid epub"), strings.Contains(lower, "no epub"), strings.Contains(lower, "incompatible media"):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

func opdsFail(w http.ResponseWriter, err error) {
	log.Printf("mouseion: OPDS: %s", opdsErrorMessage(err))
	if errors.Is(err, persistence.ErrNotFound) {
		http.Error(w, "catalog not found", http.StatusNotFound)
		return
	}
	http.Error(w, opdsErrorMessage(err), http.StatusBadGateway)
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
		message = "This acquisition request is invalid. No book was added; return to My Books and choose an active entry."
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
