// Package webapp provides Mouseion's authenticated server-rendered web client.
package webapp

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
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
	"github.com/riverqueue/river/rivertype"
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
	UpdateLearningCampaignProgress(context.Context, string, string, domain.BookProgress, domain.DeckProgress) (domain.LearningCampaign, error)
	AbandonLearningCampaign(context.Context, string, string) (domain.LearningCampaign, error)
	ListUnassignedReadyDeckPreparations(context.Context, string) ([]domain.DeckPreparation, error)
	GetSourceMaterial(context.Context, string, string) (domain.SourceMaterial, error)
	GetExtractedUnitSnapshot(context.Context, string, string) (string, domain.ExtractedUnits, error)
	GetEPUBUnitClassifications(context.Context, string, string, string, string) ([]domain.EPUBUnitClassification, error)
	GetEPUBReviewedScope(context.Context, string, string, string) (domain.EPUBReviewedScopeSnapshot, error)
	CreateEPUBReviewedScope(context.Context, domain.EPUBReviewedScopeSnapshot) (domain.EPUBReviewedScopeSnapshot, error)
}
type OPDS interface {
	Browse(context.Context, string, string, string) (opds.Feed, error)
	Languages(context.Context, string, string) (opds.Feed, error)
	BrowseLanguage(context.Context, string, string, string) (opds.Feed, error)
	Search(context.Context, string, string, string) (opds.Feed, error)
	Acquire(context.Context, string, string, string, opds.Entry) (epub.ImportResult, error)
}
type Analysis interface {
	SubmitAnalysis(context.Context, string, string) (analysis.Handle, error)
	SubmitScopedAnalysis(context.Context, string, string, string) (analysis.Handle, error)
	Get(context.Context, string, int64) (analysis.Status, error)
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
}

type Handler struct {
	services Services
	mux      *http.ServeMux
}

func New(s Services) *Handler {
	h := &Handler{services: s, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /login", h.loginPage)
	h.mux.HandleFunc("POST /login", h.login)
	h.mux.HandleFunc("POST /onboarding", h.onboard)
	h.mux.Handle("POST /logout-all", s.WebAuth)
	h.mux.Handle("GET /{$}", h.user(http.HandlerFunc(h.dashboard)))
	h.mux.Handle("GET /library", h.user(http.HandlerFunc(h.library)))
	h.mux.Handle("GET /campaigns", h.user(http.HandlerFunc(h.campaigns)))
	h.mux.Handle("POST /campaigns", h.user(http.HandlerFunc(h.queueCampaign)))
	h.mux.Handle("POST /campaigns/{id}/activate", h.user(http.HandlerFunc(h.activateCampaign)))
	h.mux.Handle("POST /campaigns/{id}/book-finished", h.user(http.HandlerFunc(h.finishCampaignBook)))
	h.mux.Handle("POST /campaigns/{id}/deck-reviewed", h.user(http.HandlerFunc(h.reviewCampaignDeck)))
	h.mux.Handle("POST /campaigns/{id}/abandon", h.user(http.HandlerFunc(h.abandonCampaign)))
	h.mux.Handle("GET /books/{id}", h.user(http.HandlerFunc(h.book)))
	h.mux.Handle("GET /books/{id}/scope", h.user(http.HandlerFunc(h.reviewEPUBScope)))
	h.mux.Handle("POST /books/{id}/scope", h.user(http.HandlerFunc(h.confirmEPUBScope)))
	h.mux.Handle("POST /books/{id}/analyze", h.user(http.HandlerFunc(h.analyzeBook)))
	h.mux.Handle("POST /books/{id}/deck/preparations", h.user(http.HandlerFunc(h.createDeckPreparation)))
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
	h.mux.Handle("GET /known-vocab", h.user(http.HandlerFunc(h.knownVocabPage)))
	h.mux.Handle("POST /known-vocab/import", h.user(http.HandlerFunc(h.importKnownVocab)))
	h.mux.Handle("GET /known-vocab/imports/{id}/status", h.user(http.HandlerFunc(h.knownVocabImportStatus)))
	return h
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
		h.services.WebAuth.ServeHTTP(w, r)
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	if c, err := r.Cookie(webauth.CookieName); err == nil {
		_ = h.services.Auth.Logout(r.Context(), c.Value)
	}
	h.clearSession(w)
	redirect(w, r, "/login")
}
func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/library")
}
func (h *Handler) library(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	books, err := h.services.Store.ListSourceMaterials(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, LibraryPage(u, h.csrf(w, r), books, r.URL.Query().Get("message")))
}

type campaignView struct {
	Campaign domain.LearningCampaign
	Book     domain.SourceMaterialSummary
	Deck     domain.DeckPreparation
	Coverage *domain.AnalysisCoverage
}

type preparedCampaignOption struct {
	Book domain.SourceMaterialSummary
	Deck domain.DeckPreparation
}

