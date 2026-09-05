package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
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
		".table-region",
		"overflow-x: auto",
		".table-region td",
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
	if err := LibraryPage(domain.User{Username: "learner"}, "csrf", []domain.SourceMaterialSummary{book}, "", "", false).Render(context.Background(), &output); err != nil {
		t.Fatalf("render library: %v", err)
	}

	html := output.String()
	for _, pattern := range []string{`class="page-header"`, `class="resource-card library-book"`, `class="status-badge`} {
		if !strings.Contains(html, pattern) {
			t.Errorf("library missing shared pattern %q", pattern)
		}
	}
	if !strings.Contains(html, `class="bibliographic-title"><a href="/books/book-1">`) {
		t.Error("book title must use the bibliographic typography role")
	}
	if !strings.Contains(html, `<p class="metadata">`) {
		t.Error("book metadata must use the metadata typography role")
	}
}

func TestLibraryUsesSharedFeedbackAndEmptyState(t *testing.T) {
	var output bytes.Buffer
	if err := LibraryPage(domain.User{Username: "learner"}, "csrf", nil, "Book added", "", false).Render(context.Background(), &output); err != nil {
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
	for _, pattern := range []string{`class="skip-link"`, `href="#main-content"`, `aria-label="Primary navigation"`, `id="main-content"`, `tabindex="-1"`, `could not be updated:`, `aria-label="Deck preparation progress"`} {
		if !strings.Contains(output.String(), pattern) {
			t.Errorf("application shell missing %q: %s", pattern, output.String())
		}
	}
}

func TestEnhancedUploadAndProgressKeepAccessibleNativeContracts(t *testing.T) {
	var upload bytes.Buffer
	if err := VocabularyPageWithResult(domain.User{}, "csrf", []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, "de", nil, nil, "").Render(context.Background(), &upload); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`method="post"`, `action="/vocabulary/import"`, `enctype="multipart/form-data"`, `hx-encoding="multipart/form-data"`, `id="vocabulary-results"`} {
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
		`aria-label="Contextual translations: Running progress"`,
		`data-workflow="contextual translation"`,
		`hx-trigger="every 2s"`,
		`method="post"`,
		`action="/enrichment-jobs/7/cancel"`,
		`hx-post="/enrichment-jobs/7/cancel"`,
	} {
		if !strings.Contains(progress.String(), want) {
			t.Errorf("enrichment status missing %q: %s", want, progress.String())
		}
	}
}

func TestVocabularyPageListsOnlyDerivedLanguages(t *testing.T) {
	var output bytes.Buffer
	if err := VocabularyPageWithResult(domain.User{}, "csrf", []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, "de", nil, nil, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `<select name="language"`) || !strings.Contains(html, `<option value="de" selected`) {
		t.Fatalf("selected derived language missing: %s", html)
	}
	if strings.Contains(html, "<datalist") || strings.Contains(html, "return_to") {
		t.Fatalf("Vocabulary page exposes an unrestricted language or return_to field: %s", html)
	}
}

func TestVocabularyPageEmptyLibraryPointsToConnectionsAndHidesImport(t *testing.T) {
	var output bytes.Buffer
	if err := VocabularyPageWithResult(domain.User{}, "csrf", nil, "", nil, nil, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, "No study languages yet") || !strings.Contains(html, `href="/connections"`) {
		t.Fatalf("empty Vocabulary state missing catalogue guidance: %s", html)
	}
	if strings.Contains(html, `enctype="multipart/form-data"`) || strings.Contains(html, "Choose a study language") {
		t.Fatalf("empty Vocabulary state exposes import controls: %s", html)
	}
}

func TestOperationalStatusStopsPollingAtTerminalStates(t *testing.T) {
	var output bytes.Buffer
	if err := EnrichmentJobStatus(enrichmentjob.Status{ID: 7, State: rivertype.JobStateCompleted, Completed: 5, Total: 5}, "csrf").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "hx-trigger") || strings.Contains(output.String(), "hx-get") {
		t.Fatalf("completed enrichment status still polls: %s", output.String())
	}

	output.Reset()
	if err := KnownVocabImportStatus(knownvocab.Status{ID: 12, State: rivertype.JobStateCancelled, Language: "de"}).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "hx-trigger") || strings.Contains(output.String(), "hx-get") {
		t.Fatalf("cancelled vocabulary import still polls: %s", output.String())
	}
}

