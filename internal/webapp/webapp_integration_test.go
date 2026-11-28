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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingAnalysis struct {
	fixtures.Analysis
	owner, source, scope string
	calls                int
}
type staticCapabilities struct {
	value analyzer.Capabilities
	err   error
}

func (s staticCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return s.value, s.err
}

func readyGerman() staticCapabilities {
	return staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
}

func createAccount(t *testing.T, ctx context.Context, store *persistence.PostgresStore, username, password string) domain.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	u, err := store.CreateUserWithPassword(ctx, username, hash, false)
	require.NoError(t, err)
	return u
}

func (r *recordingAnalysis) SubmitToReadBookAnalysis(_ context.Context, owner, bookID, source string) (analysis.Handle, error) {
	r.owner, r.source, r.scope = owner, source, ""
	r.calls++
	return analysis.Handle{ID: 42, DisplayNumber: 1}, nil
}

func (r *recordingAnalysis) Get(_ context.Context, owner string, id int64) (analysis.Status, error) {
	if owner != r.owner || id != 42 {
		return analysis.Status{}, analysis.ErrNotFound
	}
	return analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, Progress: 100, CorpusID: "corpus-result", Attempt: 1}, nil
}

func TestFirstAccountOnboardingAndExistingLogin(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "first-account-web-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, Capabilities: readyGerman(), SessionLifetime: time.Hour})

	page := perform(t, h, "GET", "/login", nil, nil)
	assert.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Create your account", "body=%s", page.Body.String())
	assert.NotContains(t, page.Body.String(), `action="/login"`, "body=%s", page.Body.String())
	csrf := hiddenToken(t, page.Body.String())
	csrfCookieValue := cookieNamed(t, page.Result().Cookies(), csrfCookie)
	got := perform(t, h, "POST", "/onboarding", url.Values{"username": {"alice"}, "password": {"alice-password"}}, nil)
	assert.Equal(t, http.StatusForbidden, got.Code)
	created := perform(t, h, "POST", "/onboarding", url.Values{"csrf_token": {csrf}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{csrfCookieValue})
	assert.Equal(t, http.StatusSeeOther, created.Code)
	assert.Equal(t, "/", created.Header().Get("Location"))
	assert.NotEmpty(t, cookieNamed(t, created.Result().Cookies(), webauth.CookieName).Value)

	page = perform(t, h, "GET", "/login", nil, nil)
	assert.Equal(t, http.StatusOK, page.Code)
	assert.NotContains(t, page.Body.String(), "Create your account", "body=%s", page.Body.String())
	assert.Contains(t, page.Body.String(), `action="/login"`, "body=%s", page.Body.String())
	navigationRequest := httptest.NewRequestWithContext(t.Context(), "GET", "/reading?sort=title", nil)
	navigationRequest.Header.Set("Accept", "text/html")
	navigationRequest.Header.Set("Sec-Fetch-Mode", "navigate")
	navigation := httptest.NewRecorder()
	h.ServeHTTP(navigation, navigationRequest)
	assert.Equal(t, http.StatusSeeOther, navigation.Code)
	assert.Equal(t, "/login?next=%2Freading%3Fsort%3Dtitle", navigation.Header().Get("Location"))
	returnPage := perform(t, h, "GET", navigation.Header().Get("Location"), nil, nil)
	assert.Contains(t, returnPage.Body.String(), `name="next" value="/reading?sort=title"`, "body=%s", returnPage.Body.String())
	returnCSRF := hiddenToken(t, returnPage.Body.String())
	returnCSRFCookie := cookieNamed(t, returnPage.Result().Cookies(), csrfCookie)
	returnedLogin := perform(t, h, "POST", "/login", url.Values{"csrf_token": {returnCSRF}, "next": {"/reading?sort=title"}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{returnCSRFCookie})
	assert.Equal(t, http.StatusSeeOther, returnedLogin.Code)
	assert.Equal(t, "/reading?sort=title", returnedLogin.Header().Get("Location"))
	csrf = hiddenToken(t, page.Body.String())
	csrfCookieValue = cookieNamed(t, page.Result().Cookies(), csrfCookie)
	blocked := perform(t, h, "POST", "/onboarding", url.Values{"csrf_token": {csrf}, "username": {"bob"}, "password": {"bob-password"}}, []*http.Cookie{csrfCookieValue})
	assert.Equal(t, http.StatusNotFound, blocked.Code)
	login := perform(t, h, "POST", "/login", url.Values{"csrf_token": {csrf}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{csrfCookieValue})
	assert.Equal(t, http.StatusSeeOther, login.Code)
	assert.Equal(t, "/", login.Header().Get("Location"))
}

func TestAuthenticatedNeedsLanguageBookIsRetiredFromMyBooks(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "needs-language-actionable-integration-secret")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner := createAccount(t, ctx, store, "needs-language-owner", "owner-password")
	unchosen, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Awaiting a language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	testutil.SeedLegacyInboxDisposition(t, ctx, store.Pool(), owner.ID, unchosen.ID)
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, owner.Username, "owner-password")
	page := perform(t, h, http.MethodGet, "/reading?needs-language", nil, cookies)
	assert.Equal(t, http.StatusNotFound, page.Code, "the needs-language browse state is retired")

	browse, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "", "", false, 0, 20)
	require.NoError(t, err)
	assert.Empty(t, browse.Items, "an unchosen language-less Book is not in Reading")
	studyLanguages, err := store.ListStudyLanguages(ctx, owner.ID)
	require.NoError(t, err)
	assert.Empty(t, studyLanguages, "a retired Inbox Book produces no study-language scope")
}