func (h *Handler) campaigns(w http.ResponseWriter, r *http.Request) {
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	u := user(r)
	campaigns, err := h.services.Store.ListLearningCampaigns(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	books, err := h.services.Store.ListSourceMaterials(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	bookByID := make(map[string]domain.SourceMaterialSummary, len(books))
	for _, book := range books {
		bookByID[book.Source.ID] = book
	}
	views := make([]campaignView, 0, len(campaigns))
	for _, campaign := range campaigns {
		deck, deckErr := h.services.PreparedDeck.Get(r.Context(), u.ID, campaign.DeckPreparationID)
		if deckErr != nil {
			fail(w, deckErr)
			return
		}
		book := bookByID[campaign.SourceMaterialID]
		view := campaignView{Campaign: campaign, Book: book, Deck: deck}
		if campaign.Status == domain.CampaignQueued && book.AnalysisStatus == "analyzed" && h.services.AnalysisInsights != nil {
			coverage, coverageErr := h.services.AnalysisInsights.Coverage(r.Context(), u.ID, book.CorpusID)
			if coverageErr != nil && !errors.Is(coverageErr, analysisinsights.ErrStatisticsUnavailable) {
				fail(w, coverageErr)
				return
			}
			if coverageErr == nil {
				view.Coverage = &coverage
			}
		}
		views = append(views, view)
	}
	ready, err := h.services.Store.ListUnassignedReadyDeckPreparations(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	options := make([]preparedCampaignOption, 0, len(ready))
	for _, deck := range ready {
		if book, ok := bookByID[deck.SourceMaterialID]; ok {
			options = append(options, preparedCampaignOption{Book: book, Deck: deck})
		}
	}
	render(w, r, CampaignsPage(u, h.csrf(w, r), views, options, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
}

func (h *Handler) queueCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	u := user(r)
	deck, err := h.services.PreparedDeck.Get(r.Context(), u.ID, r.FormValue("deck_preparation_id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if _, err = h.services.Store.CreateLearningCampaign(r.Context(), u.ID, deck.SourceMaterialID, deck.ID); err != nil {
		if errors.Is(err, persistence.ErrInvalidTransition) {
			redirect(w, r, "/campaigns?error="+url.QueryEscape("Only a ready, unassigned deck can be added to the queue."))
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/campaigns?message="+url.QueryEscape("Book and deck added to your learning queue."))
}

func (h *Handler) activateCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	_, err := h.services.Store.UpdateLearningCampaignProgress(r.Context(), user(r).ID, r.PathValue("id"), domain.BookReading, domain.DeckStudying)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, persistence.ErrActiveCampaign) {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Finish or abandon the active campaign before starting another."))
		return
	}
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only a queued campaign can be started."))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/campaigns?message="+url.QueryEscape("Learning campaign started."))
}

func (h *Handler) finishCampaignBook(w http.ResponseWriter, r *http.Request) {
	h.updateActiveCampaignProgress(w, r, true)
}

func (h *Handler) reviewCampaignDeck(w http.ResponseWriter, r *http.Request) {
	h.updateActiveCampaignProgress(w, r, false)
}

func (h *Handler) abandonCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	_, err := h.services.Store.AbandonLearningCampaign(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only the active campaign can be abandoned."))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/campaigns?message="+url.QueryEscape("Campaign abandoned. Its ungraduated vocabulary is available again."))
}

func (h *Handler) updateActiveCampaignProgress(w http.ResponseWriter, r *http.Request, finishBook bool) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	campaign, err := h.services.Store.GetLearningCampaign(r.Context(), u.ID, r.PathValue("id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if campaign.Status != domain.CampaignActive {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Only the active campaign can be updated."))
		return
	}
	nextBook, nextDeck := campaign.BookProgress, campaign.DeckProgress
	message := "Deck marked reviewed."
	if finishBook {
		nextBook = domain.BookFinished
		message = "Book marked finished."
	} else {
		nextDeck = domain.DeckReviewed
	}
	updated, err := h.services.Store.UpdateLearningCampaignProgress(r.Context(), u.ID, campaign.ID, nextBook, nextDeck)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirect(w, r, "/campaigns?error="+url.QueryEscape("Campaign progress could not be updated."))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if updated.Status == domain.CampaignComplete {
		message = "Campaign complete. Its vocabulary is now known."
	}
	redirect(w, r, "/campaigns?message="+url.QueryEscape(message))
}
func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	summary, ok := h.loadBook(w, r, u.ID)
	if !ok {
		return
	}
	var coverage *domain.AnalysisCoverage
	statisticsUnavailable := false
	if summary.AnalysisStatus == "analyzed" && h.services.AnalysisInsights != nil {
		value, err := h.services.AnalysisInsights.Coverage(r.Context(), u.ID, summary.CorpusID)
		if errors.Is(err, analysisinsights.ErrStatisticsUnavailable) {
			statisticsUnavailable = true
		} else if err != nil {
			fail(w, err)
			return
		} else {
			coverage = &value
		}
	}
	render(w, r, BookPage(u, h.csrf(w, r), summary, coverage, statisticsUnavailable, r.URL.Query().Get("message")))
}