func TestKnownVocabImportTargetsVocabulary(t *testing.T) {
	if got := knownVocabImportAction("de"); got != "/vocabulary/import?language=de" {
		t.Fatalf("import target = %q", got)
	}
	if got := knownVocabImportAction(""); got != "/vocabulary/import" {
		t.Fatalf("empty import target = %q", got)
	}
	if got := knownVocabImportRecoveryTarget("de"); got != "/vocabulary?language=de" {
		t.Fatalf("recovery target = %q", got)
	}
	if got := knownVocabImportRecoveryTarget(""); got != "/vocabulary" {
		t.Fatalf("empty recovery target = %q", got)
	}
}

func TestKnownVocabTerminalStatesExplainResultsAndUseContainedTables(t *testing.T) {
	var processing bytes.Buffer
	if err := KnownVocabImportStatus(knownvocab.Status{ID: 12, Language: "de", State: rivertype.JobStateRunning, Processed: 1, Total: 3}).Render(context.Background(), &processing); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(processing.String(), "Processing") || !strings.Contains(processing.String(), "You can leave this page") || !strings.Contains(processing.String(), `aria-busy="true"`) || !strings.Contains(processing.String(), `hx-get="/vocabulary/imports/12/status"`) {
		t.Fatalf("processing status missing safe-leave contract: %s", processing.String())
	}

	for _, test := range []struct {
		name  string
		state rivertype.JobState
		want  string
		table bool
	}{
		{name: "partial", state: rivertype.JobStateCompleted, want: "partial rejection", table: true},
		{name: "failed", state: rivertype.JobStateDiscarded, want: "Import failed"},
		{name: "cancelled", state: rivertype.JobStateCancelled, want: "Import cancelled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			status := knownvocab.Status{ID: 12, Language: "de", State: test.state, Imported: 2, AlreadyKnown: 1, Rejected: []knownvocab.Rejection{{Row: 3, Original: "bad\tline", Error: "expected exactly one lemma"}}}
			if err := KnownVocabImportStatus(status).Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			html := output.String()
			if !strings.Contains(html, test.want) || !strings.Contains(html, "2 new") || !strings.Contains(html, "1 duplicates") || (test.table != strings.Contains(html, `class="table-region"`)) {
				t.Fatalf("status missing result contract: %s", html)
			}
			if test.state == rivertype.JobStateDiscarded || test.state == rivertype.JobStateCancelled {
				for _, want := range []string{
					`Selected language: <code>de</code>`,
					`href="/vocabulary?language=de"`,
					"Return to Vocabulary to retry the import",
				} {
					if !strings.Contains(html, want) {
						t.Errorf("recovery status missing %q: %s", want, html)
					}
				}
			}
		})
	}
}

type knownVocabContextStore struct {
	Store
}

func (knownVocabContextStore) ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error) {
	return []domain.KnownVocabulary{}, nil
}
func (knownVocabContextStore) ListStudyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil
}

func TestKnownVocabImportLanguageIgnoresReturnTo(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/vocabulary/import?language=it&return_to=known-vocab", nil)
	if gotLanguage := knownVocabImportLanguage(request); gotLanguage != "it" {
		t.Fatalf("URL language = %q", gotLanguage)
	}
	request.Form = url.Values{"language": {" de "}, "return_to": {" settings "}}
	if gotLanguage := knownVocabImportLanguage(request); gotLanguage != "de" {
		t.Fatalf("form language = %q", gotLanguage)
	}
}

func TestKnownVocabImportParseFailuresPreserveVocabularyContext(t *testing.T) {
	h := &Handler{services: Services{Store: knownVocabContextStore{}}}

	var oversized bytes.Buffer
	oversized.WriteString("--known-vocabulary\r\nContent-Disposition: form-data; name=\"vocabulary_file\"; filename=\"words.txt\"\r\nContent-Type: text/plain\r\n\r\n")
	oversized.Write(bytes.Repeat([]byte("word\n"), 1<<20))

	for _, test := range []struct {
		name string
		body *bytes.Reader
	}{
		{name: "malformed", body: bytes.NewReader([]byte("not a multipart body"))},
		{name: "oversized", body: bytes.NewReader(oversized.Bytes())},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/vocabulary/import?language=de", test.body)
			request.Header.Set("Content-Type", `multipart/form-data; boundary=known-vocabulary`)
			response := httptest.NewRecorder()
			h.importKnownVocab(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
			}
			for _, want := range []string{
				"The import is too large or could not be read.",
				"<strong>German</strong> <code>de</code>",
				`id="vocabulary-results"`,
			} {
				if !strings.Contains(response.Body.String(), want) {
					t.Errorf("parse failure response missing %q: %s", want, response.Body.String())
				}
			}
		})
	}
}
