package webapp

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

func (h *Handler) connections(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	c, e := h.services.Store.ListOpdsConnections(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, ConnectionsPage(u, h.csrf(w, r), c, r.URL.Query().Get("message")))
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
	_, e := h.services.Store.CreateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{Name: strings.TrimSpace(r.FormValue("name")), URL: connectionURL, Username: r.FormValue("username"), Password: r.FormValue("password")})
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/connections?message=Catalog+added")
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
	redirect(w, r, "/connections?message=Catalog+deleted")
}
func (h *Handler) catalog(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	c, e := h.services.Store.GetOpdsConnection(r.Context(), u.ID, r.URL.Query().Get("connection"))
	if e != nil {
		http.NotFound(w, r)
		return
	}
	languages, degraded := h.supportedNLP(r.Context())
	render(w, r, CatalogPage(u, h.csrf(w, r), c, languages, degraded))
}
func (h *Handler) browse(w http.ResponseWriter, r *http.Request) {
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	if _, err := h.supportedLanguage(r, language); err != nil {
		h.catalogFailure(w, r, r.URL.Query().Get("connection"), err, r.URL.RequestURI())
		return
	}
	u := user(r)
	connection, e := h.services.Store.GetOpdsConnection(r.Context(), u.ID, r.URL.Query().Get("connection"))
	if e != nil {
		h.catalogFailure(w, r, r.URL.Query().Get("connection"), e, "/connections")
		return
	}
	target, targetErr := h.decodeClientTarget(r.URL.Query().Get("url"), connection.ID, language)
	if targetErr != nil {
		h.catalogFailure(w, r, connection.ID, targetErr, r.URL.RequestURI())
		return
	}
	feed, e := h.services.OPDS.BrowsePage(r.Context(), u.ID, connection.ID, target)
	if e != nil {
		h.catalogFailure(w, r, connection.ID, e, r.URL.RequestURI())
		return
	}
	feed = h.prepareFeedForClient(connection.ID, language, feed)
	trail := decodeTrail(r.URL.Query()["trail"])
	currentURL := r.URL.Query().Get("url")
	owned := h.acquisitionState(r)
	if currentURL == "" {
		component := CatalogRootResults(h.csrf(w, r), connection.ID, language, h.acquisitionReturnPath(r.URL.RequestURI()), r.URL.Query().Get("message"), feed, owned)
		if isHTMX(r) {
			render(w, r, component)
		} else {
			render(w, r, CatalogRootPage(u, h.csrf(w, r), connection, language, r.URL.Query().Get("message"), feed, owned))
		}
		return
	}
	trail = append(trail, CatalogCrumb{Title: feed.Title, URL: currentURL})
	if isHTMX(r) {
		render(w, r, FeedFragmentWithState(h.csrf(w, r), connection.ID, language, h.acquisitionReturnPath(r.URL.RequestURI()), r.URL.Query().Get("message"), feed, trail, owned))
	} else {
		render(w, r, CatalogFeedPage(u, h.csrf(w, r), connection, feed, trail, language, h.acquisitionReturnPath(r.URL.RequestURI()), r.URL.Query().Get("message"), owned))
	}
}
func (h *Handler) browseLanguage(w http.ResponseWriter, r *http.Request) {
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	if language == "" {
		h.catalogFailure(w, r, r.URL.Query().Get("connection"), errors.New("Choose a language to browse its EPUB books."), "/catalog?connection="+url.QueryEscape(r.URL.Query().Get("connection")))
		return
	}
	capability, err := h.supportedLanguage(r, language)
	if err != nil {
		h.catalogFailure(w, r, r.URL.Query().Get("connection"), err, "/catalog?connection="+url.QueryEscape(r.URL.Query().Get("connection")))
		return
	}
	u := user(r)
	connectionID := r.URL.Query().Get("connection")
	connection, e := h.services.Store.GetOpdsConnection(r.Context(), u.ID, connectionID)
	if e != nil {
		h.catalogFailure(w, r, connectionID, e, "/connections")
		return
	}
	var feed opds.Feed
	if target := r.URL.Query().Get("url"); target != "" {
		decodedTarget, targetErr := h.decodeClientTarget(target, connectionID, language)
		if targetErr != nil {
			h.catalogFailure(w, r, connectionID, targetErr, r.URL.RequestURI())
			return
		}
		feed, e = h.services.OPDS.BrowsePage(r.Context(), u.ID, connectionID, decodedTarget)
		feed = opds.FilterEPUBEntries(feed)
	} else {
		languages, languageErr := h.services.OPDS.Languages(r.Context(), u.ID, connectionID)
		if languageErr != nil {
			h.catalogFailure(w, r, connectionID, languageErr, r.URL.RequestURI())
			return
		}
		languageID := catalogLanguageID(capability, languages)
		if languageID == "" {
			h.catalogFailure(w, r, connectionID, errors.New("The catalog does not advertise the selected ready language."), "/catalog?connection="+url.QueryEscape(connectionID))
			return
		}
		feed, e = h.services.OPDS.BrowseLanguagePage(r.Context(), u.ID, connectionID, languageID, r.URL.Query().Get("url"))
	}
	if e != nil {
		h.catalogFailure(w, r, connectionID, e, r.URL.RequestURI())
		return
	}
	feed = h.prepareFeedForClient(connectionID, language, feed)
	owned := h.acquisitionState(r)
	returnTo := h.acquisitionReturnPath(r.URL.RequestURI())
	if isHTMX(r) {
		render(w, r, LanguageResultsWithState(h.csrf(w, r), connectionID, language, returnTo, r.URL.Query().Get("message"), feed, owned))
	} else {
		render(w, r, CatalogLanguagePage(u, h.csrf(w, r), connection, language, returnTo, r.URL.Query().Get("message"), feed, owned))
	}
}
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	if _, err := h.supportedLanguage(r, language); err != nil {
		h.catalogFailure(w, r, r.URL.Query().Get("connection"), err, "/catalog?connection="+url.QueryEscape(r.URL.Query().Get("connection")))
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		h.catalogFailure(w, r, r.URL.Query().Get("connection"), errors.New("Enter a title or author to search this catalog."), r.URL.RequestURI())
		return
	}
	u := user(r)
	connectionID := r.URL.Query().Get("connection")
	connection, e := h.services.Store.GetOpdsConnection(r.Context(), u.ID, connectionID)
	if e != nil {
		h.catalogFailure(w, r, connectionID, e, "/connections")
		return
	}
	var feed opds.Feed
	if target := r.URL.Query().Get("url"); target != "" {
		decodedTarget, targetErr := h.decodeClientTarget(target, connectionID, language)
		if targetErr != nil {
			h.catalogFailure(w, r, connectionID, targetErr, r.URL.RequestURI())
			return
		}
		feed, e = h.services.OPDS.BrowsePage(r.Context(), u.ID, connectionID, decodedTarget)
	} else {
		feed, e = h.services.OPDS.SearchPage(r.Context(), u.ID, connectionID, query, "")
	}
	if e != nil {
		if errors.Is(e, opds.ErrSearchUnavailable) {
			h.catalogFailure(w, r, connectionID, errors.New("Search is not available for this catalog. Browse its collections instead."), "/catalog?connection="+url.QueryEscape(connectionID))
			return
		}
		h.catalogFailure(w, r, connectionID, e, r.URL.RequestURI())
		return
	}
	feed = h.prepareFeedForClient(connectionID, language, feed)
	queryValues := r.URL.Query()
	queryValues.Del("return_to")
	returnTo := h.acquisitionReturnPath("/opds/search?" + queryValues.Encode())
	backTo := searchBackPath(connectionID, language, r.URL.Query().Get("return_to"), decodeTrail(r.URL.Query()["trail"]))
	owned := h.acquisitionState(r)
	if isHTMX(r) {
		render(w, r, SearchResultsWithState(h.csrf(w, r), connectionID, language, returnTo, backTo, r.URL.Query().Get("message"), query, feed, owned))
	} else {
		render(w, r, CatalogSearchPage(u, h.csrf(w, r), connection, language, returnTo, backTo, query, r.URL.Query().Get("message"), feed, owned))
	}
}
func (h *Handler) acquire(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	target, decodeErr := decodeAcquisitionTarget(h.targetKey, r.FormValue("acquisition"))
	if decodeErr != nil {
		h.catalogFailure(w, r, r.FormValue("connection"), errors.New("invalid acquisition entry: return to the catalog and choose an EPUB entry"), h.acquisitionReturnPath(r.FormValue("return_to")))
		return
	}
	language := target.Language
	if _, err := h.supportedLanguage(r, language); err != nil {
		h.catalogFailure(w, r, target.Connection, err, h.acquisitionReturnPath(r.FormValue("return_to")))
		return
	}
	connectionID := target.Connection
	if r.FormValue("connection") != "" && r.FormValue("connection") != connectionID || r.FormValue("language") != "" && r.FormValue("language") != language {
		h.catalogFailure(w, r, connectionID, errors.New("invalid acquisition entry: catalog context changed"), h.acquisitionReturnPath(r.FormValue("return_to")))
		return
	}
	connection, e := h.services.Store.GetOpdsConnection(r.Context(), u.ID, connectionID)
	if e != nil {
		h.catalogFailure(w, r, connectionID, e, "/connections")
		return
	}
	if target.Entry.ID == "" || strings.TrimSpace(target.Entry.Title) == "" {
		h.catalogFailure(w, r, connectionID, errors.New("invalid acquisition entry"), h.acquisitionReturnPath(r.FormValue("return_to")))
		return
	}
	target.Entry.Links = []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: target.Href}}
	href := target.Href
	entry := target.Entry
	result, acquireErr := h.services.OPDS.Acquire(r.Context(), u.ID, connectionID, language, entry)
	e = acquireErr
	if acquireErr == nil {
		h.rememberAcquisition(w, r, connectionID, language, entry, href, result.Source.ID)
		if isHTMX(r) {
			render(w, r, AcquisitionSuccessCardWithReturn(entry, result.Source.ID, result.AlreadyPresent, h.acquisitionReturnPath(r.FormValue("return_to"))))
			return
		}
		returnTo := h.acquisitionReturnPath(r.FormValue("return_to"))
		message := "Added to My Library. Continue browsing or open the owned book; analysis starts separately."
		if result.AlreadyPresent {
			message = "That book is already in My Library. Continue browsing or open the existing book."
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
			h.catalogFailure(w, r, connection.ID, e, "/connections")
			return
		}
		renderStatus(w, r, catalogFailureStatus(e), AcquisitionFailurePage(user(r), h.csrf(w, r), connection, language, h.acquisitionReturnPath(r.FormValue("return_to")), message, clientEntry, clientHref))
		return
	}
}