type epubScopeUnitView struct {
	Unit           domain.ExtractedUnit
	Classification domain.EPUBUnitClassification
	CharacterCount int
	TokenEstimate  int
}

type epubScopeView struct {
	Book       domain.SourceMaterial
	SnapshotID string
	Units      []epubScopeUnitView
	Groups     []epubScopeGroupView
	PriorScope *domain.EPUBReviewedScopeSnapshot
	Preset     string
	Comparison epubScopeComparison
}

type epubScopeComparison struct {
	Old, New, Added, Removed                           []epubScopeUnitView
	OldCharacters, OldTokens, NewCharacters, NewTokens int
}

type epubScopeGroupView struct {
	Group          epub.UnitGroup
	CharacterCount int
	TokenEstimate  int
}

func (h *Handler) loadEPUBScope(w http.ResponseWriter, r *http.Request, owner string) (epubScopeView, bool) {
	book, err := h.services.Store.GetSourceMaterial(r.Context(), owner, r.PathValue("id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return epubScopeView{}, false
	}
	if err != nil {
		fail(w, err)
		return epubScopeView{}, false
	}
	snapshotID, units, err := h.services.Store.GetExtractedUnitSnapshot(r.Context(), owner, book.ID)
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) {
		http.Error(w, "Review is unavailable because this book has no extracted EPUB units.", http.StatusConflict)
		return epubScopeView{}, false
	}
	if err != nil {
		fail(w, err)
		return epubScopeView{}, false
	}
	classifications, err := h.services.Store.GetEPUBUnitClassifications(r.Context(), owner, book.ID, epub.ClassifierName, epub.ClassifierVersion)
	if errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		http.Error(w, "Review is unavailable because this book has no unit classifications.", http.StatusConflict)
		return epubScopeView{}, false
	}
	if err != nil {
		fail(w, err)
		return epubScopeView{}, false
	}
	if len(units.Units) != len(classifications) {
		fail(w, errors.New("review scope: incomplete persisted classifications"))
		return epubScopeView{}, false
	}
	view := epubScopeView{Book: book, SnapshotID: snapshotID, Units: make([]epubScopeUnitView, len(units.Units))}
	for i, unit := range units.Units {
		if classifications[i].SourceUnitSnapshot.UnitID != unit.ID || classifications[i].SourceUnitSnapshot.SnapshotID != snapshotID {
			fail(w, errors.New("review scope: classification snapshot mismatch"))
			return epubScopeView{}, false
		}
		characters := len([]rune(unit.Text))
		view.Units[i] = epubScopeUnitView{Unit: unit, Classification: classifications[i], CharacterCount: characters, TokenEstimate: (characters + 3) / 4}
	}
	byID := make(map[string]epubScopeUnitView, len(view.Units))
	for _, item := range view.Units {
		byID[item.Unit.ID] = item
	}
	for _, group := range epub.BuildUnitGroups(units.Units) {
		item := epubScopeGroupView{Group: group}
		for _, id := range group.UnitIDs {
			item.CharacterCount += byID[id].CharacterCount
			item.TokenEstimate += byID[id].TokenEstimate
		}
		view.Groups = append(view.Groups, item)
	}
	return view, true
}

func (h *Handler) reviewEPUBScope(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	view, ok := h.loadEPUBScope(w, r, u.ID)
	if !ok {
		return
	}
	selected, err := h.applyScopePreset(r, u.ID, &view)
	if err != nil {
		h.renderEPUBScopeError(w, r, u, view, err.Error())
		return
	}
	render(w, r, EPUBScopeReviewPage(u, h.csrf(w, r), view, "", selected))
}

