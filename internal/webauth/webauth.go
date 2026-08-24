// Package webauth exposes the web-session authentication layer.
package webauth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const CookieName = "mouseion_session"

type userKey struct{}

type Handler struct {
	auth          *auth.Service
	secureCookies bool
	lifetime      time.Duration
	mux           *http.ServeMux
}

func New(service *auth.Service, secureCookies bool, lifetime time.Duration) *Handler {
	if lifetime <= 0 {
		lifetime = auth.DefaultSessionLifetime
	}
	h := &Handler{auth: service, secureCookies: secureCookies, lifetime: lifetime}
	mux := http.NewServeMux()
	h.mux = mux
	mux.HandleFunc("POST /login", h.login)
	mux.Handle("POST /logout", h.RequireUser(http.HandlerFunc(h.logout)))
	mux.Handle("POST /logout-all", h.RequireUser(http.HandlerFunc(h.logoutAll)))
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }
func UserFromContext(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(userKey{}).(domain.User)
	return u, ok
}

// RequireUser authenticates the session and exposes a stable domain.User in context.
func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		u, err := h.auth.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return false
	}
	return true
}
func (h *Handler) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(h.lifetime.Seconds())})
}
func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Path: "/", HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	token, err := h.auth.Login(r.Context(), in.Username, in.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	h.setCookie(w, token)
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(CookieName)
	if c != nil {
		_ = h.auth.Logout(r.Context(), c.Value)
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	if err := h.auth.LogoutEverywhere(r.Context(), u.ID); err != nil {
		http.Error(w, "unable to invalidate sessions", http.StatusInternalServerError)
		return
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
