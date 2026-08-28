package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestLayoutUsesBundledPinnedFrontendAssets(t *testing.T) {
	var output bytes.Buffer
	if err := Layout("Foundations", nil, "").Render(context.Background(), &output); err != nil {
		t.Fatalf("render layout: %v", err)
	}

	html := output.String()
	for _, want := range []string{
		`href="/static/vendor/pico-2.1.1.min.css"`,
		`src="/static/vendor/htmx-2.0.7.min.js"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("layout missing bundled asset %q", want)
		}
	}
	if strings.Contains(html, "cdn.jsdelivr.net") {
		t.Error("layout must not depend on the jsDelivr CDN")
	}

	for _, asset := range []struct {
		path string
		want string
	}{
		{path: "/static/vendor/pico-2.1.1.min.css", want: "Pico CSS"},
		{path: "/static/vendor/htmx-2.0.7.min.js", want: `version:"2.0.7"`},
	} {
		t.Run(asset.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, asset.path, nil)
			response := httptest.NewRecorder()
			StaticHandler().ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if !strings.Contains(response.Body.String(), asset.want) {
				t.Errorf("asset body missing %q", asset.want)
			}
		})
	}
}

func TestAppStylesExposeMouseionFoundations(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()
	StaticHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	css := response.Body.String()
	for _, want := range []string{
		"--mouseion-color-surface:",
		"--mouseion-color-text-muted:",
		"--mouseion-color-border:",
		"--mouseion-color-accent:",
		"--mouseion-color-success:",
		"--mouseion-color-warning:",
		"--mouseion-color-danger:",
		"--mouseion-font-application:",
		"--mouseion-font-reading:",
		"--mouseion-space-1:",
		"--mouseion-space-8:",
		"--mouseion-width-reading:",
		"--mouseion-width-data:",
		".bibliographic-title",
		".reading-text",
		".metadata",
		".numeric",
		"@media (max-width: 40rem)",
		"@media (min-width: 72rem)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("application CSS missing foundation %q", want)
		}
	}
}

func TestLibraryAppliesBibliographicAndMetadataRoles(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{
		Source: domain.SourceMaterial{
			ID:        "book-1",
			Title:     "Die Verwandlung",
			Language:  "de",
			CreatedAt: time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC),
		},
		AnalysisStatus: "ready",
	}
	if err := LibraryPage(domain.User{Username: "learner"}, "csrf", []domain.SourceMaterialSummary{book}, "").Render(context.Background(), &output); err != nil {
		t.Fatalf("render library: %v", err)
	}

	html := output.String()
	if !strings.Contains(html, `<h2 class="bibliographic-title">`) {
		t.Error("book title must use the bibliographic typography role")
	}
	if !strings.Contains(html, `<p class="metadata">`) {
		t.Error("book metadata must use the metadata typography role")
	}
}
