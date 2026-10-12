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
		value      string
		secure     bool
		maxAge     int
		wantHeader string
	}{
		{"set over plain HTTP", "token-value", false, 3600, "mouseion_session=token-value; Path=/; Max-Age=3600; HttpOnly; SameSite=Lax"},
		{"set over HTTPS only", "token-value", true, 3600, "mouseion_session=token-value; Path=/; Max-Age=3600; HttpOnly; Secure; SameSite=Lax"},
		{"clear over plain HTTP", "", false, -1, "mouseion_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax"},
		{"clear over HTTPS only", "", true, -1, "mouseion_session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			http.SetCookie(w, NewCookie(CookieName, tt.value, tt.secure, tt.maxAge))
			require.Equal(t, tt.wantHeader, w.Header().Get("Set-Cookie"))
		})
	}
}
