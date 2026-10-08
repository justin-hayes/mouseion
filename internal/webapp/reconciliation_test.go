package webapp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReconciliation_NoRawHexInTemplates guards against designers inlining raw
// hex colors directly in Templ markup instead of using the documented CSS
// tokens. All color decisions belong in the bundled stylesheet tokens.
func TestReconciliation_NoRawHexInTemplates(t *testing.T) {
	matches, err := filepath.Glob("*.templ")
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no templ files found: %v", err)
	hexColor := regexp.MustCompile(`#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b`)
	for _, path := range matches {
		data, err := os.ReadFile(path) //nolint:gosec // filepath.Glob only returns checked-in template paths.
		require.NoError(t, err, "read %s", path)
		for _, loc := range hexColor.FindAllIndex(data, -1) {
			assert.Fail(t, "%s contains raw hex color %q; use documented CSS tokens", path, data[loc[0]:loc[1]])
		}
	}
}

// TestReconciliation_DeadTemplatesRemoved pins the residual-consistency result
// of issue #381: the unreachable Dashboard and direct KnownVocabPage templates
// are gone from generated code, and both GET / and GET /known-vocab are now
// handler-level redirects rather than rendered pages.
func TestReconciliation_DeadTemplatesRemoved(t *testing.T) {
	files, err := filepath.Glob("*_templ.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		require.NoError(t, readErr, "read %s", file)
		for _, dead := range []string{`func Dashboard(`, `func KnownVocabPage(`} {
			assert.False(t, strings.Contains(string(data), dead), "generated %s still contains removed template %q", file, dead)
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

func TestReconciliation_KnownVocabRedirectsToVocabulary(t *testing.T) {
	h := &Handler{}
	for _, path := range []string{"/known-vocab", "/known-vocab?language=de"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.knownVocabPage(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
			assert.Equal(t, http.StatusSeeOther, rec.Code)
			assert.Equal(t, "/vocabulary", rec.Header().Get("Location"))
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
	matches, err := filepath.Glob("*.templ")
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no templ files found: %v", err)
	todo := regexp.MustCompile(`(?i)\b(todo|fixme|xxx)\b`)
	for _, path := range matches {
		data, err := os.ReadFile(path) //nolint:gosec // filepath.Glob only returns checked-in template paths.
		require.NoError(t, err, "read %s", path)
		for _, loc := range todo.FindAllIndex(data, -1) {
			assert.Fail(t, "%s contains placeholder marker %q", path, data[loc[0]:loc[1]])
		}
	}
}
