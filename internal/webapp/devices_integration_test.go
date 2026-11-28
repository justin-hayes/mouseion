//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevicesAddShowsCredentialOnceAndIsolatesOwners(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "devices-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "devices-alice", "alice-password")
	createAccount(t, ctx, store, "devices-bob", "bob-password")
	deps := storeDependencies(store)
	deps.Devices = auth.NewDeviceService(store)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: deps, CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, alice.Username, "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, "devices-bob", "bob-password")

	t.Run("account panel links Devices", func(t *testing.T) {
		page := perform(t, h, http.MethodGet, "/devices", nil, aliceCookies)
		require.Equal(t, http.StatusOK, page.Code)
		assert.Contains(t, page.Body.String(), `<a href="/devices">Devices</a>`)
		assert.Contains(t, page.Body.String(), "No devices yet")
	})

	var password, username string
	t.Run("add renders the credential once without a redirect", func(t *testing.T) {
		response := perform(t, h, http.MethodPost, "/devices", url.Values{"csrf_token": {aliceCSRF}, "name": {"  Palma  "}}, aliceCookies)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		assert.Empty(t, response.Header().Get("Location"))
		body := response.Body.String()
		assert.Contains(t, body, "This password will not be shown again")
		assert.Contains(t, body, "http://example.com/kosync")
		username = regexp.MustCompile(`id="device-username">([^<]+)<`).FindStringSubmatch(body)[1]
		password = regexp.MustCompile(`id="device-password">([^<]+)<`).FindStringSubmatch(body)[1]
		assert.True(t, strings.HasPrefix(username, "palma-"), username)
		assert.Regexp(t, `^[2-9a-hjkmnp-z]{4}(-[2-9a-hjkmnp-z]{4}){3}$`, password)
	})

	t.Run("a later load lists the Device without its password", func(t *testing.T) {
		page := perform(t, h, http.MethodGet, "/devices", nil, aliceCookies)
		body := page.Body.String()
		assert.Equal(t, "no-store", page.Header().Get("Cache-Control"))
		assert.Contains(t, body, username)
		assert.Contains(t, body, "Palma")
		assert.Contains(t, body, "Last seen: Never")
		assert.NotContains(t, body, password)
		assert.NotContains(t, body, "will not be shown again")
	})

	t.Run("another owner never sees the Device", func(t *testing.T) {
		page := perform(t, h, http.MethodGet, "/devices", nil, bobCookies)
		assert.NotContains(t, page.Body.String(), username)
		assert.Contains(t, page.Body.String(), "No devices yet")
		added := perform(t, h, http.MethodPost, "/devices", url.Values{"csrf_token": {bobCSRF}, "name": {"Palma"}}, bobCookies)
		require.Equal(t, http.StatusOK, added.Code)
		again := perform(t, h, http.MethodGet, "/devices", nil, aliceCookies)
		assert.Equal(t, 1, strings.Count(again.Body.String(), "Username: <code>"))
	})

	t.Run("a missing or wrong CSRF token is refused", func(t *testing.T) {
		for _, token := range []string{"", "wrong"} {
			response := perform(t, h, http.MethodPost, "/devices", url.Values{"csrf_token": {token}, "name": {"Refused"}}, aliceCookies)
			assert.Equal(t, http.StatusForbidden, response.Code)
		}
		page := perform(t, h, http.MethodGet, "/devices", nil, aliceCookies)
		assert.NotContains(t, page.Body.String(), "Refused")
	})

	t.Run("an invalid name is refused and kept in the form", func(t *testing.T) {
		for _, name := range []string{"   ", strings.Repeat("x", 61)} {
			response := perform(t, h, http.MethodPost, "/devices", url.Values{"csrf_token": {aliceCSRF}, "name": {name}}, aliceCookies)
			assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
			body := response.Body.String()
			assert.Contains(t, body, deviceNameError)
			assert.NotContains(t, body, "will not be shown again")
			if strings.TrimSpace(name) != "" {
				assert.Contains(t, body, `value="`+name+`"`)
			}
		}
	})

	t.Run("a signed-out request is not served", func(t *testing.T) {
		assert.NotEqual(t, http.StatusOK, perform(t, h, http.MethodGet, "/devices", nil, nil).Code)
		assert.NotEqual(t, http.StatusOK, perform(t, h, http.MethodPost, "/devices", url.Values{"name": {"x"}}, nil).Code)
	})
}
