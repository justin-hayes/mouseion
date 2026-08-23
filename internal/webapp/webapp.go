// Package webapp provides Mouseion's authenticated server-rendered web client.
package webapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

const csrfCookie = "mouseion_csrf"

type Store interface {
	PutLanguageProfile(context.Context, string, string, string) (domain.LanguageProfile, error)
	ListLanguageProfiles(context.Context, string) ([]domain.LanguageProfile, error)
	DeleteLanguageProfile(context.Context, string, string) error
	PutSupportedLanguage(context.Context, string, string) (domain.SupportedLanguage, error)
	ListSupportedLanguages(context.Context) ([]domain.SupportedLanguage, error)
	CreateOpdsConnection(context.Context, domain.OpdsConnection) (domain.OpdsConnection, error)
	GetOpdsConnection(context.Context, string) (domain.OpdsConnection, error)
	ListOpdsConnections(context.Context) ([]domain.OpdsConnection, error)
	UpdateOpdsConnection(context.Context, domain.OpdsConnection) (domain.OpdsConnection, error)
	DeleteOpdsConnection(context.Context, string) error
	ListSourceMaterials(context.Context, string) ([]domain.SourceMaterialSummary, error)
	ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
}
type OPDS interface {
	Browse(context.Context, string, string, string) (opds.Feed, error)
	Languages(context.Context, string, string) (opds.Feed, error)
	BrowseLanguage(context.Context, string, string, string) (opds.Feed, error)
	Search(context.Context, string, string, string) (opds.Feed, error)
	Acquire(context.Context, string, string, opds.Entry) (epub.ImportResult, error)
}
type Analysis interface {
	SubmitAnalysis(context.Context, string, string) (analysis.Handle, error)
	Get(context.Context, string, int64) (analysis.Status, error)
}

// Services keeps UI dependencies explicit and makes web-level tests independent of infrastructure.
type Services struct {
	Auth            *auth.Service
	WebAuth         *webauth.Handler
	Store           Store
	OPDS            OPDS
	Analysis        Analysis
	KnownVocab      *knownvocab.Service
	CardExport      *cardexport.Service
	SecureCookies   bool
	SessionLifetime time.Duration
}

type Handler struct {
	services Services
	mux      *http.ServeMux
}

