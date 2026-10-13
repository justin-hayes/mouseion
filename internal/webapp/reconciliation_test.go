package webapp

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openPackageRoot confines reads to this package's source directory.
func openPackageRoot(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(".")
	require.NoError(t, err)
	testutil.Cleanup(t, "package root", root.Close)
	return root
}

// TestReconciliation_NoRawHexInTemplates guards against designers inlining raw
// hex colors directly in Templ markup instead of using the documented CSS
// tokens. All color decisions belong in the bundled stylesheet tokens.
func TestReconciliation_NoRawHexInTemplates(t *testing.T) {
	root := openPackageRoot(t)
	matches, err := fs.Glob(root.FS(), "*.templ")
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no templ files found: %v", err)
	hexColor := regexp.MustCompile(`#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b`)
	for _, path := range matches {
		data, err := root.ReadFile(path)
		require.NoError(t, err, "read %s", path)
		for _, loc := range hexColor.FindAllIndex(data, -1) {
			assert.Fail(t, fmt.Sprintf("%s contains raw hex color %q; use documented CSS tokens", path, data[loc[0]:loc[1]]))
		}
	}
}

// TestReconciliation_DeadTemplatesRemoved pins the residual-consistency result
// of issue #381: the unreachable Dashboard and direct KnownVocabPage templates
// are gone from generated code, and GET / is now a
// handler-level redirect, and GET /known-vocab is retired without a redirect.
func TestReconciliation_DeadTemplatesRemoved(t *testing.T) {
	root := openPackageRoot(t)
	files, err := fs.Glob(root.FS(), "*_templ.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		data, readErr := root.ReadFile(file)
		require.NoError(t, readErr, "read %s", file)
		for _, dead := range []string{`func Dashboard(`, `func KnownVocabPage(`, `func VocabularyPageWithResult(`, `func KnownVocabImportStatus(`} {
			assert.NotContains(t, string(data), dead, "generated %s still contains removed template %q", file, dead)
		}
	}
}

func TestReconciliation_HomeRedirectsToLibrary(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.dashboard(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/library", rec.Header().Get("Location"))
}

func TestReconciliation_KnownVocabAliasIsRetiredWithoutRedirect(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	for _, path := range []string{"/known-vocab", "/known-vocab?language=de"} {
		t.Run(path, func(t *testing.T) {
			response := performReadingRequest(t, h, http.MethodGet, path, nil, cookies)
			assert.Equal(t, http.StatusNotFound, response.Code)
			assert.Empty(t, response.Header().Get("Location"))
		})
	}
}

// TestReconciliation_ImportRoutesAreRetiredWithoutRedirect pins ADR 0092: the
// import page, its submission, and its status endpoint are gone for a signed-in
// learner and 404 without a redirect, exactly as /known-vocab does.
func TestReconciliation_ImportRoutesAreRetiredWithoutRedirect(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)
	tests := []struct {
		name   string
		method string
		path   string
		form   url.Values
	}{
		{name: "import page", method: http.MethodGet, path: "/vocabulary/import"},
		{name: "import submission", method: http.MethodPost, path: "/vocabulary/import", form: url.Values{"csrf_token": {csrf}}},
		{name: "import status", method: http.MethodGet, path: "/vocabulary/imports/7/status"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performReadingRequest(t, h, test.method, test.path, test.form, cookies)
			assert.Equal(t, http.StatusNotFound, response.Code)
			assert.Empty(t, response.Header().Get("Location"))
		})
	}
}

func TestRedirectRejectsExternalDestination(t *testing.T) {
	recorder := httptest.NewRecorder()
	redirect(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), "https://evil.example/phishing")
	assert.Equal(t, http.StatusSeeOther, recorder.Code)
	assert.Equal(t, "/", recorder.Header().Get("Location"))
}

// TestReconciliation_NoTODOInTemplates blocks placeholder markers from being
// reintroduced into the shipped templates as residual design debt.
func TestReconciliation_NoTODOInTemplates(t *testing.T) {
	root := openPackageRoot(t)
	matches, err := fs.Glob(root.FS(), "*.templ")
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no templ files found: %v", err)
	todo := regexp.MustCompile(`(?i)\b(todo|fixme|xxx)\b`)
	for _, path := range matches {
		data, err := root.ReadFile(path)
		require.NoError(t, err, "read %s", path)
		for _, loc := range todo.FindAllIndex(data, -1) {
			assert.Fail(t, fmt.Sprintf("%s contains placeholder marker %q", path, data[loc[0]:loc[1]]))
		}
	}
}
