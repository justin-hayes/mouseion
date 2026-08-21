//go:build integration

package webauth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func testURL() string {
	if v := os.Getenv("MOUSEION_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
}
func setup(t *testing.T) (*auth.Service, http.Handler) {
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
	return s, New(s, false, time.Hour)
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
	s, h := setup(t)
	ctx := context.Background()
	if w := request(t, h, "POST", "/admin/users", `{"username":"x","password":"x"}`, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", w.Code)
	}
	admin, err := s.BootstrapAdmin(ctx, "admin", "admin-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.BootstrapAdmin(ctx, "second", "password"); err == nil {
		t.Fatal("second bootstrap succeeded")
	}
	alice, err := s.CreateUser(ctx, admin.ID, "alice", "alice-password", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.CreateUser(ctx, admin.ID, "bob", "bob-password", false)
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
	if w = request(t, h, "POST", "/admin/users", `{"username":"mallory","password":"password"}`, cookie); w.Code != http.StatusForbidden {
		t.Fatalf("non-admin status=%d body=%s", w.Code, w.Body.String())
	}
	raw, err := s.Login(ctx, "bob", "bob-password")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPassword(ctx, admin.ID, bob.ID, "new-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, raw); err == nil {
		t.Fatal("password reset did not invalidate sessions")
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
