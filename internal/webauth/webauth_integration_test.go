//go:build integration

package webauth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstAccountAuthLifecycleAgainstPostgres(t *testing.T) {
	_, service, _ := setup(t)
	ctx := context.Background()
	exists, err := service.HasUsers(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "fresh install already has users")
	u, token, err := service.CreateFirstAccount(ctx, "alice", "alice-password")
	require.NoError(t, err, "create first account: %+v token=%q err=%v", u, token, err)
	require.NotEmpty(t, token, "create first account: %+v token=%q err=%v", u, token, err)
	got, err := service.Authenticate(ctx, token)
	require.NoError(t, err, "authenticate first session: %+v err=%v", got, err)
	assert.Equal(t, u.ID, got.ID, "authenticate first session: %+v err=%v", got, err)
	_, _, err = service.CreateFirstAccount(ctx, "bob", "bob-password")
	assert.ErrorIs(t, err, auth.ErrFirstAccountExists, "second first account: %v", err)
}

func setup(t *testing.T) (*persistence.PostgresStore, *auth.Service, http.Handler) {
	t.Helper()
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "auth store", store.Close)
	s := auth.New(store, time.Hour)
	return store, s, New(s, false, time.Hour)
}
func request(t *testing.T, h http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestAuthenticationAndAuthorizationAgainstPostgres(t *testing.T) {
	store, s, h := setup(t)
	ctx := context.Background()
	for _, path := range []string{"/admin/bootstrap", "/admin/users", "/admin/users/someone/reset-password"} {
		w := request(t, h, "POST", path, `{}`, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, "removed route %s status=%d", path, w.Code)
	}
	aliceHash, err := auth.HashPassword("alice-password")
	require.NoError(t, err)
	alice, err := store.CreateUserWithPassword(ctx, "alice", aliceHash, false)
	require.NoError(t, err)
	bobHash, err := auth.HashPassword("bob-password")
	require.NoError(t, err)
	bob, err := store.CreateUserWithPassword(ctx, "bob", bobHash, false)
	require.NoError(t, err)
	err = auth.AuthorizeOwner(alice, bob.ID)
	assert.Error(t, err, "cross-user owner check allowed") //nolint:testifylint // Authorization and login failures are independent security checks.
	_, err = s.Login(ctx, "alice", "wrong")
	assert.Error(t, err, "wrong password logged in") //nolint:testifylint // Wrong-password rejection is independent of the successful login below.
	w := request(t, h, "POST", "/login", `{"username":"alice","password":"alice-password"}`, nil)
	assert.Equal(t, http.StatusNoContent, w.Code, "login status=%d body=%s", w.Code, w.Body.String())
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1, "login set no session cookie: body=%s", w.Body.String())
	cookie := cookies[0]
	assert.True(t, cookie.HttpOnly, "unsafe cookie: %+v", cookie)
	assert.Greater(t, cookie.MaxAge, 0, "unsafe cookie: %+v", cookie)
	raw, err := s.Login(ctx, "bob", "bob-password")
	require.NoError(t, err)
	got, err := s.Authenticate(ctx, raw)
	require.NoError(t, err, "bob authenticate: %+v %v", got, err)
	assert.Equal(t, bob.ID, got.ID, "bob authenticate: %+v %v", got, err)
	raw, err = s.Login(ctx, "alice", "alice-password")
	require.NoError(t, err)
	err = s.Logout(ctx, raw)
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, raw)
	assert.Error(t, err, "logout did not invalidate session")
}
