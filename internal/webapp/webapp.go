// Package webapp provides Mouseion's authenticated server-rendered web client.
package webapp

import (
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
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

const (
	csrfCookie     = "mouseion_csrf"
	maxRequestBody = 4 << 20
)

// StudyLanguageStore provides the active study-language and vocabulary
// language settings consumed by the shell and vocabulary surfaces.
type StudyLanguageStore interface {
	ListStudyLanguages(context.Context, string) ([]domain.StudyLanguage, error)
	ListKnownVocabularyLanguages(context.Context, string) ([]domain.StudyLanguage, error)
	GetStoredActiveStudyLanguage(context.Context, string) (string, error)
	SetActiveStudyLanguage(context.Context, string, string) error
	MostRecentlyActivatedStudyLanguage(context.Context, string) (string, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
}

// BookStore provides the owner-scoped book reads and My Books membership
// operations shared by the library, Reading Journey, and Primary Goal surfaces.
type BookStore interface {
	GetBook(context.Context, string, string) (domain.Book, error)
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
	ListSourceMaterials(context.Context, string) ([]domain.SourceMaterialSummary, error)
	RemoveBookFromMyBooks(context.Context, string, string) error
}

// JourneyStore provides Reading Journey membership and ordering.
type JourneyStore interface {
	GetReadingJourney(context.Context, string, string) (domain.ReadingJourney, error)
	ResolveJourneyBookID(context.Context, string, string) (string, bool, error)
	AddToReadingJourney(context.Context, string, string, string, int64) (int64, error)
	RemoveFromReadingJourney(context.Context, string, string, string, int64) (int64, error)
	MoveReadingJourneyEntry(context.Context, string, string, string, int, int64) (int64, error)
}

// GoalStore provides the Primary Goal lifecycle.
type GoalStore interface {
	GetPrimaryGoal(context.Context, string, string) (domain.PrimaryGoal, error)
	CreatePrimaryGoal(context.Context, string, string, string) (domain.PrimaryGoal, error)
	ChangePrimaryGoal(context.Context, string, string, string, string) (domain.PrimaryGoal, error)
	ClearPrimaryGoal(context.Context, string, string, string) error
}

// CatalogStore provides owner-scoped OPDS connection management.
type CatalogStore interface {
	CreateOpdsConnection(context.Context, string, domain.OpdsConnection) (domain.OpdsConnection, error)
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
	ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error)
	UpdateOpdsConnection(context.Context, string, domain.OpdsConnection) (domain.OpdsConnection, error)
	DeleteOpdsConnection(context.Context, string, string) error
}

// AnalysisJobStore provides the analysis job list shown by the jobs surface.
type AnalysisJobStore interface {
	ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error)
}

// StoreDependencies groups the focused persistence capabilities consumed by the
// web application. Production supplies one persistence store to every field.
type StoreDependencies struct {
	StudyLanguages StudyLanguageStore
	Books          BookStore
	Journey        JourneyStore
	Goals          GoalStore
	Catalog        CatalogStore
	AnalysisJobs   AnalysisJobStore
}

type csrfFailureContextKey struct{}
type OPDS interface {
	AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
}
type Analysis interface {
	SubmitAnalysis(context.Context, string, string) (analysis.Handle, error)
	Get(context.Context, string, int64) (analysis.Status, error)
}