func New(s Services) *Handler {
	h := &Handler{services: s, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /login", h.loginPage)
	h.mux.HandleFunc("POST /login", h.login)
	h.mux.HandleFunc("GET /register", h.registerPage)
	h.mux.HandleFunc("POST /register", h.register)
	h.mux.Handle("POST /admin/bootstrap", s.WebAuth)
	h.mux.Handle("POST /logout-all", s.WebAuth)
	h.mux.Handle("POST /admin/users/{id}/reset-password", s.WebAuth)
	h.mux.Handle("GET /{$}", h.user(http.HandlerFunc(h.dashboard)))
	h.mux.Handle("GET /library", h.learner(http.HandlerFunc(h.library)))
	h.mux.Handle("GET /books/{id}", h.learner(http.HandlerFunc(h.book)))
	h.mux.Handle("POST /books/{id}/analyze", h.learner(http.HandlerFunc(h.analyzeBook)))
	h.mux.Handle("POST /books/{id}/deck", h.learner(http.HandlerFunc(h.generateDeck)))
	h.mux.Handle("POST /logout", h.user(http.HandlerFunc(h.logout)))
	h.mux.Handle("GET /languages", h.adminOnly(http.HandlerFunc(h.languages)))
	h.mux.Handle("POST /languages", h.adminOnly(http.HandlerFunc(h.saveLanguage)))
	h.mux.Handle("GET /settings", h.learner(http.HandlerFunc(h.settings)))
	h.mux.Handle("POST /settings/languages", h.learner(http.HandlerFunc(h.addStudyLanguage)))
	h.mux.Handle("POST /settings/languages/remove", h.learner(http.HandlerFunc(h.removeStudyLanguage)))
	h.mux.Handle("GET /connections", h.learner(http.HandlerFunc(h.connections)))
	h.mux.Handle("GET /admin/connections", h.adminOnly(http.HandlerFunc(h.adminConnections)))
	h.mux.Handle("POST /connections", h.adminOnly(http.HandlerFunc(h.createConnection)))
	h.mux.Handle("POST /admin/connections", h.adminOnly(http.HandlerFunc(h.createConnection)))
	h.mux.Handle("POST /connections/{id}", h.adminOnly(http.HandlerFunc(h.updateConnection)))
	h.mux.Handle("POST /admin/connections/{id}", h.adminOnly(http.HandlerFunc(h.updateConnection)))
	h.mux.Handle("POST /connections/{id}/delete", h.adminOnly(http.HandlerFunc(h.deleteConnection)))
	h.mux.Handle("POST /admin/connections/{id}/delete", h.adminOnly(http.HandlerFunc(h.deleteConnection)))
	h.mux.Handle("GET /catalog", h.learner(http.HandlerFunc(h.catalog)))
	h.mux.Handle("GET /opds/browse", h.learner(http.HandlerFunc(h.browse)))
	h.mux.Handle("GET /opds/language", h.learner(http.HandlerFunc(h.browseLanguage)))
	h.mux.Handle("GET /opds/search", h.learner(http.HandlerFunc(h.search)))
	h.mux.Handle("POST /opds/acquire", h.learner(http.HandlerFunc(h.acquire)))
	h.mux.Handle("GET /jobs", h.learner(http.HandlerFunc(h.jobs)))
	h.mux.Handle("GET /jobs/{id}", h.learner(http.HandlerFunc(h.job)))
	h.mux.Handle("GET /jobs/{id}/status", h.learner(http.HandlerFunc(h.jobStatus)))
	h.mux.Handle("GET /known-vocab", h.learner(http.HandlerFunc(h.knownVocabPage)))
	h.mux.Handle("POST /known-vocab/import", h.learner(http.HandlerFunc(h.importKnownVocab)))
	h.mux.Handle("GET /admin", h.adminOnly(http.HandlerFunc(h.admin)))
	h.mux.Handle("GET /admin/users", h.adminOnly(http.HandlerFunc(h.adminUsers)))
	h.mux.Handle("POST /admin/users", h.adminOnly(http.HandlerFunc(h.createUser)))
	return h
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }
func (h *Handler) user(next http.Handler) http.Handler              { return h.services.WebAuth.RequireUser(next) }
func (h *Handler) learner(next http.Handler) http.Handler {
	return h.user(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user(r).IsAdmin {
			http.Error(w, "user account required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}
func (h *Handler) adminOnly(next http.Handler) http.Handler {
	return h.user(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !user(r).IsAdmin {
			http.Error(w, "administrator required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}
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
	render(w, r, LoginPage(h.csrf(w, r), r.URL.Query().Get("error")))
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
		redirect(w, r, "/login?error=Invalid+credentials")
		return
	}
	h.setSession(w, token)
	h.rotateCSRF(w)
	redirect(w, r, "/")
}
func (h *Handler) registerPage(w http.ResponseWriter, r *http.Request) {
	render(w, r, RegisterPage(h.csrf(w, r), r.URL.Query().Get("error")))
}
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	_, err := h.services.Auth.BootstrapAdmin(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		redirect(w, r, "/register?error="+url.QueryEscape("Registration unavailable: "+err.Error()))
		return
	}
	token, err := h.services.Auth.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		http.Error(w, "administrator created; sign in", 500)
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
	if user(r).IsAdmin {
		redirect(w, r, "/admin")
		return
	}
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
func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	summary, ok := h.loadBook(w, r, u.ID)
	if !ok {
		return
	}
	render(w, r, BookPage(u, h.csrf(w, r), summary, r.URL.Query().Get("message")))
}
func (h *Handler) analyzeBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	if _, ok := h.loadBook(w, r, u.ID); !ok {
		return
	}
	handle, err := h.services.Analysis.SubmitAnalysis(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/books/%s?message=Analysis+job+%d+submitted", r.PathValue("id"), handle.ID))
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
func (h *Handler) languages(w http.ResponseWriter, r *http.Request) {
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	p, e := h.services.Store.ListSupportedLanguages(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, LanguagesPage(u, h.csrf(w, r), p, r.URL.Query().Get("message")))
}
func (h *Handler) saveLanguage(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	_, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	_, e := h.services.Store.PutSupportedLanguage(r.Context(), strings.TrimSpace(r.FormValue("language")), strings.TrimSpace(r.FormValue("display_name")))
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/languages?message=Language+saved")
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	h.renderSettings(w, r, nil, nil, r.URL.Query().Get("message"))
}

func (h *Handler) addStudyLanguage(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	language := strings.TrimSpace(r.FormValue("language"))
	supported, err := h.services.Store.ListSupportedLanguages(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	for _, candidate := range supported {
		if candidate.Language == language {
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
	supported, err := h.services.Store.ListSupportedLanguages(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
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
	render(w, r, SettingsPage(u, h.csrf(w, r), supported, profiles, language, result, known, message))
}
func (h *Handler) connections(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	c, e := h.services.Store.ListOpdsConnections(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, ConnectionsPage(u, h.csrf(w, r), c))
}
func (h *Handler) adminConnections(w http.ResponseWriter, r *http.Request) {
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	c, e := h.services.Store.ListOpdsConnections(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, AdminConnectionsPage(u, h.csrf(w, r), c, r.URL.Query().Get("message")))
}
func (h *Handler) createConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	_, e := h.services.Store.CreateOpdsConnection(r.Context(), domain.OpdsConnection{Name: strings.TrimSpace(r.FormValue("name")), URL: strings.TrimSpace(r.FormValue("url")), Username: r.FormValue("username"), Password: r.FormValue("password"), Language: strings.TrimSpace(r.FormValue("language"))})
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/admin/connections?message=Catalog+added")
}
func (h *Handler) updateConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	current, e := h.services.Store.GetOpdsConnection(r.Context(), r.PathValue("id"))
	if e != nil {
		http.NotFound(w, r)
		return
	}
	password := r.FormValue("password")
	if password == "" {
		password = current.Password
	}
	_, e = h.services.Store.UpdateOpdsConnection(r.Context(), domain.OpdsConnection{ID: current.ID, Name: strings.TrimSpace(r.FormValue("name")), URL: strings.TrimSpace(r.FormValue("url")), Username: r.FormValue("username"), Password: password, Language: strings.TrimSpace(r.FormValue("language"))})
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/admin/connections?message=Catalog+updated")
}
func (h *Handler) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if e := h.services.Store.DeleteOpdsConnection(r.Context(), r.PathValue("id")); e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/admin/connections?message=Catalog+deleted")
}
func (h *Handler) catalog(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	c, e := h.services.Store.GetOpdsConnection(r.Context(), r.URL.Query().Get("connection"))
	if e != nil {
		http.NotFound(w, r)
		return
	}
	profiles, e := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	languages, e := h.services.OPDS.Languages(r.Context(), u.ID, c.ID)
	if e != nil {
		opdsFail(w, e)
		return
	}
	render(w, r, CatalogPage(u, h.csrf(w, r), c, languageOptions(profiles, languages)))
}
func (h *Handler) browse(w http.ResponseWriter, r *http.Request) {
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
	languageID := strings.TrimSpace(r.URL.Query().Get("language"))
	if languageID == "" {
		render(w, r, CatalogNotice("Choose a language to browse its EPUB books."))
		return
	}
	u := user(r)
	feed, e := h.services.OPDS.BrowseLanguage(r.Context(), u.ID, r.URL.Query().Get("connection"), languageID)
	if e != nil {
		opdsFail(w, e)
		return
	}
	render(w, r, LanguageResults(h.csrf(w, r), r.URL.Query().Get("connection"), feed))
}
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
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
	entry := opds.Entry{ID: r.FormValue("entry_id"), Title: r.FormValue("title"), Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: r.FormValue("href")}}}
	result, e := h.services.OPDS.Acquire(r.Context(), u.ID, r.FormValue("connection"), entry)
	if e != nil {
		opdsFail(w, e)
		return
	}
	handle, e := h.services.Analysis.SubmitAnalysis(r.Context(), u.ID, result.Source.ID)
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, fmt.Sprintf("/books/%s?message=%s", result.Source.ID, url.QueryEscape(fmt.Sprintf("Imported to My Library. Analysis job #%d is queued — follow its progress below.", handle.ID))))
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
	if pasted := r.FormValue("vocabulary"); pasted != "" {
		_, _ = input.WriteString(pasted)
	}
	file, _, err := r.FormFile("vocabulary_file")
	if err == nil {
		defer file.Close()
		if input.Len() > 0 {
			_ = input.WriteByte('\n')
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
	if len(bytes.TrimSpace(input.Bytes())) == 0 {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Paste vocabulary or choose a file to import.")
		return
	}
	if h.services.KnownVocab == nil {
		fail(w, errors.New("known vocabulary service is unavailable"))
		return
	}
	result, err := h.services.KnownVocab.Import(r.Context(), u.ID, language, &input)
	if err != nil {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Import failed: "+err.Error())
		return
	}
	known, err := h.services.Store.ListKnownVocabulary(r.Context(), u.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	h.renderKnownVocabResult(w, r, language, &result, known, "")
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
func (h *Handler) generateDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	book, ok := h.loadBook(w, r, u.ID)
	if !ok {
		return
	}
	artifact, err := h.services.CardExport.ExportCoverage(r.Context(), u.ID, book.Source.ID)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/tab-separated-values; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="mouseion-anki.tsv"`)
	w.Header().Set("X-Mouseion-Note-Type", base64.RawURLEncoding.EncodeToString([]byte(artifact.NoteType)))
	_, _ = io.WriteString(w, artifact.TSV)
}
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	u := user(r)
	if !u.IsAdmin {
		http.Error(w, "administrator required", http.StatusForbidden)
		return u, false
	}
	return u, true
}
func (h *Handler) admin(w http.ResponseWriter, r *http.Request) {
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	render(w, r, AdminPage(u, h.csrf(w, r)))
}
func (h *Handler) adminUsers(w http.ResponseWriter, r *http.Request) {
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	render(w, r, AdminUsersPage(u, h.csrf(w, r), r.URL.Query().Get("message")))
}
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		h.services.WebAuth.ServeHTTP(w, r)
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	_, e := h.services.Auth.CreateUser(r.Context(), u.ID, r.FormValue("username"), r.FormValue("password"), auth.RoleUser)
	if errors.Is(e, auth.ErrForbidden) {
		http.Error(w, "administrator required", 403)
		return
	}
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/admin/users?message=User+registered")
}
func fail(w http.ResponseWriter, err error) {
	log.Printf("mouseion: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func opdsFail(w http.ResponseWriter, err error) {
	log.Printf("mouseion: OPDS: %v", err)
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

type CatalogLanguage struct {
	ID, Name string
	Study    bool
}

func studyLanguageLabel(language CatalogLanguage) string {
	if language.Study {
		return " — study language"
	}
	return ""
}

func languageOptions(profiles []domain.LanguageProfile, feed opds.Feed) []CatalogLanguage {
	study := make(map[string]bool, len(profiles)*2)
	for _, profile := range profiles {
		study[strings.ToLower(strings.TrimSpace(profile.Language))] = true
		study[strings.ToLower(strings.TrimSpace(profile.DisplayName))] = true
	}
	options := make([]CatalogLanguage, 0, len(feed.Entries))
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
		options = append(options, CatalogLanguage{ID: id, Name: name, Study: study[strings.ToLower(name)] || study[strings.ToLower(id)]})
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Study != options[j].Study {
			return options[i].Study
		}
		return strings.ToLower(options[i].Name) < strings.ToLower(options[j].Name)
	})
	return options
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
