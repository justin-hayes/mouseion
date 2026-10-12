package webapp

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClearSessionExpiresCookieUnderBothSecureSettings(t *testing.T) {
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
			h := &Handler{services: Services{SecureCookies: tt.secure}}
			w := httptest.NewRecorder()
			h.clearSession(w)
			require.Equal(t, tt.wantHeader, w.Header().Get("Set-Cookie"))
		})
	}
}
