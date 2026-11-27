// Package webapp provides Mouseion's authenticated server-rendered web client.
package webapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/deploysecret"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/readingintent"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

const (
	csrfCookie     = "mouseion_csrf"
	maxRequestBody = 4 << 20
)

// ShellStore provides the study-language settings and the book cover reads
// shared by the application shell and every learner surface.
type ShellStore interface {
	ListStudyLanguages(context.Context, string) ([]domain.StudyLanguage, error)
	ListKnownVocabularyLanguages(context.Context, string) ([]domain.StudyLanguage, error)
	GetStoredActiveStudyLanguage(context.Context, string) (string, error)
	SetActiveStudyLanguage(context.Context, string, string) error
	MostRecentlyActivatedStudyLanguage(context.Context, string) (string, error)
	GetActiveBookCoverResource(context.Context, string, string) (domain.BookCoverResource, error)
}

// MyBooksStore provides the My Books (/library/**) browse, intent,
// membership-removal, reading-history, and metadata-refresh reads and writes.
type MyBooksStore interface {
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
	ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	ListMyBooksBrowse(context.Context, string, string, string, string, bool, int, int) (domain.MyBooksBrowseResult, error)
	// MarkBookPreviouslyRead records an unstarted To Read Book as Previously
	// read and clears its To Read disposition in one transition.
	MarkBookPreviouslyRead(context.Context, string, string) error
	TransitionBookDisposition(context.Context, string, string, int64, domain.BookDisposition) (bool, error)
	GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error)
	RemoveBookFromMyBooks(context.Context, string, string) error
}

// ReadingStore provides the Reading (/reading/**) surface: the owner-scoped
// book reads, the current reading lifecycle, the lemma review seam, and the
// vocabulary browse shown beside the current reading.
type ReadingStore interface { //nolint:interfacebloat // the Reading surface owns the current reading lifecycle and the lemma review seam; splitting either is a separate design change
	GetBook(context.Context, string, string) (domain.Book, error)
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
	ListSourceMaterials(context.Context, string) ([]domain.SourceMaterialSummary, error)
	ResolveBookID(context.Context, string, string) (string, bool, error)
	ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error)
	CountCurrentReadingVocabularyToAccept(context.Context, string, string) (int, error)
	StartCurrentReading(context.Context, string, string, string) (domain.CurrentReading, error)
	StartCurrentReadingResult(context.Context, string, string, string) (domain.StartResult, error)
	SwitchCurrentReading(context.Context, string, string, string, string, string) (domain.CurrentReading, error)
	EndCurrentReading(context.Context, string, string, string, string) error
	FinishCurrentReading(context.Context, string, string, string, string) (domain.CurrentReadingFinishResult, error)
	ListVocabularyBrowsePage(context.Context, string, string, domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error)
	lemmareview.Store
}

// VocabularyStore provides the Vocabulary (/vocabulary/**) concordance and
// sentence study reads.
type VocabularyStore interface {
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
	GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error)
	ListVocabularyConcordance(context.Context, string, string, domain.ConcordanceLookup) (domain.ConcordanceResult, error)
	GetVocabularySentenceStudy(context.Context, string, string, string, string, string, int64, int64, string) (domain.SentenceStudy, error)
}

// CatalogsStore provides owner-scoped OPDS connection management and the
// catalogue alias and sync status reads shown beside it.
type CatalogsStore interface {
	CreateOpdsConnection(context.Context, string, domain.OpdsConnection) (domain.OpdsConnection, error)
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
	ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error)
	UpdateOpdsConnection(context.Context, string, domain.OpdsConnection) (domain.OpdsConnection, error)
	DeleteOpdsConnection(context.Context, string, string) error
	GetBookCatalogEntryAlias(context.Context, string, string) (domain.BookAlias, error)
	// BrowseCatalogueEntries reads one connection's local Catalog index for
	// browsing. It filters through the Go search key and attaches ownership;
	// it never contacts the OPDS server.
	BrowseCatalogueEntries(context.Context, string, string, domain.CatalogueQuery) (domain.CataloguePage, error)
	// The built-in Classical catalog is read from the release and writes only
	// My Books membership: Add creates or restores one To Read Book per Reading
	// unit. Legacy rows are checked installation-wide before any write.
	ListLegacyLanguageRows(context.Context, string) ([]domain.LegacyLanguageRows, error)
	ListOwnedClassicalReadingUnits(context.Context, string) ([]classical.OwnedReadingUnit, error)
	AddClassicalReadingUnits(context.Context, string, []string) (classical.AddReport, error)
}

