package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/readingintent"
)

// connections renders the Catalog. The route name is historical; the
// destination, its Classical record, its OPDS sections, and its source status
// lines are not.
func (h *Handler) connections(w http.ResponseWriter, r *http.Request) {
	u, connections, statuses, err := h.catalogPageState(r)
	if err != nil {
		fail(w, err)
		return
	}
	classicalView, err := h.catalogClassicalView(r, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	browse, err := h.loadCatalogBrowse(r, u.ID, connections, classicalView)
	if err != nil {
		fail(w, err)
		return
	}
	h.loadCatalogAddConfirmation(r, u.ID, &browse)
	h.loadCatalogReadConfirmation(r, u.ID, &browse)
	render(w, r, CatalogPage(u, h.csrf(w, r), connections, r.URL.Query().Get("message"), r.URL.Query().Get("error"), statuses, browse))
}

// addCatalogueEntry is Add on one OPDS Catalog entry. It creates or restores the
// Book as To Read, then routes it through the same reading-intent preparation
// seam Move to To Read uses. It is an ordinary owner-scoped POST.
func (h *Handler) addCatalogueEntry(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	owner := user(r).ID
	connectionID := r.PathValue("id")
	entryIdentifier := strings.TrimSpace(r.PostFormValue("entry_identifier"))
	added, err := h.services.Store.CatalogueAdd.AddCatalogueEntry(r.Context(), owner, connectionID, entryIdentifier)
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	message := catalogueAddMessage(added)
	if added.Changed {
		message += h.prepareAddedCatalogueBook(r.Context(), owner, added.Book)
	}
	query := url.Values{
		"message":          {message},
		"added_book":       {added.Book.ID},
		"added_connection": {connectionID},
		"added_entry":      {entryIdentifier},
	}
	redirect(w, r, "/catalogs?"+query.Encode())
}

// markCatalogueEntryPreviouslyRead is "I've read this" on one OPDS Catalog
// entry. It creates or restores the Book as Read with a Previously read record;
// it acquires nothing, starts no job and changes no vocabulary.
func (h *Handler) markCatalogueEntryPreviouslyRead(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	owner := user(r).ID
	connectionID := r.PathValue("id")
	entryIdentifier := strings.TrimSpace(r.PostFormValue("entry_identifier"))
	read, err := h.services.Store.CatalogueRead.MarkCatalogueEntryPreviouslyRead(r.Context(), owner, connectionID, entryIdentifier)
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, domain.ErrBookIsCurrentReading) {
		redirect(w, r, "/catalogs?error="+url.QueryEscape(catalogueReadCurrentReadingRefusal))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	query := url.Values{
		"message":         {catalogueReadMessage(read)},
		"read_book":       {read.Book.ID},
		"read_connection": {connectionID},
		"read_entry":      {entryIdentifier},
	}
	redirect(w, r, "/catalogs?"+query.Encode())
}

// catalogueReadCurrentReadingRefusal is the recovery sentence for a direct
// assertion on the Book the learner is currently reading.
const catalogueReadCurrentReadingRefusal = "End this Book's current reading before marking it previously read."

// catalogueReadMessage states the history change and names the Book.
func catalogueReadMessage(read domain.CatalogueReadResult) string {
	title := strings.TrimSpace(read.Book.Title)
	if title == "" {
		title = "the text"
	}
	switch {
	case read.Created:
		return fmt.Sprintf("Recorded “%s” as previously read.", title)
	case read.Changed:
		return fmt.Sprintf("Marked “%s” as previously read.", title)
	default:
		return fmt.Sprintf("“%s” is already in Read.", title)
	}
}

// catalogueAddCouldNotStart is the recovery sentence for an Add whose
// preparation never reached a job. The Add itself already succeeded.
const catalogueAddCouldNotStart = " The Book is To Read, but analysis could not start. Retry analysis from Reading when ready."

// prepareAddedCatalogueBook sends an added Book through reading intent and
// states what that did. A submission failure never undoes the Add.
func (h *Handler) prepareAddedCatalogueBook(ctx context.Context, owner string, book domain.Book) string {
	intent, err := h.readingIntentModule()
	if err != nil {
		log.Printf("catalog add: reading intent unavailable: %v", err)
		return catalogueAddCouldNotStart
	}
	outcome, err := intent.Retry(ctx, owner, book.ID)
	if err != nil {
		log.Printf("catalog add: prepare book %q: %v", book.ID, err)
		return catalogueAddCouldNotStart
	}
	return catalogueAddPreparationMessage(ctx, h.services.Store.Catalogs, owner, book, outcome)
}

// catalogueAddMessage states the membership change and names the Book.
func catalogueAddMessage(added domain.CatalogueAddResult) string {
	title := strings.TrimSpace(added.Book.Title)
	if title == "" {
		title = "the text"
	}
	switch {
	case added.Created:
		return fmt.Sprintf("Added “%s” to My Books.", title)
	case added.Changed:
		return fmt.Sprintf("Restored “%s” to My Books.", title)
	default:
		return fmt.Sprintf("“%s” is already in My Books.", title)
	}
}

// catalogueAddPreparationMessage maps the reading-intent outcome to what
// happens to the text after Add. It never promises work that is already done or
// that could not start, and a failed acquisition keeps the To Read choice.
func catalogueAddPreparationMessage(ctx context.Context, catalog CatalogsStore, owner string, book domain.Book, outcome readingintent.Outcome) string {
	switch outcome.Kind {
	case readingintent.OutcomeJobQueued:
		return " Downloading and analyzing the text."
	case readingintent.OutcomeAlreadyCurrent, readingintent.OutcomeCurrentBookRefused:
		return " The text is already analyzed."
	case readingintent.OutcomeAcquisitionFailed:
		return " " + strings.TrimSuffix(readingAcquisitionError(ctx, catalog, owner, book.ID, book.Title, outcome.Target, outcome.Err), ".") + ". The Book stays To Read; retry acquisition from My Books when ready."
	case readingintent.OutcomeNoUsableEPUBUnits:
		return " The current source has no usable EPUB units. The Book stays To Read."
	case readingintent.OutcomeCouldNotStart:
		return catalogueAddCouldNotStart
	default:
		return ""
	}
}

// loadCatalogAddConfirmation renders the Add link only for the Book whose
// catalog-entry alias the Add named, so the page never carries an arbitrary
// link target. Focus follows the confirmation after the POST redirect.
func (h *Handler) loadCatalogAddConfirmation(r *http.Request, owner string, browse *catalogBrowseView) {
	bookID := h.catalogConfirmationIdentity(r, "added_book", "added_connection", "added_entry", owner)
	if bookID == "" {
		return
	}
	browse.AddedBookID = bookID
	browse.FocusAddedBook = true
}

// catalogConfirmationIdentity returns the Book a Catalog action named only when
// its owner-scoped catalog-entry alias still matches the posted connection and
// entry, so a page query never carries an arbitrary link target.
func (h *Handler) catalogConfirmationIdentity(r *http.Request, bookKey, connectionKey, entryKey, owner string) string {
	bookID := strings.TrimSpace(r.URL.Query().Get(bookKey))
	connectionID := strings.TrimSpace(r.URL.Query().Get(connectionKey))
	entryIdentifier := strings.TrimSpace(r.URL.Query().Get(entryKey))
	if bookID == "" || connectionID == "" || entryIdentifier == "" {
		return ""
	}
	alias, err := h.services.Store.Catalogs.GetBookCatalogEntryAlias(r.Context(), owner, bookID)
	if err != nil || alias.Namespace != domain.NamespaceSourceIdentifier || alias.AliasType != domain.AliasCatalogEntry || alias.ConnectionID != connectionID || alias.Value != entryIdentifier {
		return ""
	}
	return bookID
}

// loadCatalogReadConfirmation renders the Read link only for the Book whose
// catalog-entry alias the assertion named, so the page never carries an
// arbitrary link target. Focus follows the confirmation after the POST
// redirect.
func (h *Handler) loadCatalogReadConfirmation(r *http.Request, owner string, browse *catalogBrowseView) {
	bookID := h.catalogConfirmationIdentity(r, "read_book", "read_connection", "read_entry", owner)
	if bookID == "" {
		return
	}
	browse.ReadBookID = bookID
	browse.FocusReadBook = true
}

// catalogClassicalView loads ownership and the legacy-row precondition for the
// Classical catalog when the release carries any Works.
func (h *Handler) catalogClassicalView(r *http.Request, owner string) (classicalCatalogView, error) {
	if h.services.Classical == nil || h.services.Classical.Counts().Works == 0 {
		return classicalCatalogView{}, nil
	}
	return h.loadClassicalCatalogView(r.Context(), owner, h.services.Classical)
}

// manageSources configures the OPDS connections that feed the Catalog.
func (h *Handler) manageSources(w http.ResponseWriter, r *http.Request) {
	u, connections, statuses, err := h.catalogPageState(r)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, ManageSourcesPage(u, h.csrf(w, r), connections, r.URL.Query().Get("message"), r.URL.Query().Get("error"), r.URL.Query().Get("book_id"), statuses))
}

