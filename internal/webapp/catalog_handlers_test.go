package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCatalogsRouteRendersCatalogManagementScreen(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	request := httptest.NewRequest(http.MethodGet, "/catalogs?error=Sync+failed", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /catalogs status=%d body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{
		"<title>Catalogs · Mouseion</title>",
		"<h1>Catalogs</h1>",
		`href="/catalogs" aria-current="page"`,
		"Add catalog connection",
		`class="feedback feedback--error"`,
		"Sync failed",
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("GET /catalogs missing %q: %s", want, response.Body.String())
		}
	}
}

func TestLegacyConnectionsRoutePermanentlyRedirectsAndPreservesSupportedQuery(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	request := httptest.NewRequest(http.MethodGet, "/connections?book_id=book%2F1&message=Catalog+added&error=Try+again&ignored=secret", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /connections status=%d, want %d", response.Code, http.StatusMovedPermanently)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect location: %v", err)
	}
	if location.Path != "/catalogs" {
		t.Fatalf("redirect path=%q, want /catalogs", location.Path)
	}
	if got := location.Query(); got.Get("book_id") != "book/1" || got.Get("message") != "Catalog added" || got.Get("error") != "Try again" || got.Has("ignored") {
		t.Fatalf("redirect query=%v, want supported parameters only", got)
	}
}
