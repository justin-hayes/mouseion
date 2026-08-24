//go:build integration

package webauth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestFirstAccountAuthLifecycleAgainstPostgres(t *testing.T) {
	_, service, _ := setup(t)
	ctx := context.Background()
	if exists, err := service.HasUsers(ctx); err != nil || exists {
		t.Fatalf("fresh users exists=%v err=%v", exists, err)
	}
	u, token, err := service.CreateFirstAccount(ctx, "alice", "alice-password")
	if err != nil || token == "" {
		t.Fatalf("create first account: %+v token=%q err=%v", u, token, err)
	}
	if got, err := service.Authenticate(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("authenticate first session: %+v err=%v", got, err)
	}
	if _, _, err = service.CreateFirstAccount(ctx, "bob", "bob-password"); !errors.Is(err, auth.ErrFirstAccountExists) {
		t.Fatalf("second first account: %v", err)
	}
}

func testURL() string {
	if v := os.Getenv("MOUSEION_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
}
func setup(t *testing.T) (*persistence.PostgresStore, *auth.Service, http.Handler) {
	t.Helper()
	ctx := context.Background()
	url := testURL()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
		conn.Release()
		pool.Close()
	})
	if _, err = conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = persistence.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s := auth.New(store, time.Hour)
	return store, s, New(s, false, time.Hour)
}
func request(t *testing.T, h http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
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
		if w := request(t, h, "POST", path, `{}`, nil); w.Code != http.StatusNotFound {
			t.Fatalf("removed route %s status=%d", path, w.Code)
		}
	}
	aliceHash, err := auth.HashPassword("alice-password")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := store.CreateUserWithPassword(ctx, "alice", aliceHash, false)
	if err != nil {
		t.Fatal(err)
	}
	bobHash, err := auth.HashPassword("bob-password")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUserWithPassword(ctx, "bob", bobHash, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = auth.AuthorizeOwner(alice, bob.ID); err == nil {
		t.Fatal("cross-user owner check allowed")
	}
	if _, err = s.Login(ctx, "alice", "wrong"); err == nil {
		t.Fatal("wrong password logged in")
	}
	w := request(t, h, "POST", "/login", `{"username":"alice","password":"alice-password"}`, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.MaxAge <= 0 {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	raw, err := s.Login(ctx, "bob", "bob-password")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Authenticate(ctx, raw); err != nil || got.ID != bob.ID {
		t.Fatalf("bob authenticate: %+v %v", got, err)
	}
	raw, err = s.Login(ctx, "alice", "alice-password")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Logout(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, raw); err == nil {
		t.Fatal("logout did not invalidate session")
	}
}
