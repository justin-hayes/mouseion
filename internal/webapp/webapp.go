// Package webapp provides Mouseion's authenticated server-rendered web client.
package webapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
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

const csrfCookie = "mouseion_csrf"

type Store interface {
	PutSupportedLanguage(context.Context, string, string) (domain.SupportedLanguage, error)
	PutLanguageProfile(context.Context, string, string, string) (domain.LanguageProfile, error)
	ListLanguageProfiles(context.Context, string) ([]domain.LanguageProfile, error)
	DeleteLanguageProfile(context.Context, string, string) error
	CreateOpdsConnection(context.Context, string, domain.OpdsConnection) (domain.OpdsConnection, error)
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
	ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error)
	UpdateOpdsConnection(context.Context, string, domain.OpdsConnection) (domain.OpdsConnection, error)
	DeleteOpdsConnection(context.Context, string, string) error
	ListSourceMaterials(context.Context, string) ([]domain.SourceMaterialSummary, error)
	ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
	ListLearningCampaigns(context.Context, string) ([]domain.LearningCampaign, error)
	GetLearningCampaign(context.Context, string, string) (domain.LearningCampaign, error)
	CreateLearningCampaign(context.Context, string, string, string) (domain.LearningCampaign, error)
	UpdateLearningCampaignProgress(context.Context, string, string, persistence.LearningCampaignExpectedState, domain.BookProgress, domain.DeckProgress) (domain.LearningCampaign, error)
	AbandonLearningCampaign(context.Context, string, string, persistence.LearningCampaignExpectedState) (domain.LearningCampaign, error)
	ListUnassignedReadyDeckPreparations(context.Context, string) ([]domain.DeckPreparation, error)
	GetSourceMaterial(context.Context, string, string) (domain.SourceMaterial, error)
	GetExtractedUnitSnapshot(context.Context, string, string) (string, domain.ExtractedUnits, error)
	GetEPUBUnitClassifications(context.Context, string, string, string, string) ([]domain.EPUBUnitClassification, error)
	GetEPUBReviewedScope(context.Context, string, string, string) (domain.EPUBReviewedScopeSnapshot, error)
	CreateEPUBReviewedScope(context.Context, domain.EPUBReviewedScopeSnapshot) (domain.EPUBReviewedScopeSnapshot, error)
}
type OPDS interface {
	Browse(context.Context, string, string, string) (opds.Feed, error)
	BrowsePage(context.Context, string, string, string) (opds.Feed, error)
	Languages(context.Context, string, string) (opds.Feed, error)
	BrowseLanguage(context.Context, string, string, string) (opds.Feed, error)
	BrowseLanguagePage(context.Context, string, string, string, string) (opds.Feed, error)
	Search(context.Context, string, string, string) (opds.Feed, error)
	SearchPage(context.Context, string, string, string, string) (opds.Feed, error)
	Acquire(context.Context, string, string, string, opds.Entry) (epub.ImportResult, error)
}
type Analysis interface {
	SubmitAnalysis(context.Context, string, string) (analysis.Handle, error)
	SubmitScopedAnalysis(context.Context, string, string, string) (analysis.Handle, error)
	Get(context.Context, string, int64) (analysis.Status, error)
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
	Download(context.Context, string, string) (domain.DeckPreparation, error)
}
type PreparedDeckForAnalysis interface {
	GetForAnalysis(context.Context, string, string, string) (domain.DeckPreparation, error)
}

// Services keeps UI dependencies explicit and makes web-level tests independent of infrastructure.
type Services struct {
	Auth             *auth.Service
	WebAuth          *webauth.Handler
	Store            Store
	OPDS             OPDS
	Analysis         Analysis
	AnalysisInsights AnalysisInsights
	KnownVocab       KnownVocabulary
	Enrichment       ExternalEnrichment
	PreparedDeck     PreparedDeck
	Capabilities     analyzer.CapabilityProvider
	SecureCookies    bool
	SessionLifetime  time.Duration
	// These are optional explicit key injections for deterministic tests or
	// another deliberately managed key provider. Production uses MOUSEION_SECRET.
	AcquisitionKey       []byte
	AcquisitionTargetKey []byte
}