// CatalogEntryCoverStore reads one retained Catalog entry image for its owner.
// Anything but an available image is ErrNotFound.
type CatalogEntryCoverStore interface {
	GetCatalogEntryCoverResource(context.Context, domain.CatalogEntryKey) (domain.CatalogEntryCoverResource, error)
}

// CatalogueAddStore creates or restores the Book for one indexed Catalog entry
// as To Read, linked through its catalog-entry alias.
type CatalogueAddStore interface {
	AddCatalogueEntry(context.Context, string, string, string) (domain.CatalogueAddResult, error)
}

// CatalogueReadStore records a Catalog entry or Classical Reading unit as
// Previously read, creating or restoring its Book as Read without acquiring or
// preparing anything.
type CatalogueReadStore interface {
	MarkCatalogueEntryPreviouslyRead(context.Context, string, string, string) (domain.CatalogueReadResult, error)
	MarkClassicalReadingUnitPreviouslyRead(context.Context, string, string) (domain.CatalogueReadResult, error)
}

// JobsStore provides the one Book preparation list the jobs surface shows.
type JobsStore interface {
	ListBookPreparationJobs(context.Context, string) ([]domain.BookPreparationJob, error)
}

// StoreDependencies groups the persistence capabilities consumed by the web
// application, one interface per learner surface. Production supplies one
// persistence store to every field; overlapping method sets are intentional.
type StoreDependencies struct {
	Shell         ShellStore
	MyBooks       MyBooksStore
	Reading       ReadingStore
	Vocabulary    VocabularyStore
	Catalogs      CatalogsStore
	CatalogCovers CatalogEntryCoverStore
	CatalogueAdd  CatalogueAddStore
	CatalogueRead CatalogueReadStore
	Jobs          JobsStore
	Study         StudyStore
}

type csrfFailureContextKey struct{}
type OPDS interface {
	AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
}
type Analysis interface {
	SubmitToReadBookAnalysis(context.Context, string, string, string) (analysis.Handle, error)
	Get(context.Context, string, int64) (analysis.Status, error)
	Reconcile(context.Context, string, int64) (analysis.Status, error)
	Retry(context.Context, string, int64) (analysis.Handle, error)
	Cancel(context.Context, string, int64) (analysis.Status, error)
	EnqueueBrowseCountRebuild(context.Context, string, string) error
	GetCompletedAnalysis(context.Context, string, string, string) (analysis.CompletedAnalysis, error)
}

// ClassicalPreparation is the To Read preparation seam for a classical Book.
// A nil seam leaves disposition changes working without a preparation job.
type ClassicalPreparation interface {
	SubmitClassicalPreparation(context.Context, string, string) (analysis.ClassicalPreparationHandle, error)
}

// EPUBAcquisition enqueues the durable acquisition job a Catalog Book needs
// before its analysis can start.
type EPUBAcquisition interface {
	SubmitEPUBAcquisition(context.Context, string, string) (analysis.EPUBAcquisitionHandle, error)
}

// AnalysisUpdates offers and submits learner-confirmed Analysis updates
// (ADR 0107). A nil seam offers nothing, as when the current Analysis
// pipeline identity is unknown.
type AnalysisUpdates interface {
	AnalysisUpdateCandidates(ctx context.Context, owner string, sourceIDs []string) (map[string]domain.AnalysisUpdateCandidate, error)
	SubmitAnalysisUpdate(ctx context.Context, owner, bookID, sourceID, expectedRunID, expectedTarget string) (analysis.Handle, error)
}

// SourceStructureUpdates offers and submits learner-confirmed Source structure
// updates (ADR 0103). A nil seam offers nothing.
type SourceStructureUpdates interface {
	SourceStructureUpdateCandidates(ctx context.Context, owner string, sourceIDs []string) (map[string]domain.SourceStructureUpdateCandidate, error)
	SubmitSourceStructureUpdate(ctx context.Context, owner, bookID, sourceID string, expected domain.SourceStructureUpdateExpectation) (analysis.Handle, error)
}

