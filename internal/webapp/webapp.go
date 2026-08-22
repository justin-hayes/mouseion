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
	"strconv"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/frequency"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/review"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

const csrfCookie = "mouseion_csrf"

type Store interface {
	PutLanguageProfile(context.Context, string, string, string) (domain.LanguageProfile, error)
	ListLanguageProfiles(context.Context, string) ([]domain.LanguageProfile, error)
	CreateOpdsConnection(context.Context, domain.OpdsConnection) (domain.OpdsConnection, error)
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
	ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error)
	UpdateOpdsConnection(context.Context, string, domain.OpdsConnection) (domain.OpdsConnection, error)
	DeleteOpdsConnection(context.Context, string, string) error
	ListSourceMaterials(context.Context, string) ([]domain.SourceMaterialSummary, error)
	ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
}
type OPDS interface {
	Browse(context.Context, string, string, string) (opds.Feed, error)
	Search(context.Context, string, string, string) (opds.Feed, error)
	Acquire(context.Context, string, string, opds.Entry) (epub.ImportResult, error)
}
type Analysis interface {
	SubmitAnalysis(context.Context, string, string) (analysis.Handle, error)
	Get(context.Context, string, int64) (analysis.Status, error)
}
type Review interface {
	Present(context.Context, string) ([]review.Item, error)
	Accept(context.Context, string, vocabulary.Identity) (domain.VocabularyState, error)
	Ignore(context.Context, string, vocabulary.Identity) (domain.VocabularyState, error)
	MarkKnown(context.Context, string, vocabulary.Identity) (domain.VocabularyState, error)
	Reset(context.Context, string, vocabulary.Identity) (domain.VocabularyState, error)
	EditExample(context.Context, string, vocabulary.Identity, string) (domain.CuratedSentence, error)
	ChooseAlternate(context.Context, string, vocabulary.Identity, int) (domain.CuratedSentence, error)
}