func (h *Handler) applyScopePreset(r *http.Request, owner string, view *epubScopeView) (string, error) {
	preset := r.URL.Query().Get("preset")
	priorID := r.URL.Query().Get("prior_scope_id")
	if preset == "" && priorID == "" {
		return "", nil
	}
	if preset != "recommended" && preset != "all" && preset != "prior" {
		return "", errors.New("The requested scope preset is invalid.")
	}
	view.Preset = preset
	var prior domain.EPUBReviewedScopeSnapshot
	if priorID != "" {
		var err error
		prior, err = h.services.Store.GetEPUBReviewedScope(r.Context(), owner, view.Book.ID, priorID)
		if err != nil {
			return "", errors.New("The prior scope is unavailable for this book and owner.")
		}
		if prior.SchemaVersion != domain.EPUBReviewedScopeSchemaVersion || prior.SourceUnitSnapshot.SnapshotID != view.SnapshotID || prior.SourceUnitSnapshot.ExtractedUnitsSchemaVersion != domain.ExtractedUnitsSchemaVersion || prior.Classifier.Name != epub.ClassifierName || prior.Classifier.Version != epub.ClassifierVersion {
			return "", errors.New("The prior scope is stale or uses a different schema or classifier. Review the current units instead.")
		}
		view.PriorScope = &prior
	}
	if preset == "prior" && priorID == "" {
		return "", errors.New("A prior scope ID is required for the prior-selection preset.")
	}
	selected := make(map[string]bool, len(view.Units))
	for _, item := range view.Units {
		selected[item.Unit.ID] = preset == "all" || (preset == "recommended" && item.Classification.RecommendedInclusion)
	}
	if preset == "prior" {
		for _, item := range prior.SelectedUnits {
			selected[item.UnitID] = true
		}
	}
	var ids []string
	old := map[string]bool{}
	for _, item := range prior.SelectedUnits {
		old[item.UnitID] = true
	}
	for _, item := range view.Units {
		if selected[item.Unit.ID] {
			ids = append(ids, item.Unit.ID)
			view.Comparison.New = append(view.Comparison.New, item)
			view.Comparison.NewCharacters += item.CharacterCount
			view.Comparison.NewTokens += item.TokenEstimate
		}
		if old[item.Unit.ID] {
			view.Comparison.Old = append(view.Comparison.Old, item)
			view.Comparison.OldCharacters += item.CharacterCount
			view.Comparison.OldTokens += item.TokenEstimate
		}
		if selected[item.Unit.ID] && !old[item.Unit.ID] {
			view.Comparison.Added = append(view.Comparison.Added, item)
		}
		if old[item.Unit.ID] && !selected[item.Unit.ID] {
			view.Comparison.Removed = append(view.Comparison.Removed, item)
		}
	}
	return "submitted:" + strings.Join(ids, ","), nil
}

func (h *Handler) confirmEPUBScope(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	view, ok := h.loadEPUBScope(w, r, u.ID)
	if !ok {
		return
	}
	if submittedSnapshot := r.FormValue("snapshot_id"); submittedSnapshot == "" || submittedSnapshot != view.SnapshotID {
		h.renderEPUBScopeError(w, r, u, view, "The EPUB unit snapshot changed. Reload the page and review the current hierarchy.")
		return
	}
	selected := make(map[string]struct{}, len(r.Form["unit_id"]))
	for _, id := range r.Form["unit_id"] {
		if _, duplicate := selected[id]; duplicate {
			h.renderEPUBScopeError(w, r, u, view, "A unit was submitted more than once. Please review your selection.")
			return
		}
		selected[id] = struct{}{}
	}
	groups := make(map[string]epub.UnitGroup, len(view.Groups))
	for _, item := range view.Groups {
		groups[item.Group.ID] = item.Group
	}
	includeGroups, ok := submittedGroups(r.Form["group_include"], groups)
	if !ok {
		h.renderEPUBScopeError(w, r, u, view, "The selection contains a group that does not belong to this book.")
		return
	}
	excludeGroups, ok := submittedGroups(r.Form["group_exclude"], groups)
	if !ok {
		h.renderEPUBScopeError(w, r, u, view, "The selection contains a group that does not belong to this book.")
		return
	}
	for id := range includeGroups {
		if _, contradictory := excludeGroups[id]; contradictory {
			h.renderEPUBScopeError(w, r, u, view, "A group cannot be included and excluded in the same submission.")
			return
		}
		for _, unitID := range groups[id].UnitIDs {
			selected[unitID] = struct{}{}
		}
	}
	for id := range excludeGroups {
		for _, unitID := range groups[id].UnitIDs {
			delete(selected, unitID)
		}
	}
	references := make([]domain.EPUBSelectedUnitReference, 0, len(selected))
	recommended := true
	for _, item := range view.Units {
		_, included := selected[item.Unit.ID]
		if included {
			references = append(references, domain.EPUBSelectedUnitReference{UnitID: item.Unit.ID, Order: item.Unit.Order})
			delete(selected, item.Unit.ID)
		}
		if included != item.Classification.RecommendedInclusion {
			recommended = false
		}
	}
	if len(selected) != 0 {
		h.renderEPUBScopeError(w, r, u, view, "The selection contains a unit that does not belong to this book.")
		return
	}
	if len(references) == 0 {
		h.renderEPUBScopeError(w, r, u, view, "Select at least one readable unit before confirming the analysis scope.")
		return
	}
	mode := domain.EPUBScopeSelectionOverridden
	if recommended {
		mode = domain.EPUBScopeSelectionRecommended
	}
	scope := domain.EPUBReviewedScopeSnapshot{SchemaVersion: domain.EPUBReviewedScopeSchemaVersion, ScopeID: uuid.NewString(), OwnerID: u.ID, SourceMaterialID: view.Book.ID, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: view.SnapshotID, ExtractedUnitsSchemaVersion: domain.ExtractedUnitsSchemaVersion}, Classifier: domain.EPUBClassifierIdentity{Name: epub.ClassifierName, Version: epub.ClassifierVersion}, SelectionMode: mode, SelectedUnits: references}
	if _, err := h.services.Store.CreateEPUBReviewedScope(r.Context(), scope); err != nil {
		h.renderEPUBScopeError(w, r, u, view, "The scope could not be saved. Reload the page and review the current units.")
		return
	}
	handle, err := h.services.Analysis.SubmitScopedAnalysis(r.Context(), u.ID, view.Book.ID, scope.ScopeID)
	if err != nil {
		h.renderEPUBScopeError(w, r, u, view, "The saved scope could not be queued for analysis.")
		return
	}
	redirect(w, r, "/books/"+view.Book.ID+"?message="+url.QueryEscape(fmt.Sprintf("Analysis scope saved with %d selected units; job %d submitted.", len(references), handle.DisplayNumber)))
}

