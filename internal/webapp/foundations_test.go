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
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/riverqueue/river/rivertype"
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
	for _, pattern := range []string{`class="page-header"`, `class="resource-card"`, `class="status-badge`} {
		if !strings.Contains(html, pattern) {
			t.Errorf("library missing shared pattern %q", pattern)
		}
	}
	if !strings.Contains(html, `<h2 class="bibliographic-title">`) {
		t.Error("book title must use the bibliographic typography role")
	}
	if !strings.Contains(html, `<p class="metadata">`) {
		t.Error("book metadata must use the metadata typography role")
	}
}

func TestLibraryUsesSharedFeedbackAndEmptyState(t *testing.T) {
	var output bytes.Buffer
	if err := LibraryPage(domain.User{Username: "learner"}, "csrf", nil, "Book added").Render(context.Background(), &output); err != nil {
		t.Fatalf("render library: %v", err)
	}

	html := output.String()
	for _, pattern := range []string{
		`class="feedback feedback--success"`,
		`class="empty-state"`,
		`class="empty-state__actions"`,
	} {
		if !strings.Contains(html, pattern) {
			t.Errorf("library missing shared pattern %q", pattern)
		}
	}
}

func TestLayoutExposesAccessibleApplicationShell(t *testing.T) {
	var output bytes.Buffer
	if err := Layout("Mouseion", nil, "csrf-token").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{`class="skip-link"`, `href="#main-content"`, `aria-label="Primary navigation"`, `id="main-content"`, `tabindex="-1"`, `Request failed:`, `aria-label="Deck preparation progress"`} {
		if !strings.Contains(output.String(), pattern) {
			t.Errorf("application shell missing %q: %s", pattern, output.String())
		}
	}
}

func TestEnhancedUploadAndProgressKeepAccessibleNativeContracts(t *testing.T) {
	var upload bytes.Buffer
	if err := KnownVocabPageWithResult(domain.User{}, "csrf", nil, "de", nil, nil, "").Render(context.Background(), &upload); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`method="post"`, `action="/known-vocab/import"`, `enctype="multipart/form-data"`, `hx-encoding="multipart/form-data"`} {
		if !strings.Contains(upload.String(), want) {
			t.Errorf("known-vocabulary upload missing %q: %s", want, upload.String())
		}
	}

	var progress bytes.Buffer
	status := enrichmentjob.Status{ID: 7, Completed: 2, Total: 5, State: rivertype.JobStateRunning}
	if err := EnrichmentJobStatus(status, "csrf").Render(context.Background(), &progress); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`aria-label="Contextual translation progress"`,
		`method="post"`,
		`action="/enrichment-jobs/7/cancel"`,
		`hx-post="/enrichment-jobs/7/cancel"`,
	} {
		if !strings.Contains(progress.String(), want) {
			t.Errorf("enrichment status missing %q: %s", want, progress.String())
		}
	}
}