func (h *Handler) catalogPageState(r *http.Request) (domain.User, []domain.OpdsConnection, map[string]domain.CatalogueSyncStatus, error) {
	u := user(r)
	connections, err := h.services.Store.Catalogs.ListOpdsConnections(r.Context(), u.ID)
	if err != nil {
		return u, nil, nil, err
	}
	items, err := h.services.CatalogueSync.ListCatalogueSyncStatuses(r.Context(), u.ID)
	if err != nil {
		return u, nil, nil, err
	}
	statuses := make(map[string]domain.CatalogueSyncStatus, len(items))
	for _, status := range items {
		statuses[status.ConnectionID] = status
	}
	return u, connections, statuses, nil
}

// redirectManageSources returns to the Manage sources page with one outcome.
func redirectManageSources(w http.ResponseWriter, r *http.Request, key, message, bookID string) {
	query := url.Values{}
	if message != "" {
		query.Set(key, message)
	}
	if bookID != "" {
		query.Set("book_id", bookID)
	}
	location := "/catalogs/sources"
	if encoded := query.Encode(); encoded != "" {
		location += "?" + encoded
	}
	redirect(w, r, location)
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

// classicalAuthorIndex renders the built-in Classical catalog index: every
// author's Works with their Openings, lengths, facts and Add controls. It is
// deliberately not scoped to the active study language.
func (h *Handler) classicalAuthorIndex(w http.ResponseWriter, r *http.Request) {
	view, err := h.loadClassicalCatalogView(r.Context(), user(r).ID, h.services.Classical)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, ClassicalAuthorIndexPage(user(r), h.csrf(w, r), h.services.Classical.Authors(), h.services.Classical.Counts(), h.services.Classical.SourceAttributions(), view, h.loadClassicalAddConfirmation(r), r.URL.Query().Get("error")))
}

