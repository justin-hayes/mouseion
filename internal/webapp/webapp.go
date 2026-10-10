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
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/lemmarisk"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
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

// MyBooksStore provides the My Books (/library/**) browse, intent, visibility,
// and metadata-refresh reads and writes.
type MyBooksStore interface {
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
	GetBookDetailForMyBooksRefresh(context.Context, string, string) (domain.MyBook, error)
	ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	ListMyBooksBrowseWithVisibility(context.Context, string, string, string, string, bool, bool, int, int) (persistence.MyBooksBrowseResult, error)
	ImportPreviouslyRead(context.Context, string, string) (domain.ReadingCompletion, error)
	TransitionBookDisposition(context.Context, string, string, int64, domain.BookDisposition) (bool, error)
	SetBookHidden(context.Context, string, string, int64, bool) (bool, error)
	GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error)
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
	StartCurrentReadingResult(context.Context, string, string, string) (persistence.StartResult, error)
	SwitchCurrentReading(context.Context, string, string, string, string, string) (domain.CurrentReading, error)
	EndCurrentReading(context.Context, string, string, string, string) error
	FinishCurrentReading(context.Context, string, string, string, string) (domain.CurrentReadingFinishResult, error)
	ListVocabularyBrowsePage(context.Context, string, string, domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error)
	LemmaReviewStore
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
}

// JobsStore provides the analysis job list shown by the jobs surface.
type JobsStore interface {
	ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error)
}

// LemmaReviewStore provides the lemma review seam used by the Reading surface.
type LemmaReviewStore interface {
	ListLemmaReviewOccurrences(context.Context, string, string, string) ([]domain.LemmaReviewOccurrence, error)
	SaveLemmaReviewFlags(context.Context, []domain.LemmaReviewFlag) error
	PutLemmaDecision(context.Context, domain.LemmaReviewOccurrence, string, bool, string, string) error
	PutLemmaDecisions(context.Context, []domain.LemmaReviewDecision) error
	HasCurrentLemmaCorrections(context.Context, string, string) (bool, error)
	ReadLemmaReviewProposal(context.Context, domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error)
	PutLemmaDecisionProposal(context.Context, domain.LemmaReviewProposal, string) error
	ListDeckPreparationsForSourceMaterial(context.Context, string, string) ([]domain.DeckPreparation, error)
}

// StoreDependencies groups the persistence capabilities consumed by the web
// application, one interface per learner surface. Production supplies one
// persistence store to every field; overlapping method sets are intentional.
type StoreDependencies struct {
	Shell      ShellStore
	MyBooks    MyBooksStore
	Reading    ReadingStore
	Vocabulary VocabularyStore
	Catalogs   CatalogsStore
	Jobs       JobsStore
}

