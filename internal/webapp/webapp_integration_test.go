//go:build integration

package webapp

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

type recordingAnalysis struct{ owner, source string }

func (r *recordingAnalysis) SubmitAnalysis(_ context.Context, owner, source string) (analysis.Handle, error) {
	r.owner, r.source = owner, source
	return analysis.Handle{ID: 42}, nil
}

func integrationDatabaseURL() string {
	if v := os.Getenv("MOUSEION_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
}

func TestLoginBrowseAcquireAndOwnerScoping(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "webapp-integration-secret")
	ctx := context.Background()
	databaseURL := integrationDatabaseURL()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
	if _, err = conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = persistence.Migrate(databaseURL); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	admin, err := authService.BootstrapAdmin(ctx, "admin", "admin-password")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := authService.CreateUser(ctx, admin.ID, "alice", "alice-password", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := authService.CreateUser(ctx, admin.ID, "bob", "bob-password", false)
	if err != nil {
		t.Fatal(err)
	}
	epubBytes := testEPUB(t)
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/opds":
			w.Header().Set("Content-Type", "application/atom+xml")
			fmt.Fprintf(w, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Library</title><entry><id>book-1</id><title>Test Book</title><link rel="%s" type="%s" href="/book.epub"/></entry></feed>`, opds.AcquisitionRel, opds.EPUBMediaType)
		case "/book.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			_, _ = w.Write(epubBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer catalog.Close()
	connection, err := store.CreateOpdsConnection(ctx, domain.OpdsConnection{OwnerID: alice.ID, Name: "Library", URL: catalog.URL + "/opds", Language: "de"})
	if err != nil {
		t.Fatal(err)
	}
	bobConnection, err := store.CreateOpdsConnection(ctx, domain.OpdsConnection{OwnerID: bob.ID, Name: "Private", URL: catalog.URL + "/opds", Language: "de"})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingAnalysis{}
	opdsService := opds.NewService(store, epub.NewService(store), catalog.Client())
	webAuth := webauth.New(authService, false, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webAuth, Store: store, OPDS: opdsService, Analysis: recorder, SessionLifetime: time.Hour})
	loginPage := perform(t, h, "GET", "/login", nil, nil)
	csrf := hiddenToken(t, loginPage.Body.String())
	csrfCookieValue := cookieNamed(t, loginPage.Result().Cookies(), csrfCookie)
	form := url.Values{"csrf_token": {csrf}, "username": {"alice"}, "password": {"alice-password"}}
	login := perform(t, h, "POST", "/login", form, []*http.Cookie{csrfCookieValue})
	if login.Code != http.StatusSeeOther {
		t.Fatalf("login=%d %s", login.Code, login.Body.String())
	}
	session := cookieNamed(t, login.Result().Cookies(), webauth.CookieName)
	csrfCookieValue = cookieNamed(t, login.Result().Cookies(), csrfCookie)
	csrf = csrfCookieValue.Value
	cookies := []*http.Cookie{csrfCookieValue, session}
	browse := perform(t, h, "GET", "/opds/browse?connection="+connection.ID, nil, cookies)
	if browse.Code != 200 || !strings.Contains(browse.Body.String(), "Test Book") {
		t.Fatalf("browse=%d %s", browse.Code, browse.Body.String())
	}
	denied := perform(t, h, "GET", "/opds/browse?connection="+bobConnection.ID, nil, cookies)
	if denied.Code == 200 {
		t.Fatal("Alice browsed Bob's connection")
	}
	acquireForm := url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "entry_id": {"book-1"}, "title": {"Test Book"}, "href": {catalog.URL + "/book.epub"}}
	acquired := perform(t, h, "POST", "/opds/acquire", acquireForm, cookies)
	if acquired.Code != http.StatusSeeOther {
		t.Fatalf("acquire=%d %s", acquired.Code, acquired.Body.String())
	}
	if recorder.owner != alice.ID || recorder.source == "" {
		t.Fatalf("analysis wiring owner=%q source=%q", recorder.owner, recorder.source)
	}
}

func perform(t *testing.T, h http.Handler, method, path string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func hiddenToken(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("csrf token absent: %s", body)
	}
	return match[1]
}
func cookieNamed(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cookie %s absent", name)
	return nil
}
func testEPUB(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"mimetype": "application/epub+zip", "META-INF/container.xml": `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`, `OEBPS/content.opf`: `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">book-1</dc:identifier><dc:title>Test Book</dc:title></metadata><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter"/></spine></package>`, `OEBPS/chapter.xhtml`: `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>Hallo Welt.</p></body></html>`}
	for name, content := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