// classicalAuthor retires the author-detail page. The index now lists every
// Work under its author, so an old author URL permanently redirects to that
// author's section of the index while an unknown author stays not found.
func (h *Handler) classicalAuthor(w http.ResponseWriter, r *http.Request) {
	author, ok := h.services.Classical.Author(r.PathValue("author"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, classicalAuthorIndexURL(author.Name), http.StatusMovedPermanently)
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
	render(w, r, ClassicalWorkPage(user(r), h.csrf(w, r), work, view, h.loadClassicalAddConfirmation(r), r.URL.Query().Get("error")))
}

// addClassicalWork adds one Book per current Reading unit of a Work. It is the
// primary action on a work; the index posts it with return=index so the learner
// stays there.
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
	owner := user(r).ID
	report, err := h.services.Store.Catalogs.AddClassicalReadingUnits(r.Context(), owner, keys)
	target := classicalWorkURL(work.Key)
	if r.PostFormValue("return") == classicalAddReturnIndex {
		target = classicalCatalogURL()
	}
	if err != nil {
		h.redirectClassicalAddFailure(w, r, target, err)
		return
	}
	h.completeClassicalAdd(w, r, target, report, work.Key)
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
	owner := user(r).ID
	report, err := h.services.Store.Catalogs.AddClassicalReadingUnits(r.Context(), owner, []string{key})
	target := classicalWorkURL(work.Key)
	if r.PostFormValue("return") == classicalAddReturnIndex {
		target = classicalCatalogURL()
	}
	if err != nil {
		h.redirectClassicalAddFailure(w, r, target, err)
		return
	}
	h.completeClassicalAdd(w, r, target, report, work.Key)
}