func (h *Handler) catalogFailure(w http.ResponseWriter, r *http.Request, connectionID string, err error, retryURL string) {
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
	if retryURL == "" {
		retryURL = "/catalog?connection=" + url.QueryEscape(connection.ID)
	}
	retryURL = h.acquisitionReturnPath(retryURL)
	component := CatalogFailureFragment(connection, message, retryURL)
	status := catalogFailureStatus(err)
	if isHTMX(r) {
		renderStatus(w, r, status, component)
	} else {
		renderStatus(w, r, status, CatalogFailurePage(user(r), h.csrf(w, r), connection, message, retryURL))
	}
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
		message = "This acquisition request is invalid. No book was added; return to the catalog and choose an EPUB entry."
	case strings.Contains(lower, "fetch feed"), strings.Contains(lower, "download epub"):
		message = "The catalog could not be reached. Check its URL and network availability, then try again."
	}
	return message
}

type CatalogCrumb struct{ Title, URL string }

func catalogLanguageID(language domain.SupportedLanguage, feed opds.Feed) string {
	for _, entry := range feed.Entries {
		href := navigationLink(entry)
		parsed, err := url.Parse(href)
		if err != nil || href == "" {
			continue
		}
		id, err := url.PathUnescape(strings.TrimPrefix(path.Base(strings.TrimRight(parsed.Path, "/")), "/"))
		languageID, idErr := strconv.Atoi(id)
		if err != nil || idErr != nil || languageID < 1 {
			continue
		}
		name := strings.TrimSpace(entry.Title)
		if strings.EqualFold(name, language.Language) || strings.EqualFold(name, language.DisplayName) {
			return id
		}
	}
	return ""
}