type Handler struct {
	services       Services
	mux            *http.ServeMux
	acquisitionKey []byte
	targetKey      []byte
}

func New(s Services) *Handler {
	h, err := NewWithError(s)
	if err != nil {
		panic(err)
	}
	return h
}

func NewWithError(s Services) (*Handler, error) {
	cookieKey, targetKey, err := webKeys(s)
	if err != nil {
		return nil, err
	}
	h := &Handler{services: s, mux: http.NewServeMux(), acquisitionKey: cookieKey, targetKey: targetKey}
	h.mux.HandleFunc("GET /login", h.loginPage)
	h.mux.HandleFunc("POST /login", h.login)
	h.mux.HandleFunc("POST /onboarding", h.onboard)
	h.mux.Handle("POST /logout-all", h.user(http.HandlerFunc(h.logoutAll)))
	h.mux.Handle("GET /{$}", h.user(http.HandlerFunc(h.dashboard)))
	h.mux.Handle("GET /library", h.user(http.HandlerFunc(h.library)))
	h.mux.Handle("GET /campaigns", h.user(http.HandlerFunc(h.campaigns)))
	h.mux.Handle("POST /campaigns", h.user(http.HandlerFunc(h.queueCampaign)))
	h.mux.Handle("POST /campaigns/{id}/activate", h.user(http.HandlerFunc(h.activateCampaign)))
	h.mux.Handle("POST /campaigns/{id}/book-finished", h.user(http.HandlerFunc(h.finishCampaignBook)))
	h.mux.Handle("POST /campaigns/{id}/deck-reviewed", h.user(http.HandlerFunc(h.reviewCampaignDeck)))
	h.mux.Handle("POST /campaigns/{id}/abandon", h.user(http.HandlerFunc(h.abandonCampaign)))
	h.mux.Handle("GET /books/{id}", h.user(http.HandlerFunc(h.book)))
	h.mux.Handle("GET /books/{id}/analyses/{runID}", h.user(http.HandlerFunc(h.analysisResult)))
	h.mux.Handle("POST /books/{id}/analyses/{runID}/deck/preparations", h.user(http.HandlerFunc(h.createAnalysisDeckPreparation)))
	h.mux.Handle("GET /books/{id}/scope", h.user(http.HandlerFunc(h.reviewEPUBScope)))
	h.mux.Handle("POST /books/{id}/scope", h.user(http.HandlerFunc(h.confirmEPUBScope)))
	h.mux.Handle("POST /books/{id}/analyze", h.user(http.HandlerFunc(h.analyzeBook)))
	h.mux.Handle("POST /jobs/{id}/deck/preparations", h.user(http.HandlerFunc(h.createDeckPreparation)))
	h.mux.Handle("GET /deck-preparations/{id}/status", h.user(http.HandlerFunc(h.deckPreparationStatus)))
	h.mux.Handle("POST /deck-preparations/{id}/cancel", h.user(http.HandlerFunc(h.cancelDeckPreparation)))
	h.mux.Handle("POST /deck-preparations/{id}/retry", h.user(http.HandlerFunc(h.retryDeckPreparation)))
	h.mux.Handle("GET /deck-preparations/{id}/download", h.user(http.HandlerFunc(h.downloadDeckPreparation)))
	h.mux.Handle("GET /enrichment-jobs/{id}/status", h.user(http.HandlerFunc(h.enrichmentJobStatus)))
	h.mux.Handle("POST /enrichment-jobs/{id}/cancel", h.user(http.HandlerFunc(h.cancelEnrichmentJob)))
	h.mux.Handle("POST /logout", h.user(http.HandlerFunc(h.logout)))
	h.mux.Handle("GET /settings", h.user(http.HandlerFunc(h.settings)))
	h.mux.Handle("POST /settings/languages", h.user(http.HandlerFunc(h.addStudyLanguage)))
	h.mux.Handle("POST /settings/languages/remove", h.user(http.HandlerFunc(h.removeStudyLanguage)))
	h.mux.Handle("GET /connections", h.user(http.HandlerFunc(h.connections)))
	h.mux.Handle("POST /connections", h.user(http.HandlerFunc(h.createConnection)))
	h.mux.Handle("POST /connections/{id}", h.user(http.HandlerFunc(h.updateConnection)))
	h.mux.Handle("POST /connections/{id}/delete", h.user(http.HandlerFunc(h.deleteConnection)))
	h.mux.Handle("GET /catalog", h.user(http.HandlerFunc(h.catalog)))
	h.mux.Handle("GET /opds/browse", h.user(http.HandlerFunc(h.browse)))
	h.mux.Handle("GET /opds/language", h.user(http.HandlerFunc(h.browseLanguage)))
	h.mux.Handle("GET /opds/search", h.user(http.HandlerFunc(h.search)))
	h.mux.Handle("POST /opds/acquire", h.user(http.HandlerFunc(h.acquire)))
	h.mux.Handle("GET /jobs", h.user(http.HandlerFunc(h.jobs)))
	h.mux.Handle("GET /jobs/{id}", h.user(http.HandlerFunc(h.job)))
	h.mux.Handle("GET /jobs/{id}/status", h.user(http.HandlerFunc(h.jobStatus)))
	h.mux.Handle("POST /jobs/{id}/retry", h.user(http.HandlerFunc(h.retryJob)))
	h.mux.Handle("POST /jobs/{id}/cancel", h.user(http.HandlerFunc(h.cancelJob)))
	h.mux.Handle("GET /known-vocab", h.user(http.HandlerFunc(h.knownVocabPage)))
	h.mux.Handle("POST /known-vocab/import", h.user(http.HandlerFunc(h.importKnownVocab)))
	h.mux.Handle("GET /known-vocab/imports/{id}/status", h.user(http.HandlerFunc(h.knownVocabImportStatus)))
	return h, nil
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }
func (h *Handler) user(next http.Handler) http.Handler              { return h.services.WebAuth.RequireUser(next) }
func render(w http.ResponseWriter, r *http.Request, component interface {
	Render(context.Context, io.Writer) error
}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "unable to render page", 500)
	}
}
func (h *Handler) csrf(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) >= 32 {
		return c.Value
	}
	return h.rotateCSRF(w)
}
func (h *Handler) rotateCSRF(w http.ResponseWriter) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
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
	http.SetCookie(w, &http.Cookie{Name: webauth.CookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.services.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(h.services.SessionLifetime.Seconds())})
}
func (h *Handler) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: webauth.CookieName, Path: "/", HttpOnly: true, Secure: h.services.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, path, http.StatusSeeOther)
}
func user(r *http.Request) domain.User { u, _ := webauth.UserFromContext(r.Context()); return u }
func isHTMX(r *http.Request) bool      { return r.Header.Get("HX-Request") == "true" }