// markClassicalUnitPreviouslyRead records one current Reading unit as read
// outside Mouseion. It creates or restores the Book as Read with a Previously
// read record and queues no preparation.
func (h *Handler) markClassicalUnitPreviouslyRead(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	key := r.PathValue("unitKey")
	work, _, ok := h.services.Classical.Unit(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	target := classicalWorkURL(work.Key)
	if r.PostFormValue("return") == classicalAddReturnIndex {
		target = classicalCatalogURL()
	}
	read, err := h.services.Store.CatalogueRead.MarkClassicalReadingUnitPreviouslyRead(r.Context(), user(r).ID, key)
	if errors.Is(err, domain.ErrBookIsCurrentReading) {
		redirect(w, r, target+"?error="+url.QueryEscape(catalogueReadCurrentReadingRefusal))
		return
	}
	if errors.Is(err, classical.ErrLegacyRows) {
		log.Printf("mouseion: classical catalog previously read refused code=%s", classical.LegacyRowsCode)
		redirect(w, r, target+"?error="+url.QueryEscape(classical.LegacyRowsReason))
		return
	}
	if err != nil {
		log.Printf("classical catalog previously read: %v", err)
		redirect(w, r, target+"?error="+url.QueryEscape("This text could not be recorded as read. Refresh the catalog and try again."))
		return
	}
	query := url.Values{
		"message":   {catalogueReadMessage(read)},
		"read_book": {read.Book.ID},
	}
	redirect(w, r, target+"?"+query.Encode())
}

// completeClassicalAdd submits the preparation the Add expresses, then returns
// to the catalog with one confirmation. Both Add routes end here.
func (h *Handler) completeClassicalAdd(w http.ResponseWriter, r *http.Request, target string, report classical.AddReport, workKey string) {
	preparation := h.prepareAddedClassicalBooks(r.Context(), user(r).ID, report)
	h.redirectClassicalAdd(w, r, target, classicalAddConfirmationFor(report, preparation, activeLanguageOf(r), workKey))
}

// classicalAddPreparation is the preparation outcome of one Add, aggregated
// over its Books. The store has already made every Book To Read; this only
// reports what the reading-intent route did with them.
type classicalAddPreparation struct {
	Queued          int
	AlreadyPrepared int
	CouldNotStart   int
	Refusal         string
}

// prepareAddedClassicalBooks sends each added Book through the reading-intent
// route. Books are visited in constituent-book order, so a whole-Work Add
// enqueues its preparation jobs in that order. Preparation is ensure-once, so
// re-adding a prepared Book queues nothing. A submission failure never undoes
// the Add; it is reported in the confirmation.
func (h *Handler) prepareAddedClassicalBooks(ctx context.Context, owner string, report classical.AddReport) classicalAddPreparation {
	var preparation classicalAddPreparation
	if len(report.Books) == 0 {
		return preparation
	}
	intent, err := h.readingIntentModule()
	if err != nil {
		log.Printf("classical catalog add: reading intent unavailable: %v", err)
		preparation.CouldNotStart = len(report.Books)
		return preparation
	}
	for _, book := range report.Books {
		outcome, err := intent.Retry(ctx, owner, book.BookID)
		if err != nil {
			log.Printf("classical catalog add: prepare book %q: %v", book.BookID, err)
			preparation.CouldNotStart++
			continue
		}
		prepared := classicalAddPreparationFor(outcome)
		preparation.Queued += prepared.Queued
		preparation.AlreadyPrepared += prepared.AlreadyPrepared
		preparation.CouldNotStart += prepared.CouldNotStart
		if prepared.Refusal != "" {
			preparation.Refusal = prepared.Refusal
		}
	}
	return preparation
}

// classicalAddPreparationFor classifies one reading-intent outcome for the Add
// confirmation. A definition that can no longer be prepared keeps its specific
// stable refusal; every other non-start is the generic recovery sentence.
func classicalAddPreparationFor(outcome readingintent.Outcome) classicalAddPreparation {
	switch outcome.Kind {
	case readingintent.OutcomeJobQueued:
		return classicalAddPreparation{Queued: 1}
	case readingintent.OutcomeAlreadyCurrent, readingintent.OutcomeCurrentBookRefused:
		return classicalAddPreparation{AlreadyPrepared: 1}
	case readingintent.OutcomeWithdrawnDefinition:
		return classicalAddPreparation{CouldNotStart: 1, Refusal: classicalWithdrawnRefusal}
	case readingintent.OutcomeSourceRefused:
		return classicalAddPreparation{CouldNotStart: 1, Refusal: classicalSourceMismatchRefusal}
	default:
		return classicalAddPreparation{CouldNotStart: 1}
	}
}

const (
	// classicalAddReturnIndex returns the learner to the catalog index.
	classicalAddReturnIndex = "index"
)

// classicalAddConfirmation is what one Add reports to the page it returns to.
// WorkKey names the added Books' work only while another study language is
// active, so the page can offer the "In My Books" path that opens them in
// Ancient Greek.
type classicalAddConfirmation struct {
	Message string
	WorkKey string
	BookID  string
}

// InMyBooksURL returns the existing "In My Books" path for the added or
// recorded Books. A read assertion names its Book directly; an Add names the
// Work only while another study language is active.
func (c classicalAddConfirmation) InMyBooksURL() string {
	if c.BookID != "" {
		return classicalBookInMyBooksURL(c.BookID)
	}
	if c.WorkKey == "" {
		return ""
	}
	return classicalWorkInMyBooksURL(c.WorkKey)
}

func classicalAddConfirmationFor(report classical.AddReport, preparation classicalAddPreparation, activeLanguage, workKey string) classicalAddConfirmation {
	message := classicalAddMessage(report)
	if !classicalAddNamesOtherLanguage(activeLanguage) {
		return classicalAddConfirmation{Message: message + classicalAddPreparationMessage(preparation)}
	}
	// Every membership message is one sentence ending with a period, so the
	// language joins that sentence instead of starting a second one.
	return classicalAddConfirmation{
		Message: strings.TrimSuffix(message, ".") + " in " + classical.LanguageDisplayName + ". Your study language is unchanged." + classicalAddPreparationMessage(preparation),
		WorkKey: workKey,
	}
}

// classicalAddPreparationMessage states what happens to the text after Add.
// The catalog never promises work that is already done or that could not start.
func classicalAddPreparationMessage(preparation classicalAddPreparation) string {
	switch {
	case preparation.Refusal != "":
		return " " + preparation.Refusal
	case preparation.CouldNotStart > 0:
		return " The text could not be prepared. Retry preparation from My Books when ready."
	case preparation.Queued > 0:
		return " Downloading and preparing the text."
	case preparation.AlreadyPrepared > 0:
		return " The text is already prepared."
	default:
		return ""
	}
}

// classicalAddNamesOtherLanguage reports whether the Add must name Ancient
// Greek: the learner has a study language and it is not the language the Books
// were added to. With no resolved language, the Add makes Ancient Greek the only
// study language, so My Books shows the Books without a language link.
func classicalAddNamesOtherLanguage(activeLanguage string) bool {
	return activeLanguage != "" && canonicalization.NormalizeLanguage(activeLanguage) != classical.Language
}

// redirectClassicalAdd returns to the catalog page with the Add outcome. The
// page never carries an arbitrary link target: it derives the "In My Books"
// path from the catalog's own work key.
func (h *Handler) redirectClassicalAdd(w http.ResponseWriter, r *http.Request, target string, confirmation classicalAddConfirmation) {
	query := url.Values{"message": {confirmation.Message}}
	if confirmation.WorkKey != "" {
		query.Set("added_work", confirmation.WorkKey)
	}
	redirect(w, r, target+"?"+query.Encode())
}

// loadClassicalAddConfirmation reads the Add outcome from the page query. A work
// key is honored only when it names a catalog work and another study language
// is still active; the link then goes through the existing "In My Books" path,
// which switches the study language on the learner's deliberate follow.
func (h *Handler) loadClassicalAddConfirmation(r *http.Request) classicalAddConfirmation {
	message := r.URL.Query().Get("message")
	if message == "" {
		return classicalAddConfirmation{}
	}
	confirmation := classicalAddConfirmation{Message: message}
	if bookID := r.URL.Query().Get("read_book"); bookID != "" {
		if _, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), user(r).ID, bookID); err == nil {
			confirmation.BookID = bookID
		}
		return confirmation
	}
	workKey := r.URL.Query().Get("added_work")
	if workKey == "" || !classicalAddNamesOtherLanguage(activeLanguageOf(r)) {
		return confirmation
	}
	if _, ok := h.services.Classical.Work(workKey); !ok {
		return confirmation
	}
	confirmation.WorkKey = workKey
	return confirmation
}

