package webauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireUserRedirectsOnlyBrowserNavigation(t *testing.T) {
	h := (&Handler{}).RequireUser(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler called")
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
			r := httptest.NewRequest(tt.method, "/library?sort=title", nil)
			r.Header.Set("Accept", tt.accept)
			r.Header.Set("Sec-Fetch-Mode", tt.fetchMode)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.wantStatus || w.Header().Get("Location") != tt.wantLocation {
				t.Fatalf("status=%d location=%q body=%q", w.Code, w.Header().Get("Location"), w.Body.String())
			}
		})
	}
}

func TestSafeReturnPath(t *testing.T) {
	for raw, want := range map[string]string{
		"/books/one?tab=jobs":   "/books/one?tab=jobs",
		"https://evil.example/": "/",
		"//evil.example/":       "/",
		`/\evil.example/`:       "/",
		"library":               "/",
		"":                      "/",
	} {
		if got := SafeReturnPath(raw); got != want {
			t.Errorf("SafeReturnPath(%q)=%q want %q", raw, got, want)
		}
	}
}
