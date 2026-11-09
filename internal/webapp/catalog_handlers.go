package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) connections(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	c, e := h.services.Store.Catalogs.ListOpdsConnections(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	items, statusErr := h.services.CatalogueSync.ListCatalogueSyncStatuses(r.Context(), u.ID)
	if statusErr != nil {
		fail(w, statusErr)
		return
	}
	statuses := make(map[string]domain.CatalogueSyncStatus, len(items))
	for _, status := range items {
		statuses[status.ConnectionID] = status
	}
	render(w, r, ConnectionsPageForBook(u, h.csrf(w, r), c, r.URL.Query().Get("message"), r.URL.Query().Get("error"), r.URL.Query().Get("book_id"), statuses, h.services.Classical))
}

// classicalCatalogView is the learner-specific state the release catalog pages
// need beside the manifest: ownership for "In My Books" and whether Add is
// available. Both reads are owner-scoped except the installation-wide legacy
// precondition.
type classicalCatalogView struct {
	AddAvailable         bool
	AddUnavailableReason string
	OwnedUnits           map[string]string
	OwnedWorks           map[string]string
}

// loadClassicalCatalogView reads ownership and the legacy-row precondition.
// The legacy check is installation-wide, so every catalog Add shares it.
func (h *Handler) loadClassicalCatalogView(ctx context.Context, owner string, catalog *classical.Catalog) (classicalCatalogView, error) {
	legacy, err := h.services.Store.Catalogs.ListLegacyLanguageRows(ctx, classical.Language)
	if err != nil {
		return classicalCatalogView{}, err
	}
	owned, err := h.services.Store.Catalogs.ListOwnedClassicalReadingUnits(ctx, owner)
	if err != nil {
		return classicalCatalogView{}, err
	}
	view := classicalCatalogView{
		AddAvailable: len(legacy) == 0,
		OwnedUnits:   make(map[string]string, len(owned)),
		OwnedWorks:   map[string]string{},
	}
	if !view.AddAvailable {
		view.AddUnavailableReason = classical.LegacyRowsReason
	}
	for _, unit := range owned {
		view.OwnedUnits[unit.ReadingUnitKey] = unit.BookID
	}
	for _, author := range catalog.Authors() {
		for _, work := range author.Works {
			for _, unit := range work.Units {
				if bookID := view.OwnedUnits[unit.Key]; bookID != "" {
					view.OwnedWorks[work.Key] = bookID
					break
				}
			}
		}
	}
	return view, nil
}

// classicalAuthorIndex renders the built-in Classical catalog's authors and
// their works. It is deliberately not scoped to the active study language.
func (h *Handler) classicalAuthorIndex(w http.ResponseWriter, r *http.Request) {
	view, err := h.loadClassicalCatalogView(r.Context(), user(r).ID, h.services.Classical)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, ClassicalAuthorIndexPage(user(r), h.csrf(w, r), h.services.Classical.Authors(), h.services.Classical.Counts(), view))
}