func activeLanguageOf(r *http.Request) string {
	language, _ := activeStudyLanguageForContext(r.Context())
	return language
}

// redirectClassicalAddFailure names the stable legacy reason instead of
// treating a refused Add as a server error. Any other failure is logged for the
// operator and the learner sees a recovery action, never the raw error.
func (h *Handler) redirectClassicalAddFailure(w http.ResponseWriter, r *http.Request, target string, err error) {
	if errors.Is(err, classical.ErrLegacyRows) {
		log.Printf("mouseion: classical catalog add refused code=%s", classical.LegacyRowsCode)
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
	case created == 1 && opened == 1:
		return "Added 1 Book and opened 1 existing Book."
	case created == 1:
		return fmt.Sprintf("Added 1 Book and opened %d existing Books.", opened)
	case opened == 1:
		return fmt.Sprintf("Added %d Books and opened 1 existing Book.", created)
	default:
		return fmt.Sprintf("Added %d Books and opened %d existing Books.", created, opened)
	}
}

// classicalBookInMyBooks makes the Book's language active and lands on the
// existing Book's row.
func (h *Handler) classicalBookInMyBooks(w http.ResponseWriter, r *http.Request) {
	h.openClassicalBookInMyBooks(w, r, r.PathValue("bookID"))
}

