package webauth

import "net/http"

// NewCookie builds every app cookie. The Secure flag follows
// MOUSEION_COOKIE_SECURE; plain HTTP is the tailnet design (ADR 0009), so the
// policy is decided and suppressed here rather than at each call site.
func NewCookie(name, value string, secure bool, maxAge int) *http.Cookie {
	//nolint:gosec // plain HTTP is supported on the private tailnet; HttpOnly and SameSite remain enabled.
	return &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge}
}