// classicalAuthor renders one author's works.
func (h *Handler) classicalAuthor(w http.ResponseWriter, r *http.Request) {
	author, ok := h.services.Classical.Author(r.PathValue("author"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	view, err := h.loadClassicalCatalogView(r.Context(), user(r).ID, h.services.Classical)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, ClassicalAuthorPage(user(r), h.csrf(w, r), author, view, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
}

// classicalWork renders one Work's constituent books.
func (h *Handler) classicalWork(w http.ResponseWriter, r *http.Request) {
	work, ok := h.services.Classical.Work(r.PathValue("workKey"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	view, err := h.loadClassicalCatalogView(r.Context(), user(r).ID, h.services.Classical)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, ClassicalWorkPage(user(r), h.csrf(w, r), work, view, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
}

// addClassicalWork adds one Book per current Reading unit of a Work. It is the
// primary action on a work; a whole-work unit reaches it from the author page
// with return=author so the learner stays there.
func (h *Handler) addClassicalWork(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	work, ok := h.services.Classical.Work(r.PathValue("workKey"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	keys := make([]string, 0, len(work.Units))
	for _, unit := range work.Units {
		keys = append(keys, unit.Key)
	}
	report, err := h.services.Store.Catalogs.AddClassicalReadingUnits(r.Context(), user(r).ID, keys)
	if err != nil {
		h.redirectClassicalAddFailure(w, r, classicalWorkURL(work.Key), err)
		return
	}
	target := classicalWorkURL(work.Key)
	if r.PostFormValue("return") == "author" {
		target = classicalAuthorURL(work.Author)
	}
	redirect(w, r, target+"?message="+url.QueryEscape(classicalAddMessage(report)))
}

// addClassicalUnit adds one constituent book or whole-work unit.
func (h *Handler) addClassicalUnit(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	key := r.PathValue("unitKey")
	work, _, ok := h.services.Classical.Unit(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	report, err := h.services.Store.Catalogs.AddClassicalReadingUnits(r.Context(), user(r).ID, []string{key})
	if err != nil {
		h.redirectClassicalAddFailure(w, r, classicalWorkURL(work.Key), err)
		return
	}
	redirect(w, r, classicalWorkURL(work.Key)+"?message="+url.QueryEscape(classicalAddMessage(report)))
}

// redirectClassicalAddFailure names the stable legacy reason instead of
// treating a refused Add as a server error. Any other failure is logged for the
// operator and the learner sees a recovery action, never the raw error.
func (h *Handler) redirectClassicalAddFailure(w http.ResponseWriter, r *http.Request, target string, err error) {
	if errors.Is(err, classical.ErrLegacyRows) {
		redirect(w, r, target+"?error="+url.QueryEscape(classical.LegacyRowsReason))
		return
	}
	log.Printf("classical catalog add: %v", err)
	redirect(w, r, target+"?error="+url.QueryEscape("This text could not be added. Refresh the catalog and try again."))
}

// classicalAddMessage states only what changed in My Books. It never mentions
// preparation, downloads or coverage.
func classicalAddMessage(report classical.AddReport) string {
	created := report.CreatedBooks()
	opened := len(report.Books) - created
	switch {
	case created == 0 && opened == 1:
		return "Opened the existing Book."
	case created == 0:
		return fmt.Sprintf("Opened %d existing Books.", opened)
	case opened == 0 && created == 1:
		return "Added 1 Book to My Books."
	case opened == 0:
		return fmt.Sprintf("Added %d Books to My Books.", created)
	default:
		return fmt.Sprintf("Added %d Books and opened %d existing Books.", created, opened)
	}
}

// classicalBookInMyBooks makes Ancient Greek the active study language and
// lands on the existing Book's row.
func (h *Handler) classicalBookInMyBooks(w http.ResponseWriter, r *http.Request) {
	h.openClassicalBookInMyBooks(w, r, r.PathValue("bookID"))
}

// classicalWorkInMyBooks lands on the first owned Book of a Work in book order.
func (h *Handler) classicalWorkInMyBooks(w http.ResponseWriter, r *http.Request) {
	work, ok := h.services.Classical.Work(r.PathValue("workKey"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	owned, err := h.services.Store.Catalogs.ListOwnedClassicalReadingUnits(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	bookID := ""
	for _, unit := range work.Units {
		for _, candidate := range owned {
			if candidate.ReadingUnitKey == unit.Key {
				bookID = candidate.BookID
				break
			}
		}
		if bookID != "" {
			break
		}
	}
	if bookID == "" {
		http.NotFound(w, r)
		return
	}
	h.openClassicalBookInMyBooks(w, r, bookID)
}

// openClassicalBookInMyBooks switches the active language and redirects to the
// row's visible bucket page. Only an owned classical Book can be opened.
func (h *Handler) openClassicalBookInMyBooks(w http.ResponseWriter, r *http.Request, bookID string) {
	owner := user(r).ID
	book, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	if book.ReadingUnit == nil {
		http.NotFound(w, r)
		return
	}
	if err := h.services.Store.Shell.SetActiveStudyLanguage(r.Context(), owner, classical.Language); err != nil {
		fail(w, err)
		return
	}
	page, err := myBooksPageForBook(r.Context(), h.services.Store.MyBooks, owner, classical.Language, book)
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, myBookVisibleBucketURL(book, page)+"#"+myBookRowID(book.Book.ID))
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
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	_, err := h.services.CatalogueSync.Enqueue(r.Context(), user(r).ID, r.PathValue("id"))
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
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	u := user(r)
	connectionURL, err := opds.NormalizeCatalogURL(r.FormValue("url"))
	if err != nil {
		http.Error(w, "catalog URL must use HTTP or HTTPS", http.StatusBadRequest)
		return
	}
	created, e := h.services.Store.Catalogs.CreateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{Name: strings.TrimSpace(r.FormValue("name")), URL: connectionURL, Username: r.FormValue("username"), Password: r.FormValue("password")})
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
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	u := user(r)
	current, e := h.services.Store.Catalogs.GetOpdsConnection(r.Context(), u.ID, r.PathValue("id"))
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
	_, e = h.services.Store.Catalogs.UpdateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{ID: current.ID, Name: strings.TrimSpace(r.FormValue("name")), URL: connectionURL, Username: r.FormValue("username"), Password: password})
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/catalogs?message=Catalog+updated")
}
func (h *Handler) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	u := user(r)
	if e := h.services.Store.Catalogs.DeleteOpdsConnection(r.Context(), u.ID, r.PathValue("id")); e != nil {
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
	case errors.Is(err, opds.ErrDownloadTooLarge):
		message = "This EPUB is larger than the 512 MiB download limit. Choose a smaller edition."
	case errors.Is(err, opds.ErrRetainedEPUBTooLarge):
		message = "This EPUB is larger than the 100 MiB limit for stored book content. Choose a less illustrated edition."
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