// CatalogueSync is the catalogue sync seam. Connection CRUD maintains the River
// periodic schedule through it without coupling handlers to River or exposing
// job credentials; the Jobs surface and Library refresh act on sync runs through
// the same seam.
type CatalogueSync interface {
	RegisterConnection(context.Context, string, string) error
	UnregisterConnection(string, string) error
	// ListCatalogueSyncStatuses overlays durable sync status with River's live
	// state, so the store alone cannot report a stale Syncing row accurately.
	ListCatalogueSyncStatuses(context.Context, string) ([]domain.CatalogueSyncStatus, error)
	List(context.Context, string) ([]cataloguesync.Status, error)
	Get(context.Context, string, int64) (cataloguesync.Status, error)
	Enqueue(context.Context, string, string) (cataloguesync.Handle, error)
	Retry(context.Context, string, int64) (cataloguesync.Handle, error)
	Cancel(context.Context, string, int64) (cataloguesync.Status, error)
}
type AnalysisInsights interface {
	Coverage(context.Context, string, string) (domain.AnalysisCoverage, error)
}

// Services keeps UI dependencies explicit and makes web-level tests independent of infrastructure.
type Services struct {
	Auth             *auth.Service
	WebAuth          *webauth.Handler
	Store            StoreDependencies
	OPDS             OPDS
	Analysis         Analysis
	AnalysisUpdates  AnalysisUpdates
	SourceStructures SourceStructureUpdates
	// ClassicalPreparation prepares a classical Book when it moves to To Read.
	ClassicalPreparation ClassicalPreparation
	// EPUBAcquisition enqueues the durable download and import an EPUB Book
	// needs before analysis.
	EPUBAcquisition EPUBAcquisition
	// ReadingIntent carries out a Book's reading intent: the To Read
	// disposition transition, the route to Classical preparation or EPUB
	// analysis, and the operator log line. A nil module composes one from the
	// handler's live services, so a test that replaces one service before a
	// move is observed.
	ReadingIntent    *readingintent.Module
	AnalysisInsights AnalysisInsights
	// Study enqueues Study material preparation. It may be nil, in which case
	// the Study page only reports what is ready.
	Study Study
	// StudyScheduler computes a word's next state when a Study card is graded.
	// It is required for grading; the FSRS adapter in internal/study is the
	// implementation.
	StudyScheduler domain.StudyScheduler
	// Dictionary is the imported dictionary a source-evidence word's Study card
	// reads its cited entries from. It may be nil, in which case the card shows
	// no dictionary outline.
	Dictionary   enrichment.LexicalProvider
	Capabilities analyzer.CapabilityProvider
	// LemmaReview is the lemma review workflow. NewWithError derives one from
	// the Reading store and Analysis when it is nil.
	LemmaReview   *lemmareview.Service
	CatalogueSync CatalogueSync
	// Classical is the built-in, read-only Classical catalog from the release's
	// embedded manifest. NewWithError loads the embedded one when nil.
	Classical       *classical.Catalog
	SecureCookies   bool
	SessionLifetime time.Duration
	// InteractiveReadTimeout bounds Browse and Concordance reads; zero uses
	// defaultInteractiveReadTimeout.
	InteractiveReadTimeout time.Duration
	// Clock reports the current time for time-dependent behaviour such as the
	// Study day. NewWithError uses time.Now when it is nil. It is a plain
	// function so a test can pass a controllable clock whose state the handler
	// shares.
	Clock func() time.Time
}

// defaultInteractiveReadTimeout stays below the 9s htmx request timeout so the
// server's recoverable timeout page wins over the browser-side one.
const defaultInteractiveReadTimeout = 8 * time.Second

// now returns the current time from the injected clock.
func (h *Handler) now() time.Time { return h.services.Clock() }

func (h *Handler) interactiveReadTimeout() time.Duration {
	if h.services.InteractiveReadTimeout > 0 {
		return h.services.InteractiveReadTimeout
	}
	return defaultInteractiveReadTimeout
}

