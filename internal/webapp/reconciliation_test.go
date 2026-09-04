package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// redirectStore is a minimal Store used only by reconciliation handler tests.
// It embeds the full Store interface so the Handler construction stays
// identical to production, but only language-list methods are reachable here.
type redirectStore struct {
	Store
	languages []domain.StudyLanguage
}

func (s *redirectStore) ListLanguageProfiles(context.Context, string) ([]domain.LanguageProfile, error) {
	return nil, nil
}
func (s *redirectStore) ListStudyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return s.languages, nil
}

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

func TestReconciliation_KnownVocabRedirectsToSettings(t *testing.T) {
	h := &Handler{services: Services{Store: &redirectStore{languages: []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}}}}
	cases := []struct {
		name      string
		path      string
		want      string
		languages []domain.StudyLanguage
	}{
		{name: "no language preserved", path: "/known-vocab", want: "/settings#known-vocabulary"},
		{name: "unknown language falls back", path: "/known-vocab?language=fr", want: "/settings#known-vocabulary"},
		{name: "library language preserved", path: "/known-vocab?language=de", want: "/settings?language=de#known-vocabulary",
			languages: []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h.services.Store = &redirectStore{languages: tc.languages}
			rec := httptest.NewRecorder()
			h.knownVocabPage(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("GET %s status=%d want %d", tc.path, rec.Code, http.StatusSeeOther)
			}
			if got := rec.Header().Get("Location"); got != tc.want {
				t.Fatalf("GET %s Location=%q want %q", tc.path, got, tc.want)
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