func (h *Handler) supportedLanguage(r *http.Request, language string) (domain.SupportedLanguage, error) {
	supported, degraded := h.supportedNLP(r.Context())
	if degraded {
		return domain.SupportedLanguage{}, errors.New("NLP language discovery is temporarily unavailable")
	}
	for _, candidate := range supported {
		if candidate.Language == language {
			return candidate, nil
		}
	}
	return domain.SupportedLanguage{}, errors.New("unsupported analysis language")
}

func decodeTrail(values []string) []CatalogCrumb {
	trail := make([]CatalogCrumb, 0, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "\x1f", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			trail = append(trail, CatalogCrumb{Title: parts[0], URL: parts[1]})
		}
	}
	return trail
}
func browseURL(connectionID, target string, trail []CatalogCrumb) string {
	values := url.Values{"connection": {connectionID}}
	if target != "" {
		values.Set("url", target)
	}
	for _, crumb := range trail {
		values.Add("trail", crumb.Title+"\x1f"+crumb.URL)
	}
	return "/opds/browse?" + values.Encode()
}

func browseURLForLanguage(connectionID, language, target string, trail []CatalogCrumb) string {
	values := url.Values{"connection": {connectionID}}
	if language != "" {
		values.Set("language", language)
	}
	if target != "" {
		values.Set("url", target)
	}
	for _, crumb := range trail {
		values.Add("trail", crumb.Title+"\x1f"+crumb.URL)
	}
	return "/opds/browse?" + values.Encode()
}