type Handler struct {
	services Services
	mux      *http.ServeMux
	// languagePages are the per-language routes served under /{language}/.
	languagePages map[string]languagePrefixed
}

func New(s Services) *Handler {
	h, err := NewWithError(s)
	if err != nil {
		panic(err)
	}
	return h
}

func NewWithError(s Services) (*Handler, error) {
	if err := deploysecret.ValidateSecret(os.Getenv("MOUSEION_SECRET")); err != nil {
		return nil, fmt.Errorf("webapp: validate MOUSEION_SECRET: %w", err)
	}
	if s.Clock == nil {
		s.Clock = time.Now
	}
	if s.LemmaReview == nil {
		s.LemmaReview = lemmareview.New(lemmareview.Config{Store: s.Store.Reading, Suggestions: enrichment.DisabledLemmaSuggestionProvider{}, BrowseCounts: s.Analysis})
	}
	if s.Classical == nil {
		catalog, err := classical.LoadCatalog(classical.ManifestJSONL)
		if err != nil {
			return nil, fmt.Errorf("webapp: load Classical catalog manifest: %w", err)
		}
		s.Classical = catalog
	}
	h := &Handler{services: s, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /login", h.loginPage)
	h.mux.HandleFunc("POST /login", h.login)
	h.mux.HandleFunc("POST /onboarding", h.onboard)
	h.mux.Handle("POST /logout-all", h.user(http.HandlerFunc(h.logoutAll)))
	h.mux.Handle("GET /{$}", h.user(http.HandlerFunc(h.dashboard)))
	h.perLanguage(pageKindMyBooks, h.library)
	h.perLanguage(pageKindReading, h.reading)
	h.perLanguage(pageKindReadingSwitch, h.switchReadingPage)
	h.perLanguage(pageKindStudy, h.study)
	h.perLanguage(pageKindStudyReports, h.studyReports)
	h.mux.Handle("POST /study/batches/take", h.user(http.HandlerFunc(h.takeStudyBatch)))
	h.mux.Handle("POST /study/batches/decline", h.user(http.HandlerFunc(h.declineStudyBatch)))
	h.mux.Handle("POST /study/grades", h.user(http.HandlerFunc(h.gradeStudyCard)))
	h.mux.Handle("POST /study/already-known", h.user(http.HandlerFunc(h.alreadyKnownStudyCard)))
	h.mux.Handle("POST /study/reports", h.user(http.HandlerFunc(h.reportStudyCard)))
	h.mux.Handle("POST /study/reports/{id}/dismiss", h.user(http.HandlerFunc(h.dismissStudyReport)))
	h.mux.Handle("GET /study/reports/{id}/remediation", h.user(http.HandlerFunc(h.studyRemediation)))
	h.mux.Handle("POST /study/reports/{id}/remediation", h.user(http.HandlerFunc(h.decideStudyRemediation)))
	h.mux.Handle("POST /study/reports/{id}/undo", h.user(http.HandlerFunc(h.undoStudyReport)))
	h.mux.Handle("POST /study/reports/{id}/next", h.user(http.HandlerFunc(h.nextAfterStudyReport)))
	h.mux.Handle("POST /study/fluency/responses", h.user(http.HandlerFunc(h.recordFluencyResponse)))
	h.mux.Handle("GET /reading/books/{bookID}/lemma-review", h.user(http.HandlerFunc(h.lemmaReview)))
	h.mux.Handle("POST /reading/books/{bookID}/lemma-review", h.user(http.HandlerFunc(h.correctLemma)))
	h.mux.Handle("POST /reading/books/{bookID}/lemma-suggestion", h.user(http.HandlerFunc(h.suggestLemma)))
	h.mux.Handle("POST /reading/books/{id}/start", h.user(http.HandlerFunc(h.startReading)))
	h.mux.Handle("POST /reading/books/{id}/switch", h.user(http.HandlerFunc(h.switchReading)))
	h.mux.Handle("POST /reading/finish", h.user(http.HandlerFunc(h.finishCurrentReading)))
	h.mux.Handle("POST /reading/end", h.user(http.HandlerFunc(h.endCurrentReading)))
	h.mux.Handle("POST /reading/books/{id}/reanalyze", h.user(http.HandlerFunc(h.reanalyzeToReadBook)))
	h.mux.Handle("POST /reading/books/{id}/analysis-update", h.user(http.HandlerFunc(h.updateToReadBookAnalysis)))
	h.mux.Handle("POST /reading/books/{id}/source-structure-update", h.user(http.HandlerFunc(h.updateBookSourceStructure)))
	h.mux.Handle("GET /books/{id}/cover", h.user(http.HandlerFunc(h.bookCover)))
	h.mux.Handle("POST /library/books/{id}/to-read", h.user(http.HandlerFunc(h.moveBookToRead)))
	h.mux.Handle("POST /library/books/{id}/remove", h.user(http.HandlerFunc(h.removeBookFromMyBooks)))
	h.mux.Handle("POST /library/books/{id}/previously-read", h.user(http.HandlerFunc(h.markBookPreviouslyRead)))
	h.mux.Handle("POST /library/books/{id}/read-again", h.user(http.HandlerFunc(h.moveBookToRead)))
	h.mux.Handle("GET /books/{id}/analyses/{runID}", h.user(http.HandlerFunc(h.analysisResult)))
	h.mux.Handle("POST /logout", h.user(http.HandlerFunc(h.logout)))
	h.mux.Handle("GET /settings", h.user(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirect(w, r, "/library")
	})))
	h.mux.Handle("GET /catalogs", h.user(http.HandlerFunc(h.connections)))
	h.mux.Handle("GET /catalogs/sources", h.user(http.HandlerFunc(h.manageSources)))
	h.mux.Handle("POST /catalogs/sources", h.user(http.HandlerFunc(h.createConnection)))
	h.mux.Handle("POST /catalogs/sources/{id}", h.user(http.HandlerFunc(h.updateConnection)))
	h.mux.Handle("POST /catalogs/sources/{id}/delete", h.user(http.HandlerFunc(h.deleteConnection)))
	h.mux.Handle("POST /catalogs/sources/{id}/sync", h.user(http.HandlerFunc(h.syncConnection)))
	h.mux.Handle("GET /catalogs/sources/{id}/cover", h.user(http.HandlerFunc(h.catalogEntryCover)))
	h.mux.Handle("POST /catalogs/sources/{id}/add", h.user(http.HandlerFunc(h.addCatalogueEntry)))
	h.mux.Handle("POST /catalogs/sources/{id}/previously-read", h.user(http.HandlerFunc(h.markCatalogueEntryPreviouslyRead)))
	h.mux.Handle("GET /catalogs/classical", h.user(http.HandlerFunc(h.classicalAuthorIndex)))
	h.mux.Handle("GET /catalogs/classical/authors/{author}", h.user(http.HandlerFunc(h.classicalAuthor)))
	h.mux.Handle("GET /catalogs/classical/works/{workKey}", h.user(http.HandlerFunc(h.classicalWork)))
	h.mux.Handle("POST /catalogs/classical/units/{unitKey}/add", h.user(http.HandlerFunc(h.addClassicalUnit)))
	h.mux.Handle("POST /catalogs/classical/units/{unitKey}/previously-read", h.user(http.HandlerFunc(h.markClassicalUnitPreviouslyRead)))
	h.mux.Handle("GET /catalogs/classical/books/{bookID}/in-my-books", h.user(http.HandlerFunc(h.classicalBookInMyBooks)))
	h.mux.Handle("GET /catalogs/books/{bookID}/in-my-books", h.user(http.HandlerFunc(h.catalogBookInMyBooks)))
	h.mux.Handle("GET /connections", h.user(http.HandlerFunc(h.legacyConnections)))
	h.mux.Handle("POST /connections", h.user(http.HandlerFunc(h.createConnection)))
	h.mux.Handle("POST /connections/{id}", h.user(http.HandlerFunc(h.updateConnection)))
	h.mux.Handle("POST /connections/{id}/delete", h.user(http.HandlerFunc(h.deleteConnection)))
	h.mux.Handle("POST /connections/{id}/sync", h.user(http.HandlerFunc(h.syncConnection)))
	h.mux.Handle("GET /jobs", h.user(http.HandlerFunc(h.jobs)))
	h.mux.Handle("GET /jobs/{id}", h.user(http.HandlerFunc(h.job)))
	h.mux.Handle("GET /jobs/{id}/status", h.user(http.HandlerFunc(h.jobStatus)))
	h.mux.Handle("POST /jobs/{id}/retry", h.user(http.HandlerFunc(h.retryJob)))
	h.mux.Handle("POST /jobs/{id}/cancel", h.user(http.HandlerFunc(h.cancelJob)))
	h.perLanguage(pageKindVocabulary, h.vocabularyConcordancePage)
	h.perLanguage(pageKindConcordance, h.vocabularyConcordancePage)
	h.perLanguage(pageKindSentenceStudy, h.vocabularySentenceStudyPage)
	h.mux.Handle("GET /study-language", h.user(http.HandlerFunc(h.studyLanguageSwitch)))
	return h, nil
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	if next, ok := h.matchLanguagePage(r); ok {
		next.ServeHTTP(w, r)
		return
	}
	h.mux.ServeHTTP(w, r)
}
func (h *Handler) user(next http.Handler) http.Handler {
	return h.services.WebAuth.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := user(r)
		view, err := h.loadShellView(r.Context(), u.ID)
		if err != nil {
			fail(w, err)
			return
		}
		if view != nil {
			r = r.WithContext(context.WithValue(r.Context(), shellViewContextKey{}, view))
		}
		next.ServeHTTP(w, r)
	}))
}
func render(w http.ResponseWriter, r *http.Request, component interface {
	Render(context.Context, io.Writer) error
}) {
	if _, failed := r.Context().Value(csrfFailureContextKey{}).(error); failed {
		return
	}
	var body bytes.Buffer
	if err := component.Render(r.Context(), &body); err != nil {
		http.Error(w, "unable to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := body.WriteTo(w); err != nil {
		return
	}
}
func (h *Handler) csrf(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) >= 32 {
		return c.Value
	}
	token := h.rotateCSRF(w, r)
	if token == "" {
		return ""
	}
	return token
}
func (h *Handler) rotateCSRF(w http.ResponseWriter, r *http.Request) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Printf("generate CSRF token: %v", err)
		*r = *r.WithContext(context.WithValue(r.Context(), csrfFailureContextKey{}, err))
		http.Error(w, "unable to establish CSRF protection", http.StatusInternalServerError)
		return ""
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, webauth.NewCookie(csrfCookie, token, h.services.SecureCookies, int(h.services.SessionLifetime.Seconds())))
	return token
}