// catalogBookInMyBooks opens any owned Book from a Catalog entry row.
func (h *Handler) catalogBookInMyBooks(w http.ResponseWriter, r *http.Request) {
	h.openCatalogBookInMyBooks(w, r, r.PathValue("bookID"))
}

func (h *Handler) openClassicalBookInMyBooks(w http.ResponseWriter, r *http.Request, bookID string) {
	h.openBookInMyBooks(w, r, bookID, true)
}

func (h *Handler) openCatalogBookInMyBooks(w http.ResponseWriter, r *http.Request, bookID string) {
	h.openBookInMyBooks(w, r, bookID, false)
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

// openBookInMyBooks switches the active language and redirects to the row's
// visible bucket page. A classical Book must still carry its Reading unit; an
// OPDS Book only needs an owned membership and a chosen language.
func (h *Handler) openBookInMyBooks(w http.ResponseWriter, r *http.Request, bookID string, requireReadingUnit bool) {
	owner := user(r).ID
	book, err := h.services.Store.MyBooks.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	if requireReadingUnit && book.ReadingUnit == nil {
		http.NotFound(w, r)
		return
	}
	language := canonicalization.NormalizeLanguage(book.Book.LanguageTag)
	if book.Book.LanguageState != domain.LanguageChosen || language == "" {
		http.NotFound(w, r)
		return
	}
	if err := h.services.Store.Shell.SetActiveStudyLanguage(r.Context(), owner, language); err != nil {
		fail(w, err)
		return
	}
	page, err := myBooksPageForBook(r.Context(), h.services.Store.MyBooks, owner, language, book)
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, myBookBucketURL(book, page)+"#"+myBookRowID(book.Book.ID))
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

// syncConnection enqueues the connection's ordinary sync. Its return target
// follows the page that submitted it: the Catalog's status line or Manage
// sources.
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
		h.redirectSyncOutcome(w, r, "error", "The catalog sync could not be started. Try again.")
		return
	}
	h.redirectSyncOutcome(w, r, "message", "Catalog sync submitted.")
}