func languagePageURL(connectionID, language, target string) string {
	values := url.Values{"connection": {connectionID}, "language": {language}, "url": {target}}
	return "/opds/language?" + values.Encode()
}

func searchPageURL(connectionID, language, query, backTo, target string) string {
	values := url.Values{"connection": {connectionID}, "language": {language}, "q": {query}, "return_to": {webauth.SafeReturnPath(backTo)}, "url": {target}}
	return "/opds/search?" + values.Encode()
}

func searchBackPath(connectionID, language, returnTo string, trail []CatalogCrumb) string {
	backTo := webauth.SafeReturnPath(returnTo)
	if strings.TrimSpace(returnTo) == "" || backTo == "/" {
		return browseURLForLanguage(connectionID, language, "", trail)
	}
	return backTo
}

func navigationLink(e opds.Entry) string {
	for _, l := range e.Links {
		if l.Rel == "subsection" || l.Rel == "alternate" || (l.Type == "application/atom+xml" && len(opds.FindEPUBs(e)) == 0) {
			return l.Href
		}
	}
	return ""
}
func acquisitionLink(e opds.Entry) *opds.Link {
	links := opds.FindEPUBs(e)
	if len(links) > 0 {
		return &links[0]
	}
	return nil
}

func feedLink(feed opds.Feed, rel string) string {
	for _, link := range feed.Links {
		for _, value := range strings.Fields(link.Rel) {
			if value == rel {
				return link.Href
			}
		}
	}
	return ""
}

func catalogFailureKind(message string) FeedbackKind {
	if strings.Contains(strings.ToLower(message), "degraded") || strings.Contains(strings.ToLower(message), "temporarily") {
		return FeedbackWarning
	}
	return FeedbackError
}

func catalogFailureTitle(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "credential"):
		return "Authentication failed"
	case strings.Contains(lower, "language"):
		return "Language selection needs attention"
	case strings.Contains(lower, "search"):
		return "Search unavailable"
	default:
		return "Catalog request failed"
	}
}
func credentialSummary(connection domain.OpdsConnection) string {
	if connection.Username != "" {
		return "Credentials saved securely for " + connection.Username + "."
	}
	return "No credentials configured."
}