func webKeyFromSecret(secret, purpose string) ([]byte, error) {
	if err := persistence.ValidateSecret(secret); err != nil {
		return nil, fmt.Errorf("webapp: initialize %s key: %w", purpose, err)
	}
	sum := sha256.Sum256([]byte("mouseion-webapp-" + purpose + "-v1\x00" + secret))
	return sum[:], nil
}

func webKeys(s Services) ([]byte, []byte, error) {
	if len(s.AcquisitionKey) > 0 || len(s.AcquisitionTargetKey) > 0 {
		if len(s.AcquisitionKey) != 32 || len(s.AcquisitionTargetKey) != 32 {
			return nil, nil, errors.New("webapp: explicitly injected acquisition keys must each be 32 bytes")
		}
		return append([]byte(nil), s.AcquisitionKey...), append([]byte(nil), s.AcquisitionTargetKey...), nil
	}
	cookieKey, err := webKeyFromSecret(os.Getenv("MOUSEION_SECRET"), "cookie")
	if err != nil {
		return nil, nil, err
	}
	targetKey, err := webKeyFromSecret(os.Getenv("MOUSEION_SECRET"), "target")
	if err != nil {
		return nil, nil, err
	}
	return cookieKey, targetKey, nil
}

func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	hasUsers, err := h.services.Auth.HasUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, LoginPage(h.csrf(w, r), r.URL.Query().Get("error"), webauth.SafeReturnPath(r.URL.Query().Get("next")), !hasUsers))
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		h.services.WebAuth.ServeHTTP(w, r)
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	token, err := h.services.Auth.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		query := url.Values{"error": {"Invalid credentials"}, "next": {webauth.SafeReturnPath(r.FormValue("next"))}}
		redirect(w, r, "/login?"+query.Encode())
		return
	}
	h.setSession(w, token)
	h.rotateCSRF(w)
	redirect(w, r, webauth.SafeReturnPath(r.FormValue("next")))
}
func (h *Handler) onboard(w http.ResponseWriter, r *http.Request) {
	hasUsers, err := h.services.Auth.HasUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if hasUsers {
		http.NotFound(w, r)
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	_, token, err := h.services.Auth.CreateFirstAccount(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if errors.Is(err, auth.ErrFirstAccountExists) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		redirect(w, r, "/login?error="+url.QueryEscape(err.Error()))
		return
	}
	h.setSession(w, token)
	h.rotateCSRF(w)
	redirect(w, r, "/")
}
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if c, err := r.Cookie(webauth.CookieName); err == nil {
			_ = h.services.Auth.Logout(r.Context(), c.Value)
		}
		h.clearSession(w)
		h.clearAcquisition(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	if c, err := r.Cookie(webauth.CookieName); err == nil {
		_ = h.services.Auth.Logout(r.Context(), c.Value)
	}
	h.clearSession(w)
	h.clearAcquisition(w)
	redirect(w, r, "/login")
}
func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := h.services.Auth.LogoutEverywhere(r.Context(), user(r).ID); err != nil {
			http.Error(w, "unable to invalidate sessions", http.StatusInternalServerError)
			return
		}
		h.clearSession(w)
		h.clearAcquisition(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	if err := h.services.Auth.LogoutEverywhere(r.Context(), user(r).ID); err != nil {
		fail(w, err)
		return
	}
	h.clearSession(w)
	h.clearAcquisition(w)
	redirect(w, r, "/login")
}
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
func (h *Handler) jobs(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	jobs, e := h.services.Store.ListAnalysisJobs(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, JobsPage(u, h.csrf(w, r), jobs, r.URL.Query().Get("message")))
}
func (h *Handler) job(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	status, ok := h.loadJob(w, r, u.ID)
	if !ok {
		return
	}
	render(w, r, JobPage(u, h.csrf(w, r), status))
}
func (h *Handler) jobStatus(w http.ResponseWriter, r *http.Request) {
	status, ok := h.loadJob(w, r, user(r).ID)
	if !ok {
		return
	}
	render(w, r, JobStatus(h.csrf(w, r), status))
}
func (h *Handler) loadJob(w http.ResponseWriter, r *http.Request, owner string) (analysis.Status, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return analysis.Status{}, false
	}
	if lifecycle, ok := h.services.Analysis.(interface {
		Reconcile(context.Context, string, int64) (analysis.Status, error)
	}); ok {
		if _, reconcileErr := lifecycle.Reconcile(r.Context(), owner, id); reconcileErr != nil && !errors.Is(reconcileErr, analysis.ErrNotFound) {
			// Status reads remain useful even if a best-effort reconciliation is unavailable.
		}
	}
	status, err := h.services.Analysis.Get(r.Context(), owner, id)
	if errors.Is(err, analysis.ErrNotFound) {
		http.NotFound(w, r)
		return analysis.Status{}, false
	}
	if err != nil {
		fail(w, err)
		return analysis.Status{}, false
	}
	return status, true
}

