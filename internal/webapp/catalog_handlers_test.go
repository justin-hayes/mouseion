package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogsRouteRendersCatalogManagementScreen(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/catalogs?error=Sync+failed", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	for _, want := range []string{
		"<title>Catalogs · Mouseion</title>",
		"<h1>Catalogs</h1>",
		`href="/catalogs" aria-current="page"`,
		"Add catalog connection",
		`class="feedback feedback--error"`,
		"Sync failed",
	} {
		assert.True(t, strings.Contains(response.Body.String(), want), "GET /catalogs missing %q: %s", want, response.Body.String())
	}
}

func TestLegacyConnectionsRoutePermanentlyRedirectsAndPreservesSupportedQuery(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/connections?book_id=book%2F1&message=Catalog+added&error=Try+again&ignored=secret", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusMovedPermanently, response.Code)
	location, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs", location.Path)
	got := location.Query()
	assert.Equal(t, "book/1", got.Get("book_id"))
	assert.Equal(t, "Catalog added", got.Get("message"))
	assert.Equal(t, "Try again", got.Get("error"))
	assert.False(t, got.Has("ignored"))
}