type csrfFailureContextKey struct{}
type OPDS interface {
	AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
}
type Analysis interface {
	SubmitAnalysis(context.Context, string, string) (analysis.Handle, error)
	SubmitToReadBookAnalysis(context.Context, string, string, string) (analysis.Handle, error)
	Get(context.Context, string, int64) (analysis.Status, error)
	Reconcile(context.Context, string, int64) (analysis.Status, error)
	Retry(context.Context, string, int64) (analysis.Handle, error)
	Cancel(context.Context, string, int64) (analysis.Status, error)
	EnqueueBrowseCountRebuild(context.Context, string, string) error
	GetCompletedAnalysis(context.Context, string, string, string) (analysis.CompletedAnalysis, error)
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
	RefreshEntry(context.Context, string, string) (cataloguesync.RefreshResult, error)
	FindAcquisitionTarget(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
}
type AnalysisInsights interface {
	Coverage(context.Context, string, string) (domain.AnalysisCoverage, error)
}
type KnownVocabulary interface {
	Submit(context.Context, string, string, string) (knownvocab.Handle, error)
	Get(context.Context, string, int64) (knownvocab.Status, error)
}
type PreparedDeck interface {
	Submit(context.Context, string, string) (prepareddeck.Handle, error)
	SubmitForGoal(context.Context, string, string, string) (prepareddeck.Handle, error)
	Get(context.Context, string, string) (domain.DeckPreparation, error)
	GetForGoalSnapshot(context.Context, string, string) (domain.DeckPreparation, error)
	Cancel(context.Context, string, string) (domain.DeckPreparation, error)
	Retry(context.Context, string, string) (prepareddeck.Handle, error)
	Reprepare(context.Context, string, string) (prepareddeck.Handle, error)
	Rerender(context.Context, string, string) (prepareddeck.Handle, error)
	Download(context.Context, string, string) (domain.DeckPreparation, error)
}

// Services keeps UI dependencies explicit and makes web-level tests independent of infrastructure.
type Services struct {
	Auth             *auth.Service
	WebAuth          *webauth.Handler
	Store            StoreDependencies
	OPDS             OPDS
	Analysis         Analysis
	AnalysisInsights AnalysisInsights
	KnownVocab       KnownVocabulary
	PreparedDeck     PreparedDeck
	Capabilities     analyzer.CapabilityProvider
	LemmaRiskIndex   lemmarisk.AlternativeIndex
	LemmaSuggestions enrichment.LemmaSuggestionProvider
	CatalogueSync    CatalogueSync
	SecureCookies    bool
	SessionLifetime  time.Duration
	// InteractiveReadTimeout bounds Browse and Concordance reads; zero uses
	// defaultInteractiveReadTimeout.
	InteractiveReadTimeout time.Duration
}

// defaultInteractiveReadTimeout stays below the 9s htmx request timeout so the
// server's recoverable timeout page wins over the browser-side one.
const defaultInteractiveReadTimeout = 8 * time.Second

func (h *Handler) interactiveReadTimeout() time.Duration {
	if h.services.InteractiveReadTimeout > 0 {
		return h.services.InteractiveReadTimeout
	}
	return defaultInteractiveReadTimeout
}

type Handler struct {
	services Services
	mux      *http.ServeMux
}

func New(s Services) *Handler {
	h, err := NewWithError(s)
	if err != nil {
		panic(err)
	}
	return h
}

func NewWithError(s Services) (*Handler, error) {
	if err := persistence.ValidateSecret(os.Getenv("MOUSEION_SECRET")); err != nil {
		return nil, fmt.Errorf("webapp: initialize web key: %w", err)
	}
	h := &Handler{services: s, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /login", h.loginPage)
	h.mux.HandleFunc("POST /login", h.login)
	h.mux.HandleFunc("POST /onboarding", h.onboard)
	h.mux.Handle("POST /logout-all", h.user(http.HandlerFunc(h.logoutAll)))
	h.mux.Handle("GET /{$}", h.user(http.HandlerFunc(h.dashboard)))
	h.mux.Handle("GET /library", h.user(http.HandlerFunc(h.library)))
	h.mux.Handle("GET /reading", h.user(http.HandlerFunc(h.reading)))
	h.mux.Handle("GET /reading/switch", h.user(http.HandlerFunc(h.switchReadingPage)))
	h.mux.Handle("GET /reading/books/{bookID}/lemma-review", h.user(http.HandlerFunc(h.lemmaReview)))
	h.mux.Handle("POST /reading/books/{bookID}/lemma-review", h.user(http.HandlerFunc(h.correctLemma)))
	h.mux.Handle("POST /reading/books/{bookID}/lemma-suggestion", h.user(http.HandlerFunc(h.suggestLemma)))
	h.mux.Handle("POST /reading/books/{id}/start", h.user(http.HandlerFunc(h.startReading)))
	h.mux.Handle("POST /reading/books/{id}/switch", h.user(http.HandlerFunc(h.switchReading)))
	h.mux.Handle("POST /reading/finish", h.user(http.HandlerFunc(h.finishCurrentReading)))
	h.mux.Handle("POST /reading/end", h.user(http.HandlerFunc(h.endCurrentReading)))
	h.mux.Handle("POST /reading/books/{id}/deck/cancel", h.user(http.HandlerFunc(h.cancelCurrentReadingDeck)))
	h.mux.Handle("POST /reading/books/{id}/deck/retry", h.user(http.HandlerFunc(h.retryCurrentReadingDeck)))
	// Preserve focused artifact controls under their old URLs; these routes
	// operate only on the current reading's exact snapshot, never its lifecycle.
	h.mux.Handle("POST /goal/books/{id}/deck/cancel", h.user(http.HandlerFunc(h.cancelCurrentReadingDeck)))
	h.mux.Handle("POST /goal/books/{id}/deck/retry", h.user(http.HandlerFunc(h.retryCurrentReadingDeck)))
	h.mux.Handle("POST /reading/books/{id}/reanalyze", h.user(http.HandlerFunc(h.reanalyzeToReadBook)))
	h.mux.Handle("GET /books/{id}/cover", h.user(http.HandlerFunc(h.bookCover)))
	h.mux.Handle("POST /library/books/{id}/refresh", h.user(http.HandlerFunc(h.refreshBookMetadata)))
	h.mux.Handle("POST /library/books/{id}/to-read", h.user(http.HandlerFunc(h.moveBookToRead)))
	h.mux.Handle("POST /library/books/{id}/hide", h.user(http.HandlerFunc(h.hideBook)))
	h.mux.Handle("POST /library/books/{id}/unhide", h.user(http.HandlerFunc(h.unhideBook)))
	h.mux.Handle("POST /library/books/{id}/previously-read", h.user(http.HandlerFunc(h.markBookPreviouslyRead)))
	h.mux.Handle("POST /library/books/{id}/read-again", h.user(http.HandlerFunc(h.moveBookToRead)))
	h.mux.Handle("GET /books/{id}/analyses/{runID}", h.user(http.HandlerFunc(h.analysisResult)))
	h.mux.Handle("GET /reading/books/{bookID}/deck/preparations/new", h.user(http.HandlerFunc(h.newReadingDeckPreparation)))
	h.mux.Handle("POST /reading/books/{id}/deck/preparations", h.user(http.HandlerFunc(h.createReadingEntryDeckPreparation)))
	h.mux.Handle("POST /jobs/{id}/deck/preparations", h.user(http.HandlerFunc(h.createDeckPreparation)))
	h.mux.Handle("GET /deck-preparations/{id}/status", h.user(http.HandlerFunc(h.deckPreparationStatus)))
	h.mux.Handle("POST /deck-preparations/{id}/cancel", h.user(http.HandlerFunc(h.cancelDeckPreparation)))
	h.mux.Handle("POST /deck-preparations/{id}/retry", h.user(http.HandlerFunc(h.retryDeckPreparation)))
	h.mux.Handle("POST /deck-preparations/{id}/reprepare", h.user(http.HandlerFunc(h.reprepareDeckPreparation)))
	h.mux.Handle("POST /deck-preparations/{id}/rerender", h.user(http.HandlerFunc(h.rerenderDeckPreparation)))
	h.mux.Handle("GET /deck-preparations/{id}/download", h.user(http.HandlerFunc(h.downloadDeckPreparation)))
	h.mux.Handle("POST /logout", h.user(http.HandlerFunc(h.logout)))
	h.mux.Handle("GET /settings", h.user(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirect(w, r, "/library")
	})))
	h.mux.Handle("GET /catalogs", h.user(http.HandlerFunc(h.connections)))
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
	h.mux.Handle("GET /vocabulary", h.user(http.HandlerFunc(h.vocabularyConcordancePage)))
	h.mux.Handle("GET /vocabulary/concordance", h.user(http.HandlerFunc(h.vocabularyConcordancePage)))
	h.mux.Handle("GET /vocabulary/concordance/sentence", h.user(http.HandlerFunc(h.vocabularySentenceStudyPage)))
	h.mux.Handle("GET /vocabulary/import", h.user(http.HandlerFunc(h.vocabularyImportPage)))
	h.mux.Handle("POST /active-study-language", h.user(http.HandlerFunc(h.activeStudyLanguage)))
	h.mux.Handle("POST /vocabulary/import", h.user(http.HandlerFunc(h.importKnownVocab)))
	h.mux.Handle("GET /vocabulary/imports/{id}/status", h.user(http.HandlerFunc(h.knownVocabImportStatus)))
	return h, nil
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
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
		if requestNamesOtherLanguage(r, view) {
			// Language precedence: recover before any reading or evidence restoration.
			redirectLanguageChanged(w, r)
			return
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
		return languageBoundCSRFToken(r.Context(), c.Value)
	}
	token := h.rotateCSRF(w, r)
	if token == "" {
		return ""
	}
	return languageBoundCSRFToken(r.Context(), token)
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
	//nolint:gosec // plain HTTP is supported on the private tailnet; HttpOnly and SameSite remain enabled.
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: token, Path: "/", HttpOnly: true, Secure: h.services.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(h.services.SessionLifetime.Seconds())})
	return token
}