func submittedGroups(values []string, groups map[string]epub.UnitGroup) (map[string]struct{}, bool) {
	result := make(map[string]struct{}, len(values))
	for _, id := range values {
		if _, exists := groups[id]; !exists {
			return nil, false
		}
		if _, duplicate := result[id]; duplicate {
			return nil, false
		}
		result[id] = struct{}{}
	}
	return result, true
}

func (h *Handler) renderEPUBScopeError(w http.ResponseWriter, r *http.Request, u domain.User, view epubScopeView, message string) {
	w.WriteHeader(http.StatusBadRequest)
	render(w, r, EPUBScopeReviewPage(u, h.csrf(w, r), view, message, "submitted:"+strings.Join(r.Form["unit_id"], ",")))
}
func (h *Handler) analyzeBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	book, ok := h.loadBook(w, r, u.ID)
	if !ok {
		return
	}
	var handle analysis.Handle
	var err error
	if book.ReviewedScopeID != "" {
		handle, err = h.services.Analysis.SubmitScopedAnalysis(r.Context(), u.ID, book.Source.ID, book.ReviewedScopeID)
	} else {
		handle, err = h.services.Analysis.SubmitAnalysis(r.Context(), u.ID, book.Source.ID)
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/books/%s?message=Analysis+job+%d+submitted", r.PathValue("id"), handle.DisplayNumber))
}
func (h *Handler) loadBook(w http.ResponseWriter, r *http.Request, owner string) (domain.SourceMaterialSummary, bool) {
	return h.loadBookID(w, r, owner, r.PathValue("id"))
}
func (h *Handler) loadBookID(w http.ResponseWriter, r *http.Request, owner, id string) (domain.SourceMaterialSummary, bool) {
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return domain.SourceMaterialSummary{}, false
	}
	books, err := h.services.Store.ListSourceMaterials(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return domain.SourceMaterialSummary{}, false
	}
	for _, book := range books {
		if book.Source.ID == id {
			return book, true
		}
	}
	http.NotFound(w, r)
	return domain.SourceMaterialSummary{}, false
}
func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	h.renderSettings(w, r, nil, nil, r.URL.Query().Get("message"))
}

