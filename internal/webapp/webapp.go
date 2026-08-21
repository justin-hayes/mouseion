// Package webapp provides Mouseion's authenticated server-rendered web client.
package webapp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

const csrfCookie = "mouseion_csrf"

type Store interface {
	PutLanguageProfile(context.Context, string, string, string) (domain.LanguageProfile, error)
	ListLanguageProfiles(context.Context, string) ([]domain.LanguageProfile, error)
	CreateOpdsConnection(context.Context, domain.OpdsConnection) (domain.OpdsConnection, error)
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
	ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error)
	DeleteOpdsConnection(context.Context, string, string) error
	ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error)
}
type OPDS interface {
	Browse(context.Context, string, string, string) (opds.Feed, error)
	Search(context.Context, string, string, string) (opds.Feed, error)
	Acquire(context.Context, string, string, opds.Entry) (epub.ImportResult, error)
}
type Analysis interface {
	SubmitAnalysis(context.Context, string, string) (analysis.Handle, error)
}

// Services keeps UI dependencies explicit and makes web-level tests independent of infrastructure.
type Services struct {
	Auth            *auth.Service
	WebAuth         *webauth.Handler
	Store           Store
	OPDS            OPDS
	Analysis        Analysis
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
	h.mux.Handle("GET /", h.user(http.HandlerFunc(h.dashboard)))
	h.mux.Handle("POST /logout", h.user(http.HandlerFunc(h.logout)))
	h.mux.Handle("GET /languages", h.user(http.HandlerFunc(h.languages)))
	h.mux.Handle("POST /languages", h.user(http.HandlerFunc(h.saveLanguage)))
	h.mux.Handle("GET /connections", h.user(http.HandlerFunc(h.connections)))
	h.mux.Handle("POST /connections", h.user(http.HandlerFunc(h.createConnection)))
	h.mux.Handle("POST /connections/{id}/delete", h.user(http.HandlerFunc(h.deleteConnection)))
	h.mux.Handle("GET /catalog", h.user(http.HandlerFunc(h.catalog)))
	h.mux.Handle("GET /opds/browse", h.user(http.HandlerFunc(h.browse)))
	h.mux.Handle("GET /opds/search", h.user(http.HandlerFunc(h.search)))
	h.mux.Handle("POST /opds/acquire", h.user(http.HandlerFunc(h.acquire)))
	h.mux.Handle("GET /jobs", h.user(http.HandlerFunc(h.jobs)))
	h.mux.Handle("GET /admin/users", h.user(http.HandlerFunc(h.adminUsers)))
	h.mux.Handle("POST /admin/users", h.user(http.HandlerFunc(h.createUser)))
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
	u := user(r)
	p, e := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	c, e := h.services.Store.ListOpdsConnections(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	j, e := h.services.Store.ListAnalysisJobs(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, Dashboard(u, h.csrf(w, r), p, c, j))
}
func (h *Handler) languages(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	p, e := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
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
	u := user(r)
	_, e := h.services.Store.PutLanguageProfile(r.Context(), u.ID, strings.TrimSpace(r.FormValue("language")), strings.TrimSpace(r.FormValue("display_name")))
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/languages?message=Language+saved")
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
	_, e := h.services.Store.CreateOpdsConnection(r.Context(), domain.OpdsConnection{OwnerID: u.ID, Name: strings.TrimSpace(r.FormValue("name")), URL: strings.TrimSpace(r.FormValue("url")), Username: r.FormValue("username"), Password: r.FormValue("password"), Language: strings.TrimSpace(r.FormValue("language"))})
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, "/connections?message=Catalog+added")
}
func (h *Handler) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	if e := h.services.Store.DeleteOpdsConnection(r.Context(), u.ID, r.PathValue("id")); e != nil {
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
	render(w, r, CatalogPage(u, h.csrf(w, r), c))
}
func (h *Handler) browse(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	feed, e := h.services.OPDS.Browse(r.Context(), u.ID, r.URL.Query().Get("connection"), r.URL.Query().Get("url"))
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, FeedFragment(h.csrf(w, r), r.URL.Query().Get("connection"), feed))
}
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	feed, e := h.services.OPDS.Search(r.Context(), u.ID, r.URL.Query().Get("connection"), r.URL.Query().Get("q"))
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, FeedFragment(h.csrf(w, r), r.URL.Query().Get("connection"), feed))
}
func (h *Handler) acquire(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	entry := opds.Entry{ID: r.FormValue("entry_id"), Title: r.FormValue("title"), Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: r.FormValue("href")}}}
	result, e := h.services.OPDS.Acquire(r.Context(), u.ID, r.FormValue("connection"), entry)
	if e != nil {
		fail(w, e)
		return
	}
	handle, e := h.services.Analysis.SubmitAnalysis(r.Context(), u.ID, result.Source.ID)
	if e != nil {
		fail(w, e)
		return
	}
	redirect(w, r, fmt.Sprintf("/jobs?message=Submitted+analysis+job+%d", handle.ID))
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
func (h *Handler) adminUsers(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if !u.IsAdmin {
		http.Error(w, "administrator required", http.StatusForbidden)
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
	u := user(r)
	if !u.IsAdmin {
		http.Error(w, "administrator required", http.StatusForbidden)
		return
	}
	_, e := h.services.Auth.CreateUser(r.Context(), u.ID, r.FormValue("username"), r.FormValue("password"), r.FormValue("is_admin") == "on")
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
	http.Error(w, err.Error(), http.StatusInternalServerError)
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
func queryEscape(value string) string { return url.QueryEscape(value) }
