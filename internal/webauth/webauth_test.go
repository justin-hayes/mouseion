package webauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireUserRedirectsOnlyBrowserNavigation(t *testing.T) {
	h := (&Handler{}).RequireUser(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		require.FailNow(t, "protected handler called")
	}))
	tests := []struct {
		name, method, accept, fetchMode string
		wantStatus                      int
		wantLocation                    string
	}{
		{"navigation with fetch metadata", "GET", "text/html", "navigate", http.StatusSeeOther, "/login?next=%2Flibrary%3Fsort%3Dtitle"},
		{"navigation without fetch metadata", "GET", "text/html,application/xhtml+xml", "", http.StatusSeeOther, "/login?next=%2Flibrary%3Fsort%3Dtitle"},
		{"fetch", "GET", "text/html", "cors", http.StatusUnauthorized, ""},
		{"api", "GET", "application/json", "", http.StatusUnauthorized, ""},
		{"unsafe post", "POST", "text/html", "navigate", http.StatusUnauthorized, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), tt.method, "/library?sort=title", nil)
			r.Header.Set("Accept", tt.accept)
			r.Header.Set("Sec-Fetch-Mode", tt.fetchMode)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code, "status=%d location=%q body=%q", w.Code, w.Header().Get("Location"), w.Body.String())
			assert.Equal(t, tt.wantLocation, w.Header().Get("Location"), "status=%d location=%q body=%q", w.Code, w.Header().Get("Location"), w.Body.String())
		})
	}
}

func TestRequireUserDoesNotRedirectHTMXRequestTypes(t *testing.T) {
	h := (&Handler{}).RequireUser(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		require.FailNow(t, "protected handler called")
	}))
	for _, requestType := range []string{"full", "partial"} {
		t.Run(requestType, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library", nil)
			r.Header.Set("Accept", "text/html")
			r.Header.Set("Sec-Fetch-Mode", "navigate")
			r.Header.Set("Hx-Request-Type", requestType)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Empty(t, w.Header().Get("Location"))
			assert.Equal(t, "HX-Request-Type", w.Header().Get("Vary"))
		})
	}
}

func TestSafeReturnPath(t *testing.T) {
	for raw, want := range map[string]string{
		"/books/one?tab=jobs":       "/books/one?tab=jobs",
		"/journey#journey-book-one": "/journey#journey-book-one",
		"https://evil.example/":     "/",
		"//evil.example/":           "/",
		`/\evil.example/`:           "/",
		"library":                   "/",
		"":                          "/",
	} {
		assert.Equal(t, want, SafeReturnPath(raw), "SafeReturnPath(%q)", raw)
	}
}