func (h *Handler) addStudyLanguage(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	language := strings.TrimSpace(r.FormValue("language"))
	supported, degraded := h.supportedNLP(r.Context())
	if degraded {
		http.Error(w, "NLP language discovery is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	var err error
	for _, candidate := range supported {
		if candidate.Language == language {
			if _, err = h.services.Store.PutSupportedLanguage(r.Context(), candidate.Language, candidate.DisplayName); err != nil {
				fail(w, err)
				return
			}
			if _, err = h.services.Store.PutLanguageProfile(r.Context(), user(r).ID, candidate.Language, candidate.DisplayName); err != nil {
				fail(w, err)
				return
			}
			redirect(w, r, "/settings?message=Study+language+added")
			return
		}
	}
	http.Error(w, "unsupported study language", http.StatusBadRequest)
}

func (h *Handler) removeStudyLanguage(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if err := h.services.Store.DeleteLanguageProfile(r.Context(), user(r).ID, strings.TrimSpace(r.FormValue("language"))); err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/settings?message=Study+language+removed")
}

func (h *Handler) renderSettings(w http.ResponseWriter, r *http.Request, result *knownvocab.ImportResult, known []domain.KnownVocabulary, message string) {
	u := user(r)
	supported, degraded := h.supportedNLP(r.Context())
	profiles, err := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	language := strings.TrimSpace(r.FormValue("language"))
	if language == "" {
		language = strings.TrimSpace(r.URL.Query().Get("language"))
	}
	if known == nil && language != "" {
		known, err = h.services.Store.ListKnownVocabulary(r.Context(), u.ID, language)
		if err != nil {
			fail(w, err)
			return
		}
	}
	render(w, r, SettingsPage(u, h.csrf(w, r), supported, profiles, degraded, language, result, known, message))
}

func (h *Handler) supportedNLP(ctx context.Context) ([]domain.SupportedLanguage, bool) {
	if h.services.Capabilities == nil {
		return nil, true
	}
	capabilities, err := h.services.Capabilities.GetCapabilities(ctx)
	if err != nil {
		return nil, true
	}
	languages := make([]domain.SupportedLanguage, 0, len(capabilities.Languages))
	for _, capability := range capabilities.Languages {
		if capability.Ready {
			languages = append(languages, domain.SupportedLanguage{Language: capability.Language, DisplayName: capability.DisplayName})
		}
	}
	return languages, capabilities.Degraded
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
	_, e := h.services.Store.CreateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{Name: strings.TrimSpace(r.FormValue("name")), URL: strings.TrimSpace(r.FormValue("url")), Username: r.FormValue("username"), Password: r.FormValue("password")})
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
	_, e = h.services.Store.UpdateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{ID: current.ID, Name: strings.TrimSpace(r.FormValue("name")), URL: strings.TrimSpace(r.FormValue("url")), Username: r.FormValue("username"), Password: password})
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
	if _, ok := h.supportedLanguage(w, r, language); !ok {
		return
	}
	u := user(r)
	feed, e := h.services.OPDS.Browse(r.Context(), u.ID, r.URL.Query().Get("connection"), r.URL.Query().Get("url"))
	if e != nil {
		opdsFail(w, e)
		return
	}
	trail := decodeTrail(r.URL.Query()["trail"])
	currentURL := r.URL.Query().Get("url")
	if currentURL == "" {
		render(w, r, CatalogRootFragment(r.URL.Query().Get("connection"), feed))
		return
	}
	trail = append(trail, CatalogCrumb{Title: feed.Title, URL: currentURL})
	render(w, r, FeedFragment(h.csrf(w, r), r.URL.Query().Get("connection"), feed, trail))
}
func (h *Handler) browseLanguage(w http.ResponseWriter, r *http.Request) {
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	if language == "" {
		render(w, r, CatalogNotice("Choose a language to browse its EPUB books."))
		return
	}
	capability, ok := h.supportedLanguage(w, r, language)
	if !ok {
		return
	}
	u := user(r)
	connectionID := r.URL.Query().Get("connection")
	languages, e := h.services.OPDS.Languages(r.Context(), u.ID, connectionID)
	if e != nil {
		opdsFail(w, e)
		return
	}
	languageID := catalogLanguageID(capability, languages)
	if languageID == "" {
		http.Error(w, "The catalog does not advertise the selected language.", http.StatusBadRequest)
		return
	}
	feed, e := h.services.OPDS.BrowseLanguage(r.Context(), u.ID, connectionID, languageID)
	if e != nil {
		opdsFail(w, e)
		return
	}
	render(w, r, LanguageResults(h.csrf(w, r), connectionID, language, feed))
}
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	if _, ok := h.supportedLanguage(w, r, language); !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		render(w, r, CatalogNotice("Enter a title or author to search this catalog."))
		return
	}
	u := user(r)
	feed, e := h.services.OPDS.Search(r.Context(), u.ID, r.URL.Query().Get("connection"), query)
	if e != nil {
		if errors.Is(e, opds.ErrSearchUnavailable) {
			render(w, r, CatalogNotice("Search is not available for this catalog. Browse its collections instead."))
			return
		}
		opdsFail(w, e)
		return
	}
	render(w, r, SearchResults(h.csrf(w, r), r.URL.Query().Get("connection"), query, feed))
}
func (h *Handler) acquire(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	language := strings.TrimSpace(r.FormValue("language"))
	if _, ok := h.supportedLanguage(w, r, language); !ok {
		return
	}
	entry := opds.Entry{ID: r.FormValue("entry_id"), Title: r.FormValue("title"), Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: r.FormValue("href")}}}
	result, e := h.services.OPDS.Acquire(r.Context(), u.ID, r.FormValue("connection"), language, entry)
	if e != nil {
		opdsFail(w, e)
		return
	}
	handle, e := h.services.Analysis.SubmitAnalysis(r.Context(), u.ID, result.Source.ID)
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, fmt.Sprintf("/books/%s?message=%s", result.Source.ID, url.QueryEscape(fmt.Sprintf("Imported to My Library. Analysis job #%d is queued — follow its progress below.", handle.DisplayNumber))))
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
	render(w, r, JobStatus(status))
}
func (h *Handler) loadJob(w http.ResponseWriter, r *http.Request, owner string) (analysis.Status, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return analysis.Status{}, false
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
func (h *Handler) knownVocabPage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	requestedLanguage := strings.TrimSpace(r.URL.Query().Get("language"))
	profiles, err := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	language := ""
	for _, profile := range profiles {
		if requestedLanguage == profile.Language {
			language = requestedLanguage
		}
	}
	if language == "" && len(profiles) > 0 {
		language = profiles[0].Language
	}
	known, err := h.services.Store.ListKnownVocabulary(r.Context(), u.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, KnownVocabPage(u, h.csrf(w, r), profiles, language, known))
}

