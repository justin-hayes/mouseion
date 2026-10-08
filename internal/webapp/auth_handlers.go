package webapp

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

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
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	token, err := h.services.Auth.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		query := url.Values{"error": {"Invalid credentials"}, "next": {webauth.SafeReturnPath(r.FormValue("next"))}}
		redirect(w, r, "/login?"+query.Encode())
		return
	}
	h.setSession(w, token)
	h.rotateCSRF(w, r)
	if _, failed := r.Context().Value(csrfFailureContextKey{}).(error); failed {
		return
	}
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
	if !h.checkCSRFAnyLanguage(w, r) {
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
	h.rotateCSRF(w, r)
	if _, failed := r.Context().Value(csrfFailureContextKey{}).(error); failed {
		return
	}
	redirect(w, r, "/")
}
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if c, err := r.Cookie(webauth.CookieName); err == nil {
			if err = h.services.Auth.Logout(r.Context(), c.Value); err != nil {
				http.Error(w, "unable to invalidate session", http.StatusInternalServerError)
				return
			}
		}
		h.clearSession(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	if c, err := r.Cookie(webauth.CookieName); err == nil {
		if err = h.services.Auth.Logout(r.Context(), c.Value); err != nil {
			fail(w, err)
			return
		}
	}
	h.clearSession(w)
	redirect(w, r, "/login")
}
func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := h.services.Auth.LogoutEverywhere(r.Context(), user(r).ID); err != nil {
			http.Error(w, "unable to invalidate sessions", http.StatusInternalServerError)
			return
		}
		h.clearSession(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	if err := h.services.Auth.LogoutEverywhere(r.Context(), user(r).ID); err != nil {
		fail(w, err)
		return
	}
	h.clearSession(w)
	redirect(w, r, "/login")
}
