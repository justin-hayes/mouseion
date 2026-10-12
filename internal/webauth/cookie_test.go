package webauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCookieAppliesSharedAttributesUnderBothSecureSettings(t *testing.T) {
	tests := []struct {
		name       string
		secure     bool
		wantHeader string
	}{
		{"plain HTTP", false, "mouseion_session=token-value; Path=/; Max-Age=3600; HttpOnly; SameSite=Lax"},
		{"HTTPS only", true, "mouseion_session=token-value; Path=/; Max-Age=3600; HttpOnly; Secure; SameSite=Lax"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			http.SetCookie(w, NewCookie(CookieName, "token-value", tt.secure, 3600))
			require.Equal(t, tt.wantHeader, w.Header().Get("Set-Cookie"))
		})
	}
}

func TestClearCookieExpiresSessionUnderBothSecureSettings(t *testing.T) {
	tests := []struct {
		name       string
		secure     bool
		wantHeader string
	}{
		{"plain HTTP", false, "mouseion_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax"},
		{"HTTPS only", true, "mouseion_session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			(&Handler{secureCookies: tt.secure}).clearCookie(w)
			require.Equal(t, tt.wantHeader, w.Header().Get("Set-Cookie"))
		})
	}
}