func (h *Handler) importKnownVocab(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		h.renderKnownVocabResult(w, r, "", nil, nil, "The import is too large or could not be read.")
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	language := strings.TrimSpace(r.FormValue("language"))
	var input bytes.Buffer
	file, header, err := r.FormFile("vocabulary_file")
	if err == nil {
		defer file.Close()
		if contentType := header.Header.Get("Content-Type"); contentType != "" {
			mediaType, _, mediaErr := mime.ParseMediaType(contentType)
			if mediaErr != nil || (mediaType != "text/plain" && mediaType != "application/octet-stream") {
				h.renderKnownVocabResult(w, r, language, nil, nil, "Choose a UTF-8 plain text file to import.")
				return
			}
		}
		if _, err = io.Copy(&input, file); err != nil {
			h.renderKnownVocabResult(w, r, language, nil, nil, "The uploaded file could not be read.")
			return
		}
	} else if !errors.Is(err, http.ErrMissingFile) {
		h.renderKnownVocabResult(w, r, language, nil, nil, "The uploaded file could not be read.")
		return
	}
	if language == "" {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Choose a language before importing.")
		return
	}
	profiles, err := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	selected := false
	for _, profile := range profiles {
		selected = selected || profile.Language == language
	}
	if !selected {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Choose one of your study languages before importing.")
		return
	}
	if input.Len() == 0 {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Choose a non-empty UTF-8 text file to import.")
		return
	}
	if h.services.KnownVocab == nil {
		fail(w, errors.New("known vocabulary service is unavailable"))
		return
	}
	handle, err := h.services.KnownVocab.Submit(r.Context(), u.ID, language, input.String())
	if err != nil {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Import failed: "+err.Error())
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		render(w, r, KnownVocabImportStatus(knownvocab.Status{ID: handle.ID, Language: language, State: rivertype.JobStateAvailable}))
		return
	}
	redirect(w, r, fmt.Sprintf("/known-vocab/imports/%d/status", handle.ID))
}

func (h *Handler) knownVocabImportStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status, err := h.services.KnownVocab.Get(r.Context(), user(r).ID, id)
	if errors.Is(err, knownvocab.ErrJobNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, KnownVocabImportStatus(status))
}

func (h *Handler) renderKnownVocabResult(w http.ResponseWriter, r *http.Request, language string, result *knownvocab.ImportResult, known []domain.KnownVocabulary, message string) {
	if known == nil && language != "" {
		var err error
		known, err = h.services.Store.ListKnownVocabulary(r.Context(), user(r).ID, language)
		if err != nil {
			fail(w, err)
			return
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		render(w, r, KnownVocabResult(language, result, known, message))
		return
	}
	if r.FormValue("return_to") == "settings" {
		h.renderSettings(w, r, result, known, message)
		return
	}
	u := user(r)
	profiles, err := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, KnownVocabPageWithResult(u, h.csrf(w, r), profiles, language, result, known, message))
}
func (h *Handler) createDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.PreparedDeck == nil {
		http.NotFound(w, r)
		return
	}
	handle, err := h.services.PreparedDeck.Submit(r.Context(), user(r).ID, r.PathValue("id"), r.FormValue("external_translation_consent") == "on")
	if err != nil {
		handlePreparationError(w, r, err)
		return
	}
	w.Header().Set("Location", "/deck-preparations/"+url.PathEscape(handle.Preparation.ID)+"/status")
	w.WriteHeader(http.StatusSeeOther)
}

type deckPreparationResponse struct {
	ID           string                      `json:"id"`
	State        domain.DeckPreparationState `json:"state"`
	Progress     int                         `json:"progress"`
	Ready        bool                        `json:"ready"`
	Error        string                      `json:"error,omitempty"`
	Filename     string                      `json:"filename"`
	DeckName     string                      `json:"deck_name"`
	DownloadURL  string                      `json:"download_url,omitempty"`
	Completeness deckCompletenessResponse    `json:"completeness"`
}

type deckCompletenessResponse struct {
	TotalCards               int `json:"total_cards"`
	CardsWithEnglish         int `json:"cards_with_english"`
	CardsWithEnglishSentence int `json:"cards_with_contextual_sentence_translations"`
	QualityOmissions         int `json:"quality_omissions"`
}

func preparationResponse(p domain.DeckPreparation) deckPreparationResponse {
	progress := 0
	if p.State == domain.DeckPreparationPreparing {
		progress = 50
	} else if p.State == domain.DeckPreparationReady || p.State == domain.DeckPreparationFailed || p.State == domain.DeckPreparationCancelled {
		progress = 100
	}
	response := deckPreparationResponse{ID: p.ID, State: p.State, Progress: progress, Ready: p.State == domain.DeckPreparationReady, Error: p.Error, Filename: p.Filename, DeckName: p.DeckName, Completeness: deckCompletenessResponse{TotalCards: p.TotalCards, CardsWithEnglish: p.CardsWithEnglish, CardsWithEnglishSentence: p.CardsWithContextualSentenceTranslations, QualityOmissions: p.QualityOmissions}}
	if response.Ready {
		response.DownloadURL = "/deck-preparations/" + url.PathEscape(p.ID) + "/download"
	}
	return response
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
	writePreparationStatus(w, p)
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
	writePreparationStatus(w, p)
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
	writePreparationStatus(w, handle.Preparation)
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
	w.Header().Set("X-Mouseion-Cards-Total", strconv.Itoa(p.TotalCards))
	w.Header().Set("X-Mouseion-Cards-With-English", strconv.Itoa(p.CardsWithEnglish))
	w.Header().Set("X-Mouseion-Cards-With-English-Sentence", strconv.Itoa(p.CardsWithContextualSentenceTranslations))
	w.Header().Set("X-Mouseion-Cards-Quality-Omitted", strconv.Itoa(p.QualityOmissions))
	_, _ = w.Write(p.Artifact)
}

func handlePreparationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
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

func opdsFail(w http.ResponseWriter, err error) {
	log.Printf("mouseion: OPDS: %v", err)
	if errors.Is(err, persistence.ErrNotFound) {
		http.Error(w, "catalog not found", http.StatusNotFound)
		return
	}
	message := "The catalog request failed. Check the connection and try again."
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, opds.ErrNoEPUB), strings.Contains(lower, "incompatible media type"):
		message = "This book is not available as an EPUB. Choose another edition or format."
	case strings.Contains(lower, "401"), strings.Contains(lower, "403"):
		message = "The catalog rejected the credentials. Update the connection username and password."
	case strings.Contains(lower, "parse atom"), strings.Contains(lower, "html"):
		message = "The catalog returned a web page instead of an OPDS feed. Check the catalog URL."
	case strings.Contains(lower, "fetch feed"), strings.Contains(lower, "download epub"):
		message = "The catalog could not be reached. Check its URL and network availability, then try again."
	}
	http.Error(w, message, http.StatusBadGateway)
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

func (h *Handler) supportedLanguage(w http.ResponseWriter, r *http.Request, language string) (domain.SupportedLanguage, bool) {
	supported, degraded := h.supportedNLP(r.Context())
	if degraded {
		http.Error(w, "NLP language discovery is temporarily unavailable", http.StatusServiceUnavailable)
		return domain.SupportedLanguage{}, false
	}
	for _, candidate := range supported {
		if candidate.Language == language {
			return candidate, true
		}
	}
	http.Error(w, "unsupported analysis language", http.StatusBadRequest)
	return domain.SupportedLanguage{}, false
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
func credentialSummary(connection domain.OpdsConnection) string {
	if connection.Username != "" {
		return "Credentials saved securely for " + connection.Username + "."
	}
	return "No credentials configured."
}
func jobRunning(status analysis.Status) bool {
	return status.State == "available" || status.State == "pending" || status.State == "running" || status.State == "retryable" || status.State == "scheduled"
}
func jobState(status analysis.Status) string {
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
func statusClass(status string) string { return strings.ReplaceAll(status, " ", "-") }
func hasCampaignStatus(campaigns []campaignView, status domain.CampaignStatus) bool {
	for _, item := range campaigns {
		if item.Campaign.Status == status {
			return true
		}
	}
	return false
}
func hasCampaignHistory(campaigns []campaignView) bool {
	return hasCampaignStatus(campaigns, domain.CampaignComplete) || hasCampaignStatus(campaigns, domain.CampaignAbandoned)
}
func campaignStatusLabel(status domain.CampaignStatus) string {
	switch status {
	case domain.CampaignQueued:
		return "Queued"
	case domain.CampaignActive:
		return "Active"
	case domain.CampaignComplete:
		return "Complete"
	case domain.CampaignAbandoned:
		return "Abandoned"
	}
	return string(status)
}
func campaignBookLabel(status domain.BookProgress) string {
	switch status {
	case domain.BookQueued:
		return "Queued"
	case domain.BookReading:
		return "Reading"
	case domain.BookFinished:
		return "Finished"
	case domain.BookAbandoned:
		return "Abandoned"
	}
	return string(status)
}
func campaignDeckLabel(status domain.DeckProgress) string {
	switch status {
	case domain.DeckQueued:
		return "Queued"
	case domain.DeckStudying:
		return "Studying"
	case domain.DeckReviewed:
		return "Reviewed"
	case domain.DeckAbandoned:
		return "Abandoned"
	}
	return string(status)
}

func campaignProgressTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return " · " + value.UTC().Format("2006-01-02 15:04 UTC")
}
func knownVocabJobLabel(status string) string {
	switch status {
	case "available", "scheduled", "retryable", "pending":
		return "Queued"
	case "running":
		return "Running"
	case "completed":
		return "Completed"
	case "discarded", "cancelled":
		return "Failed"
	default:
		return status
	}
}
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
func knownVocabUPOS(upos string) string {
	if upos == "" {
		return "Any"
	}
	return upos
}
func analyzeLabel(status string) string {
	if status == "not analyzed" {
		return "Submit to analysis"
	}
	return "Re-analyze"
}