// CatalogueSyncScheduler lets connection CRUD maintain the River periodic
// schedule without coupling handlers to River or exposing job credentials.
// The learner-facing Sync now action belongs to the follow-up connection UI.
type CatalogueSyncScheduler interface {
	RegisterConnection(context.Context, string, string) error
	UnregisterConnection(string, string) error
}
type CatalogueMetadataRefresher interface {
	RefreshEntry(context.Context, string, string) (cataloguesync.RefreshResult, error)
}
type CatalogueAcquisitionTargetProvider interface {
	FindAcquisitionTarget(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
}
type CompletedAnalysisReader interface {
	GetCompletedAnalysis(context.Context, string, string, string) (analysis.CompletedAnalysis, error)
}
type AnalysisInsights interface {
	Coverage(context.Context, string, string) (domain.AnalysisCoverage, error)
}
type KnownVocabulary interface {
	Submit(context.Context, string, string, string) (knownvocab.Handle, error)
	Get(context.Context, string, int64) (knownvocab.Status, error)
}
type ExternalEnrichment interface {
	SubmitEnrichment(context.Context, string, []enrichment.Candidate) (enrichmentjob.Handle, error)
	Get(context.Context, string, int64) (enrichmentjob.Status, error)
	Cancel(context.Context, string, int64) (enrichmentjob.Status, error)
}
type PreparedDeck interface {
	Submit(context.Context, string, string, bool) (prepareddeck.Handle, error)
	Get(context.Context, string, string) (domain.DeckPreparation, error)
	Cancel(context.Context, string, string) (domain.DeckPreparation, error)
	Retry(context.Context, string, string, bool) (prepareddeck.Handle, error)
	Rerender(context.Context, string, string) (prepareddeck.Handle, error)
	Download(context.Context, string, string) (domain.DeckPreparation, error)
}
type PreparedDeckForAnalysis interface {
	GetForAnalysis(context.Context, string, string, string) (domain.DeckPreparation, error)
}
type VocabularyStudyStore interface {
	StartDeckVocabularyStudy(context.Context, string, string) (domain.DeckPreparation, error)
	ConfirmDeckVocabularyReview(context.Context, string, string) (domain.DeckPreparation, error)
	ReleaseDeckVocabularyStudy(context.Context, string, string) (domain.DeckPreparation, error)
	CountDeckPreparationVocabularyToGraduate(context.Context, string, string) (int, error)
}
type VocabularyStudyPreparationReader interface {
	GetDeckPreparationForAnalysis(context.Context, string, string, string) (domain.DeckPreparation, error)
	GetActiveDeckVocabularyStudy(context.Context, string, string) (domain.DeckPreparation, error)
	ListDeckPreparationsForSourceMaterial(context.Context, string, string) ([]domain.DeckPreparation, error)
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
	Enrichment       ExternalEnrichment
	PreparedDeck     PreparedDeck
	Capabilities     analyzer.CapabilityProvider
	CatalogueSync    CatalogueSyncScheduler
	SecureCookies    bool
	SessionLifetime  time.Duration
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
	h.mux.Handle("GET /journey", h.user(http.HandlerFunc(h.journey)))
	h.mux.Handle("GET /journey/{bookID}", h.user(http.HandlerFunc(h.journeyEntry)))
	h.mux.Handle("POST /goal/books/{id}", h.user(http.HandlerFunc(h.choosePrimaryGoal)))
	h.mux.Handle("POST /goal/clear", h.user(http.HandlerFunc(h.clearPrimaryGoal)))
	h.mux.Handle("POST /goal/finish", h.user(http.HandlerFunc(h.finishPrimaryGoal)))
	h.mux.Handle("POST /journey/books/{id}/add", h.user(http.HandlerFunc(h.addDeckBookToJourney)))
	h.mux.Handle("POST /journey/books/{id}/reanalyze", h.user(http.HandlerFunc(h.reanalyzeJourneyBook)))
	h.mux.Handle("POST /journey/books/{id}/vocabulary-study", h.user(http.HandlerFunc(h.startBookVocabularyStudy)))
	h.mux.Handle("POST /journey/books/{id}/vocabulary-study/confirm", h.user(http.HandlerFunc(h.confirmBookVocabularyReview)))
	h.mux.Handle("POST /journey/books/{id}/vocabulary-study/release", h.user(http.HandlerFunc(h.releaseBookVocabularyStudy)))
	h.mux.Handle("POST /journey/books/{id}/remove", h.user(http.HandlerFunc(h.removeBookFromReadingJourney)))
	h.mux.Handle("POST /journey/entries/{id}/move-earlier", h.user(http.HandlerFunc(h.moveJourneyEntryEarlier)))
	h.mux.Handle("POST /journey/entries/{id}/move-later", h.user(http.HandlerFunc(h.moveJourneyEntryLater)))
	h.mux.Handle("POST /library/books/{id}/remove", h.user(http.HandlerFunc(h.removeBookFromMyBooks)))
	h.mux.Handle("POST /library/books/{id}/refresh", h.user(http.HandlerFunc(h.refreshBookMetadata)))
	h.mux.Handle("GET /books/{id}/analyses/{runID}", h.user(http.HandlerFunc(h.analysisResult)))
	h.mux.Handle("POST /journey/books/{id}/deck/preparations", h.user(http.HandlerFunc(h.createJourneyEntryDeckPreparation)))
	h.mux.Handle("POST /jobs/{id}/deck/preparations", h.user(http.HandlerFunc(h.createDeckPreparation)))
	h.mux.Handle("GET /deck-preparations/{id}/status", h.user(http.HandlerFunc(h.deckPreparationStatus)))
	h.mux.Handle("POST /deck-preparations/{id}/cancel", h.user(http.HandlerFunc(h.cancelDeckPreparation)))
	h.mux.Handle("POST /deck-preparations/{id}/retry", h.user(http.HandlerFunc(h.retryDeckPreparation)))
	h.mux.Handle("POST /deck-preparations/{id}/rerender", h.user(http.HandlerFunc(h.rerenderDeckPreparation)))
	h.mux.Handle("GET /deck-preparations/{id}/download", h.user(http.HandlerFunc(h.downloadDeckPreparation)))
	h.mux.Handle("GET /enrichment-jobs/{id}/status", h.user(http.HandlerFunc(h.enrichmentJobStatus)))
	h.mux.Handle("POST /enrichment-jobs/{id}/cancel", h.user(http.HandlerFunc(h.cancelEnrichmentJob)))
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
	h.mux.Handle("GET /vocabulary", h.user(http.HandlerFunc(h.vocabularyPage)))
	h.mux.Handle("POST /active-study-language", h.user(http.HandlerFunc(h.activeStudyLanguage)))
	h.mux.Handle("POST /vocabulary/import", h.user(http.HandlerFunc(h.importKnownVocab)))
	h.mux.Handle("GET /vocabulary/imports/{id}/status", h.user(http.HandlerFunc(h.knownVocabImportStatus)))
	h.mux.Handle("GET /known-vocab", h.user(http.HandlerFunc(h.knownVocabPage)))
	return h, nil
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	h.mux.ServeHTTP(w, r)
}
func (h *Handler) user(next http.Handler) http.Handler {
	return h.services.WebAuth.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := user(r)
		view, err := h.loadShellView(r.Context(), u.ID, shellReturnPath(r))
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "unable to render page", 500)
	}
}
func (h *Handler) csrf(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) >= 32 {
		return c.Value
	}
	return h.rotateCSRF(w, r)
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
	http.Redirect(w, r, webauth.SafeReturnPath(path), http.StatusSeeOther) //nolint:gosec // SafeReturnPath rejects external redirect destinations.
}
func user(r *http.Request) domain.User { u, _ := webauth.UserFromContext(r.Context()); return u }
func isHTMX(r *http.Request) bool      { return r.Header.Get("Hx-Request") == "true" }

func fail(w http.ResponseWriter, err error) {
	log.Printf("mouseion: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func renderStatus(w http.ResponseWriter, r *http.Request, status int, component interface {
	Render(context.Context, io.Writer) error
}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "unable to render page", http.StatusInternalServerError)
	}
}
