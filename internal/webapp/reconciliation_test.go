package webapp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestReconciliation_NoRawHexInTemplates guards against designers inlining raw
// hex colors directly in Templ markup instead of using the documented CSS
// tokens. All color decisions belong in the bundled stylesheet tokens.
func TestReconciliation_NoRawHexInTemplates(t *testing.T) {
	matches, err := filepath.Glob("*.templ")
	if err != nil || len(matches) == 0 {
		t.Fatalf("no templ files found: %v", err)
	}
	hexColor := regexp.MustCompile(`#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b`)
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, loc := range hexColor.FindAllIndex(data, -1) {
			t.Errorf("%s contains raw hex color %q; use documented CSS tokens", path, data[loc[0]:loc[1]])
		}
	}
}

// TestReconciliation_DeadTemplatesRemoved pins the residual-consistency result
// of issue #381: the unreachable Dashboard and direct KnownVocabPage templates
// are gone from generated code, and both GET / and GET /known-vocab are now
// handler-level redirects rather than rendered pages.
func TestReconciliation_DeadTemplatesRemoved(t *testing.T) {
	data, err := os.ReadFile("views_templ.go")
	if err != nil {
		t.Fatalf("read views_templ.go: %v", err)
	}
	for _, dead := range []string{`func Dashboard(`, `func KnownVocabPage(`} {
		if strings.Contains(string(data), dead) {
			t.Errorf("generated views_templ.go still contains removed template %q", dead)
		}
	}
}

func TestReconciliation_HomeRedirectsToLibrary(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.dashboard(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET / status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/library" {
		t.Fatalf("GET / Location=%q want /library", got)
	}
}

func TestReconciliation_KnownVocabRedirectsToVocabulary(t *testing.T) {
	h := &Handler{}
	for _, path := range []string{"/known-vocab", "/known-vocab?language=de"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.knownVocabPage(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("GET %s status=%d want %d", path, rec.Code, http.StatusSeeOther)
			}
			if got := rec.Header().Get("Location"); got != "/vocabulary" {
				t.Fatalf("GET %s Location=%q want /vocabulary", path, got)
			}
		})
	}
}

// TestReconciliation_NoTODOInTemplates blocks placeholder markers from being
// reintroduced into the shipped templates as residual design debt.
func TestReconciliation_NoTODOInTemplates(t *testing.T) {
	matches, err := filepath.Glob("*.templ")
	if err != nil || len(matches) == 0 {
		t.Fatalf("no templ files found: %v", err)
	}
	todo := regexp.MustCompile(`(?i)\b(todo|fixme|xxx)\b`)
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, loc := range todo.FindAllIndex(data, -1) {
			t.Errorf("%s contains placeholder marker %q", path, data[loc[0]:loc[1]])
		}
	}
}