// checkCSRF authorizes a mutation and revalidates the study language the form
// was rendered for. A submission from a page rendered under another language
// recovers to My Books before any handler state changes.
func (h *Handler) checkCSRF(w http.ResponseWriter, r *http.Request) bool {
	formLanguage, scoped, ok := h.verifyCSRFToken(w, r)
	if !ok {
		return false
	}
	if scoped && !formLanguageIsActive(r.Context(), formLanguage) {
		redirectLanguageChanged(w, r)
		return false
	}
	return true
}

// checkCSRFAnyLanguage authorizes language-agnostic mutations, such as the
// deliberate language change itself, sign-out, and catalog management.
func (h *Handler) checkCSRFAnyLanguage(w http.ResponseWriter, r *http.Request) bool {
	_, _, ok := h.verifyCSRFToken(w, r)
	return ok
}

func (h *Handler) verifyCSRFToken(w http.ResponseWriter, r *http.Request) (language string, scoped, ok bool) {
	c, err := r.Cookie(csrfCookie)
	token, language, scoped := strings.Cut(r.FormValue("csrf_token"), csrfLanguageSeparator)
	if err != nil || token == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(token)) != 1 {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return "", false, false
	}
	return language, scoped, true
}
func (h *Handler) setSession(w http.ResponseWriter, token string) {
	//nolint:gosec // plain HTTP is supported on the private tailnet; HttpOnly and SameSite remain enabled.
	http.SetCookie(w, &http.Cookie{Name: webauth.CookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.services.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(h.services.SessionLifetime.Seconds())})
}
func (h *Handler) clearSession(w http.ResponseWriter) {
	//nolint:gosec // plain HTTP is supported on the private tailnet; HttpOnly and SameSite remain enabled.
	http.SetCookie(w, &http.Cookie{Name: webauth.CookieName, Path: "/", HttpOnly: true, Secure: h.services.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
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