// checkCSRF authorizes a mutation. The URL, or the Book a Book-addressed action
// names, carries the language (ADR 0131); the token is the bare secret.
func (h *Handler) checkCSRF(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	token := r.FormValue("csrf_token")
	if err != nil || token == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(token)) != 1 {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return false
	}
	return true
}
func (h *Handler) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, webauth.NewCookie(webauth.CookieName, token, h.services.SecureCookies, int(h.services.SessionLifetime.Seconds())))
}
func (h *Handler) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, webauth.NewCookie(webauth.CookieName, "", h.services.SecureCookies, -1))
}
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	// SafeReturnPath preserves local navigation while rejecting absolute and
	// scheme-relative destinations supplied through request parameters.
	http.Redirect(w, r, webauth.SafeReturnPath(path), http.StatusSeeOther) // #nosec G710 -- SafeReturnPath rejects external redirect destinations.
}
func user(r *http.Request) domain.User { u, _ := webauth.UserFromContext(r.Context()); return u }
func isPartialHTMXRequest(r *http.Request) bool {
	return r.Header.Get("Hx-Request-Type") == "partial"
}

func fail(w http.ResponseWriter, err error) {
	log.Printf("mouseion: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func renderStatus(w http.ResponseWriter, r *http.Request, status int, component interface {
	Render(context.Context, io.Writer) error
}) {
	if _, failed := r.Context().Value(csrfFailureContextKey{}).(error); failed {
		return
	}
	var body bytes.Buffer
	if err := component.Render(r.Context(), &body); err != nil {
		http.Error(w, "unable to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := body.WriteTo(w); err != nil {
		return
	}
}