func (h *Handler) retryJob(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	lifecycle, ok := h.services.Analysis.(interface {
		Retry(context.Context, string, int64) (analysis.Handle, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, err = lifecycle.Retry(r.Context(), user(r).ID, id); err != nil {
		if errors.Is(err, analysis.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		redirect(w, r, "/jobs/"+r.PathValue("id")+"?error="+url.QueryEscape("This analysis is not available for retry."))
		return
	}
	redirect(w, r, "/jobs/"+r.PathValue("id")+"?message="+url.QueryEscape("Analysis retry submitted."))
}

func (h *Handler) cancelJob(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	lifecycle, ok := h.services.Analysis.(interface {
		Cancel(context.Context, string, int64) (analysis.Status, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, err = lifecycle.Cancel(r.Context(), user(r).ID, id); err != nil {
		if errors.Is(err, analysis.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		redirect(w, r, "/jobs/"+r.PathValue("id")+"?error="+url.QueryEscape("This analysis could not be cancelled."))
		return
	}
	redirect(w, r, "/jobs/"+r.PathValue("id")+"?message="+url.QueryEscape("Analysis cancelled."))
}
func (h *Handler) createDeckPreparation(w http.ResponseWriter, r *http.Request) {
	h.createDeckPreparationForAnalysis(w, r, r.PathValue("id"), "")
}

func (h *Handler) createAnalysisDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	reader, ok := h.services.Analysis.(CompletedAnalysisReader)
	if !ok {
		http.NotFound(w, r)
		return
	}
	u := user(r)
	result, err := reader.GetCompletedAnalysis(r.Context(), u.ID, r.PathValue("id"), r.PathValue("runID"))
	if errors.Is(err, analysis.ErrNotFound) || errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if result.OwnerID != u.ID || result.SourceMaterialID != r.PathValue("id") || result.RunID != r.PathValue("runID") || result.ScopeID == "" {
		http.NotFound(w, r)
		return
	}
	h.submitDeckPreparation(w, r, result.RunID, result.SourceMaterialID)
}

func (h *Handler) createDeckPreparationForAnalysis(w http.ResponseWriter, r *http.Request, analysisID, sourceMaterialID string) {
	if !h.checkCSRF(w, r) {
		return
	}
	h.submitDeckPreparation(w, r, analysisID, sourceMaterialID)
}

func (h *Handler) submitDeckPreparation(w http.ResponseWriter, r *http.Request, analysisID, sourceMaterialID string) {
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	handle, err := h.services.PreparedDeck.Submit(r.Context(), user(r).ID, analysisID, r.FormValue("external_translation_consent") == "on")
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if sourceMaterialID != "" && (handle.Preparation.SourceMaterialID != sourceMaterialID || handle.Preparation.AnalysisRunID != analysisID) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Location", "/deck-preparations/"+url.PathEscape(handle.Preparation.ID)+"/status")
	w.WriteHeader(http.StatusSeeOther)
}

type deckPreparationResponse struct {
	ID            string                      `json:"id"`
	State         domain.DeckPreparationState `json:"state"`
	Phase         string                      `json:"phase,omitempty"`
	Progress      int                         `json:"progress"`
	Ready         bool                        `json:"ready"`
	Error         string                      `json:"error,omitempty"`
	FailureClass  string                      `json:"failure_class,omitempty"`
	AnalysisRunID string                      `json:"analysis_run_id,omitempty"`
	Filename      string                      `json:"filename"`
	DeckName      string                      `json:"deck_name"`
	DownloadURL   string                      `json:"download_url,omitempty"`
	Completeness  deckCompletenessResponse    `json:"completeness"`
	Translation   deckTranslationResponse     `json:"translation"`
	Batch         deckBatchResponse           `json:"batch"`
}

type deckCompletenessResponse struct {
	TotalCards               int `json:"total_cards"`
	CardsWithEnglish         int `json:"cards_with_english"`
	CardsWithEnglishSentence int `json:"cards_with_contextual_sentence_translations"`
	QualityOmissions         int `json:"quality_omissions"`
}

type deckTranslationResponse struct {
	Eligible  int `json:"eligible"`
	Completed int `json:"completed"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Retrying  int `json:"retrying"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

type deckBatchResponse struct {
	AgeSeconds        int64 `json:"age_seconds"`
	Chunks            int   `json:"chunks"`
	SubmittedChunks   int   `json:"submitted_chunks"`
	PollingChunks     int   `json:"polling_chunks"`
	ReconcilingChunks int   `json:"reconciling_chunks"`
	CompletedChunks   int   `json:"completed_chunks"`
	FailedChunks      int   `json:"failed_chunks"`
	CancelledChunks   int   `json:"cancelled_chunks"`
	Requests          int   `json:"requests"`
	Completed         int   `json:"completed"`
	Failed            int   `json:"failed"`
	Expired           int   `json:"expired"`
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
}

func preparationResponse(p domain.DeckPreparation) deckPreparationResponse {
	errorMessage := ""
	if p.State == domain.DeckPreparationFailed {
		errorMessage = preparationFailureMessage(p.FailureClass)
	}
	response := deckPreparationResponse{ID: p.ID, State: p.State, Phase: p.Phase, Progress: preparationProgress(p), Ready: p.State == domain.DeckPreparationReady, Error: errorMessage, FailureClass: p.FailureClass, AnalysisRunID: p.AnalysisRunID, Filename: p.Filename, DeckName: p.DeckName, Completeness: deckCompletenessResponse{TotalCards: p.TotalCards, CardsWithEnglish: p.CardsWithEnglish, CardsWithEnglishSentence: p.CardsWithContextualSentenceTranslations, QualityOmissions: p.QualityOmissions}, Translation: deckTranslationResponse{Eligible: p.TranslationEligible, Completed: p.TranslationDone, Pending: p.TranslationPending, Running: p.TranslationRunning, Retrying: p.TranslationRetrying, Failed: p.TranslationFailed, Cancelled: p.TranslationCancelled}, Batch: deckBatchResponse{AgeSeconds: int64(p.BatchAge / time.Second), Chunks: p.BatchChunkCount, SubmittedChunks: p.BatchSubmittedChunks, PollingChunks: p.BatchPollingChunks, ReconcilingChunks: p.BatchReconcilingChunks, CompletedChunks: p.BatchCompletedChunks, FailedChunks: p.BatchFailedChunks, CancelledChunks: p.BatchCancelledChunks, Requests: p.BatchRequestCount, Completed: p.BatchCompletedRequests, Failed: p.BatchFailedRequests, Expired: p.BatchExpiredRequests, InputTokens: p.BatchInputTokens, OutputTokens: p.BatchOutputTokens}}
	if response.Ready {
		response.DownloadURL = "/deck-preparations/" + url.PathEscape(p.ID) + "/download"
	}
	return response
}

func preparationFailureMessage(class string) string {
	switch class {
	case "configuration", "unsupported_model":
		return "External translation is not available for this preparation. Check the provider configuration and retry."
	case "ambiguous_submission":
		return "Translation submission could not be confirmed safely. Retry the preparation later."
	case "validation", "malformed_result", "missing_result", "duplicate_result", "unknown_result", "reconciliation":
		return "Translation results could not be verified. Retry the preparation."
	case "provider", "upload", "poll":
		return "The translation provider is temporarily unavailable. Retry the preparation later."
	case "cancelled":
		return ""
	default:
		return "Deck preparation could not be completed. Retry the preparation."
	}
}

func writePreparationStatus(w http.ResponseWriter, p domain.DeckPreparation) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(preparationResponse(p))
}

func (h *Handler) deckPreparationStatus(w http.ResponseWriter, r *http.Request) {
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.services.PreparedDeck.Get(r.Context(), user(r).ID, r.PathValue("id"))
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if wantsPreparationJSON(r) {
		writePreparationStatus(w, p)
		return
	}
	resultURL := preparationResultURL(p)
	if r.Header.Get("HX-Request") == "true" {
		render(w, r, DeckPreparationStatus(h.csrf(w, r), p, resultURL))
		return
	}
	render(w, r, DeckPreparationStatusPage(user(r), h.csrf(w, r), p, resultURL))
}

func (h *Handler) cancelDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.services.PreparedDeck.Cancel(r.Context(), user(r).ID, r.PathValue("id"))
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if wantsPreparationJSON(r) {
		writePreparationStatus(w, p)
		return
	}
	h.redirectToPreparationStatus(w, r)
}

func (h *Handler) retryDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	handle, err := h.services.PreparedDeck.Retry(r.Context(), user(r).ID, r.PathValue("id"), r.FormValue("external_translation_consent") == "on")
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	if wantsPreparationJSON(r) {
		writePreparationStatus(w, handle.Preparation)
		return
	}
	h.redirectToPreparationStatus(w, r)
}

func wantsPreparationJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Query().Get("format") == "json"
}

func (h *Handler) redirectToPreparationStatus(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/deck-preparations/"+url.PathEscape(r.PathValue("id"))+"/status", http.StatusSeeOther)
}

func preparationResultURL(p domain.DeckPreparation) string {
	if p.SourceMaterialID == "" || p.AnalysisRunID == "" {
		return ""
	}
	return "/books/" + url.PathEscape(p.SourceMaterialID) + "/analyses/" + url.PathEscape(p.AnalysisRunID)
}

func (h *Handler) downloadDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.services.PreparedDeck.Download(r.Context(), user(r).ID, r.PathValue("id"))
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.anki")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": p.Filename}))
	w.Header().Set("X-Mouseion-Deck-Name", p.DeckName)
	w.Header().Set("X-Mouseion-Analysis-Run-ID", p.AnalysisRunID)
	w.Header().Set("X-Mouseion-Cards-Total", strconv.Itoa(p.TotalCards))
	w.Header().Set("X-Mouseion-Cards-With-English", strconv.Itoa(p.CardsWithEnglish))
	w.Header().Set("X-Mouseion-Cards-With-English-Sentence", strconv.Itoa(p.CardsWithContextualSentenceTranslations))
	w.Header().Set("X-Mouseion-Cards-Quality-Omitted", strconv.Itoa(p.QualityOmissions))
	_, _ = w.Write(p.Artifact)
}

func handlePreparationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, prepareddeck.ErrAnalysisUnavailable), errors.Is(err, prepareddeck.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, persistence.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, persistence.ErrInvalidTransition):
		http.Error(w, "invalid deck preparation state", http.StatusConflict)
	default:
		fail(w, err)
	}
}

func (h *Handler) enrichmentJobStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || h.services.Enrichment == nil {
		http.NotFound(w, r)
		return
	}
	status, err := h.services.Enrichment.Get(r.Context(), user(r).ID, id)
	if errors.Is(err, enrichmentjob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, EnrichmentJobStatus(status, h.csrf(w, r)))
}

func (h *Handler) cancelEnrichmentJob(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || h.services.Enrichment == nil {
		http.NotFound(w, r)
		return
	}
	status, err := h.services.Enrichment.Cancel(r.Context(), user(r).ID, id)
	if errors.Is(err, enrichmentjob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, EnrichmentJobStatus(status, h.csrf(w, r)))
}
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
func jobRunning(status analysis.Status) bool {
	if status.LogicalState != "" {
		return status.LogicalState == "queued" || status.LogicalState == "running"
	}
	return status.State == "available" || status.State == "pending" || status.State == "running" || status.State == "retryable" || status.State == "scheduled"
}
func jobState(status analysis.Status) string {
	if status.LogicalState != "" {
		switch status.LogicalState {
		case "completed":
			return "Completed"
		case "failed":
			return "Failed"
		case "cancelled":
			return "Cancelled"
		case "running":
			return "Running"
		default:
			return "Queued"
		}
	}
	switch status.State {
	case "completed":
		return "Succeeded"
	case "discarded", "cancelled":
		return "Failed"
	case "running":
		return "Running"
	default:
		return strings.Title(string(status.State))
	}
}
func analysisStatusSummary(status analysis.Status) string {
	switch status.LogicalState {
	case "completed":
		return "Analysis complete. Open the exact result to review its insights."
	case "failed":
		return "Analysis failed. Review the message and retry the confirmed scope when it is still valid."
	case "cancelled":
		return "Analysis cancelled. Retry the confirmed scope when you are ready."
	}
	attempt := maxOne(status.Attempt)
	return fmt.Sprintf("%d%% complete · attempt %d", status.Progress, attempt)
}
func jobRetryable(status analysis.Status) bool {
	return status.LogicalState == "failed" || status.LogicalState == "cancelled"
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func statusClass(status string) string { return strings.ReplaceAll(status, " ", "-") }

func enrichmentJobLabel(status string) string {
	switch status {
	case "available", "scheduled", "retryable", "pending":
		return "Queued"
	case "running":
		return "Running"
	case "completed":
		return "Completed"
	case "cancelled":
		return "Cancelled"
	case "discarded":
		return "Discarded"
	default:
		return status
	}
}