// Services keeps UI dependencies explicit and makes web-level tests independent of infrastructure.
type Services struct {
	Auth            *auth.Service
	WebAuth         *webauth.Handler
	Store           Store
	OPDS            OPDS
	Analysis        Analysis
	Review          Review
	Frequency       *frequency.Service
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
	h.mux.Handle("GET /", h.user(http.HandlerFunc(h.dashboard)))
	h.mux.Handle("GET /library", h.user(http.HandlerFunc(h.library)))
	h.mux.Handle("GET /books/{id}", h.user(http.HandlerFunc(h.book)))
	h.mux.Handle("POST /books/{id}/analyze", h.user(http.HandlerFunc(h.analyzeBook)))
	h.mux.Handle("POST /logout", h.user(http.HandlerFunc(h.logout)))
	h.mux.Handle("GET /languages", h.user(http.HandlerFunc(h.languages)))
	h.mux.Handle("POST /languages", h.user(http.HandlerFunc(h.saveLanguage)))
	h.mux.Handle("GET /connections", h.user(http.HandlerFunc(h.connections)))
	h.mux.Handle("POST /connections", h.user(http.HandlerFunc(h.createConnection)))
	h.mux.Handle("POST /connections/{id}", h.user(http.HandlerFunc(h.updateConnection)))
	h.mux.Handle("POST /connections/{id}/delete", h.user(http.HandlerFunc(h.deleteConnection)))
	h.mux.Handle("GET /catalog", h.user(http.HandlerFunc(h.catalog)))
	h.mux.Handle("GET /opds/browse", h.user(http.HandlerFunc(h.browse)))
	h.mux.Handle("GET /opds/search", h.user(http.HandlerFunc(h.search)))
	h.mux.Handle("POST /opds/acquire", h.user(http.HandlerFunc(h.acquire)))
	h.mux.Handle("GET /jobs", h.user(http.HandlerFunc(h.jobs)))
	h.mux.Handle("GET /jobs/{id}", h.user(http.HandlerFunc(h.job)))
	h.mux.Handle("GET /jobs/{id}/status", h.user(http.HandlerFunc(h.jobStatus)))
	h.mux.Handle("GET /review", h.user(http.HandlerFunc(h.reviewPage)))
	h.mux.Handle("POST /review/{action}", h.user(http.HandlerFunc(h.reviewAction)))
	h.mux.Handle("GET /deck", h.user(http.HandlerFunc(h.deck)))
	h.mux.Handle("GET /known-vocab", h.user(http.HandlerFunc(h.knownVocabPage)))
	h.mux.Handle("POST /known-vocab/import", h.user(http.HandlerFunc(h.importKnownVocab)))
	h.mux.Handle("GET /deck/download", h.user(http.HandlerFunc(h.downloadDeck)))
	h.mux.Handle("GET /admin", h.user(http.HandlerFunc(h.admin)))
	h.mux.Handle("GET /admin/users", h.user(http.HandlerFunc(h.adminUsers)))
	h.mux.Handle("POST /admin/users", h.user(http.HandlerFunc(h.createUser)))
	h.mux.Handle("GET /admin/frequency", h.user(http.HandlerFunc(h.adminFrequency)))
	h.mux.Handle("POST /admin/frequency", h.user(http.HandlerFunc(h.createFrequency)))
	h.mux.Handle("POST /admin/frequency/{id}/{action}", h.user(http.HandlerFunc(h.frequencyAction)))
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
	books, err := h.services.Store.ListSourceMaterials(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return domain.SourceMaterialSummary{}, false
	}
	for _, book := range books {
		if book.Source.ID == r.PathValue("id") {
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
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
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
	_, e = h.services.Store.UpdateOpdsConnection(r.Context(), u.ID, domain.OpdsConnection{ID: current.ID, OwnerID: u.ID, Name: strings.TrimSpace(r.FormValue("name")), URL: strings.TrimSpace(r.FormValue("url")), Username: r.FormValue("username"), Password: password, Language: strings.TrimSpace(r.FormValue("language"))})
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
		opdsFail(w, e)
		return
	}
	trail := decodeTrail(r.URL.Query()["trail"])
	currentURL := r.URL.Query().Get("url")
	if currentURL != "" {
		trail = append(trail, CatalogCrumb{Title: feed.Title, URL: currentURL})
	}
	render(w, r, FeedFragment(h.csrf(w, r), r.URL.Query().Get("connection"), feed, trail, currentURL == ""))
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
func (h *Handler) reviewPage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	items, err := h.services.Review.Present(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, ReviewPage(u, h.csrf(w, r), items, r.URL.Query().Get("message")))
}
func reviewIdentity(r *http.Request) vocabulary.Identity {
	return vocabulary.Identity{Language: strings.TrimSpace(r.FormValue("language")), CanonicalLemma: strings.TrimSpace(r.FormValue("lemma")), UPOS: strings.TrimSpace(r.FormValue("upos"))}
}
func (h *Handler) reviewAction(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u, id, action := user(r), reviewIdentity(r), r.PathValue("action")
	var err error
	switch action {
	case "accept":
		_, err = h.services.Review.Accept(r.Context(), u.ID, id)
	case "ignore":
		_, err = h.services.Review.Ignore(r.Context(), u.ID, id)
	case "known":
		_, err = h.services.Review.MarkKnown(r.Context(), u.ID, id)
	case "reset":
		_, err = h.services.Review.Reset(r.Context(), u.ID, id)
	case "edit":
		_, err = h.services.Review.EditExample(r.Context(), u.ID, id, r.FormValue("sentence"))
	case "alternate":
		index, parseErr := strconv.Atoi(r.FormValue("alternate"))
		if parseErr != nil {
			err = parseErr
		} else {
			_, err = h.services.Review.ChooseAlternate(r.Context(), u.ID, id, index)
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/review?message=Review+saved")
}
func (h *Handler) deck(w http.ResponseWriter, r *http.Request) {
	render(w, r, DeckPage(user(r), h.csrf(w, r)))
}

func (h *Handler) knownVocabPage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	profiles, err := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
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
	u := user(r)
	profiles, err := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, KnownVocabPageWithResult(u, h.csrf(w, r), profiles, language, result, known, message))
}
func (h *Handler) downloadDeck(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "Mouseion"
	}
	artifact, err := h.services.CardExport.Export(r.Context(), user(r).ID, name)
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
func (h *Handler) adminFrequency(w http.ResponseWriter, r *http.Request) {
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	datasets, err := h.services.Frequency.List(r.Context(), strings.TrimSpace(r.URL.Query().Get("language")))
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, AdminFrequencyPage(u, h.csrf(w, r), datasets, r.URL.Query().Get("language"), r.URL.Query().Get("message")))
}
func (h *Handler) createFrequency(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid upload", 400)
		return
	}
	file, _, err := r.FormFile("dataset")
	if err != nil {
		http.Error(w, "dataset file required", 400)
		return
	}
	defer file.Close()
	language, version := strings.TrimSpace(r.FormValue("language")), strings.TrimSpace(r.FormValue("version"))
	if r.FormValue("replace") == "on" {
		_, _, err = h.services.Frequency.Replace(r.Context(), u.ID, language, version, file)
	} else {
		_, _, err = h.services.Frequency.Create(r.Context(), u.ID, language, version, file)
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/admin/frequency?language="+url.QueryEscape(language)+"&message=Dataset+uploaded")
}
func (h *Handler) frequencyAction(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	var err error
	switch r.PathValue("action") {
	case "activate":
		err = h.services.Frequency.Activate(r.Context(), u.ID, r.PathValue("id"))
	case "deactivate":
		err = h.services.Frequency.Deactivate(r.Context(), u.ID, r.PathValue("id"))
	case "remove":
		err = h.services.Frequency.Remove(r.Context(), u.ID, r.PathValue("id"))
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/admin/frequency?message=Dataset+updated")
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
	_, e := h.services.Auth.CreateUser(r.Context(), u.ID, r.FormValue("username"), r.FormValue("password"), false)
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
func frequencyStatus(dataset domain.FrequencyDataset) string {
	if dataset.Active {
		return "Active"
	}
	return "Inactive"
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