func loginCookies(t *testing.T, h http.Handler, username, password string) ([]*http.Cookie, string) {
	t.Helper()
	page := perform(t, h, "GET", "/login", nil, nil)
	token := hiddenToken(t, page.Body.String())
	csrfCookieValue := cookieNamed(t, page.Result().Cookies(), csrfCookie)
	response := perform(t, h, "POST", "/login", url.Values{"csrf_token": {token}, "username": {username}, "password": {password}}, []*http.Cookie{csrfCookieValue})
	assert.Equal(t, http.StatusSeeOther, response.Code, "login %s=%d %s", username, response.Code, response.Body.String())
	csrfCookieValue = cookieNamed(t, response.Result().Cookies(), csrfCookie)
	return []*http.Cookie{csrfCookieValue, cookieNamed(t, response.Result().Cookies(), webauth.CookieName)}, csrfCookieValue.Value
}
func perform(t *testing.T, h http.Handler, method, path string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return performWithHeader(t, h, method, path, form, cookies, "", "")
}
func performWithHeader(t *testing.T, h http.Handler, method, path string, form url.Values, cookies []*http.Cookie, header, value string) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequestWithContext(t.Context(), method, path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if header != "" {
		r.Header.Set(header, value)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if location, ok := prefixedMyBooksRedirect(path, w); ok && method == http.MethodGet {
		return performWithHeader(t, h, method, location, form, cookies, header, value)
	}
	return w
}
func hiddenToken(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	require.Len(t, match, 2, "csrf token absent: %s", body)
	return match[1]
}
func cookieNamed(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	require.Failf(t, "cookie absent", "cookie %s absent", name)
	return nil
}
func testEPUBVariant(t *testing.T, identifier, title, text string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"mimetype": "application/epub+zip", "META-INF/container.xml": `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`, `OEBPS/content.opf`: fmt.Sprintf(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">%s</dc:identifier><dc:title>%s</dc:title></metadata><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter"/></spine></package>`, identifier, title), `OEBPS/chapter.xhtml`: fmt.Sprintf(`<html xmlns="http://www.w3.org/1999/xhtml"><body><p>%s</p></body></html>`, text)}
	for name, content := range files {
		w, err := z.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, z.Close())
	return b.Bytes()
}