func (h *Handler) redirectSyncOutcome(w http.ResponseWriter, r *http.Request, key, message string) {
	target := "/catalogs/sources"
	// The hidden field is the current contract; the legacy path stays a
	// fallback for the retired POST /connections form while it is supported.
	if r.PostFormValue("return") == "catalog" || strings.HasPrefix(r.URL.Path, "/connections") {
		target = "/catalogs"
	}
	redirect(w, r, target+"?"+key+"="+url.QueryEscape(message))
}

func (h *Handler) createConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	u := user(r)
	bookID := strings.TrimSpace(r.FormValue("book_id"))
	connectionURL, err := opds.NormalizeCatalogURL(r.FormValue("url"))
	if err != nil {
		redirectManageSources(w, r, "error", "The catalog URL must use HTTP or HTTPS.", bookID)
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
	redirectManageSources(w, r, "message", "Catalog added", bookID)
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
		redirectManageSources(w, r, "error", "The catalog URL must use HTTP or HTTPS.", "")
		return
	}
	_, e = h.services.Store.Catalogs.UpdateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{ID: current.ID, Name: strings.TrimSpace(r.FormValue("name")), URL: connectionURL, Username: r.FormValue("username"), Password: password})
	if e != nil {
		fail(w, e)
		return
	}
	redirectManageSources(w, r, "message", "Catalog updated", "")
}
func (h *Handler) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	u := user(r)
	if e := h.services.Store.Catalogs.DeleteOpdsConnection(r.Context(), u.ID, r.PathValue("id")); e != nil {
		if errors.Is(e, domain.ErrNotFound) {
			redirectManageSources(w, r, "error", "That catalog connection no longer exists.", "")
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
	redirectManageSources(w, r, "message", "Catalog deleted", "")
}
func opdsErrorMessage(err error) string {
	message := "The catalog request failed. Check the connection and try again."
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, cataloguesync.ErrCatalogEntryMissing):
		message = "The catalog no longer offers this Book. Sync the connection, then retry."
	case errors.Is(err, opds.ErrNoEPUB), errors.Is(err, opds.ErrIncompatibleMediaType):
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
	case errors.Is(err, opds.ErrImportFailed), strings.Contains(lower, "ingest downloaded epub"), strings.Contains(lower, "validate epub"):
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
