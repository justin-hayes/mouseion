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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayoutUsesBundledPinnedFrontendAssets(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, Layout("Foundations", nil, "").Render(context.Background(), &output))

	html := output.String()
	for _, want := range []string{
		`href="/static/vendor/pico-2.1.1.min.css"`,
		`src="/static/vendor/htmx-2.0.7.min.js"`,
	} {
		assert.True(t, strings.Contains(html, want), "layout missing bundled asset %q", want)
	}
	assert.False(t, strings.Contains(html, "cdn.jsdelivr.net"), "layout must not depend on the jsDelivr CDN")

	for _, asset := range []struct {
		path string
		want string
	}{
		{path: "/static/vendor/pico-2.1.1.min.css", want: "Pico CSS"},
		{path: "/static/vendor/htmx-2.0.7.min.js", want: `version:"2.0.7"`},
	} {
		t.Run(asset.path, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, asset.path, nil)
			response := httptest.NewRecorder()
			StaticHandler().ServeHTTP(response, request)

			assert.Equal(t, http.StatusOK, response.Code)
			assert.True(t, strings.Contains(response.Body.String(), asset.want), "asset body missing %q", asset.want)
		})
	}
}

func TestAppStylesExposeMouseionFoundations(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()
	StaticHandler().ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)

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
		assert.True(t, strings.Contains(css, want), "application CSS missing foundation %q", want)
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
	require.NoError(t, LibraryPage(domain.User{Username: "learner"}, "csrf", []domain.SourceMaterialSummary{book}, "", "", false).Render(context.Background(), &output))

	html := output.String()
	for _, pattern := range []string{`class="page-header"`, `class="library-grid"`, `class="library-book__identity-link"`} {
		assert.True(t, strings.Contains(html, pattern), "library missing shared pattern %q", pattern)
	}
	assert.True(t, strings.Contains(html, `class="bibliographic-title">`), "book title must use the bibliographic typography role")
	assert.False(t, strings.Contains(html, `role="grid"`), "library must use native list semantics")
}

func TestLibraryUsesSharedFeedbackAndEmptyState(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, LibraryPage(domain.User{Username: "learner"}, "csrf", nil, "Book added", "", false).Render(context.Background(), &output))

	html := output.String()
	for _, pattern := range []string{
		`class="feedback feedback--success"`,
		`class="empty-state"`,
		`class="empty-state__actions"`,
	} {
		assert.True(t, strings.Contains(html, pattern), "library missing shared pattern %q", pattern)
	}
}

func TestLayoutExposesAccessibleApplicationShell(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, Layout("Mouseion", nil, "csrf-token").Render(context.Background(), &output))
	for _, pattern := range []string{`class="skip-link"`, `href="#main-content"`, `aria-label="Primary navigation"`, `id="main-content"`, `tabindex="-1"`, `could not be updated:`, `aria-label="Deck preparation progress"`} {
		assert.True(t, strings.Contains(output.String(), pattern), "application shell missing %q: %s", pattern, output.String())
	}
}

func TestEnhancedUploadAndProgressKeepAccessibleNativeContracts(t *testing.T) {
	var upload bytes.Buffer
	require.NoError(t, VocabularyPageWithResult(domain.User{}, "csrf", []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil, "de", nil, "").Render(context.Background(), &upload))
	for _, want := range []string{`method="post"`, `action="/vocabulary/import"`, `enctype="multipart/form-data"`, `hx-encoding="multipart/form-data"`, `id="vocabulary-results"`} {
		assert.True(t, strings.Contains(upload.String(), want), "known-vocabulary upload missing %q: %s", want, upload.String())
	}

	var progress bytes.Buffer
	status := enrichmentjob.Status{ID: 7, Completed: 2, Total: 5, State: rivertype.JobStateRunning}
	require.NoError(t, EnrichmentJobStatus(status, "csrf").Render(context.Background(), &progress))
	for _, want := range []string{
		`aria-label="Contextual translations: Running progress"`,
		`data-workflow="contextual translation"`,
		`hx-trigger="every 2s"`,
		`method="post"`,
		`action="/enrichment-jobs/7/cancel"`,
		`hx-post="/enrichment-jobs/7/cancel"`,
	} {
		assert.True(t, strings.Contains(progress.String(), want), "enrichment status missing %q: %s", want, progress.String())
	}
}

func TestVocabularyPageUsesActiveLanguageWithoutPicker(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, VocabularyPageWithResult(domain.User{}, "csrf", []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil, "de", nil, "").Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "Viewing <strong>German</strong> <code>de</code>"), "active language context missing: %s", html)
	assert.False(t, strings.Contains(html, `<select name="language"`), "Vocabulary page exposes a per-page language control: %s", html)
	assert.False(t, strings.Contains(html, "Known vocabulary by language"), "Vocabulary page exposes a per-page language control: %s", html)
	assert.False(t, strings.Contains(html, "return_to"), "Vocabulary page exposes a per-page language control: %s", html)
}

func TestVocabularyPageDoesNotDisplayKnownVocabulary(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, VocabularyPageWithResult(domain.User{}, "csrf", []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, []domain.StudyLanguage{{Language: "it", DisplayName: "Italian"}}, "it", nil, "").Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "Viewing <strong>Italian</strong> <code>it</code>"), "active language context missing: %s", html)
	assert.True(t, strings.Contains(html, "Importing is unavailable"), "historical language state missing: %s", html)
	assert.False(t, strings.Contains(html, "Known vocabulary</h2>"), "Vocabulary page renders a known-vocabulary heading: %s", html)
	assert.False(t, strings.Contains(html, `class="table-region"`), "Vocabulary page renders a known-vocabulary table: %s", html)
	assert.False(t, strings.Contains(html, `enctype="multipart/form-data"`), "historical language exposes an import form: %s", html)
}

func TestKnownVocabResultShowsImportSummaryWithoutKnownList(t *testing.T) {
	var output bytes.Buffer
	result := &knownvocab.ImportResult{Imported: 2, AlreadyKnown: 1, Rejected: []knownvocab.Rejection{{Row: 4, Original: "bad line", Error: "invalid lemma"}}}
	require.NoError(t, KnownVocabResult(result, "").Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "2 new")
	assert.Contains(t, html, "1 duplicates")
	assert.Contains(t, html, "Rejected rows")
	assert.NotContains(t, html, "Known vocabulary</h2>")
	assert.NotContains(t, html, "Known vocabulary\"")
}

func TestVocabularyPageEmptyLibraryPointsToCatalogsAndHidesImport(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, VocabularyPageWithResult(domain.User{}, "csrf", nil, nil, "", nil, "").Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "No study languages yet"), "empty Vocabulary state missing catalogue guidance: %s", html)
	assert.True(t, strings.Contains(html, `href="/catalogs"`), "empty Vocabulary state missing catalogue guidance: %s", html)
	assert.False(t, strings.Contains(html, `enctype="multipart/form-data"`), "empty Vocabulary state exposes import controls: %s", html)
	assert.False(t, strings.Contains(html, `name="language"`), "empty Vocabulary state exposes import controls: %s", html)
}

func TestOperationalStatusStopsPollingAtTerminalStates(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, EnrichmentJobStatus(enrichmentjob.Status{ID: 7, State: rivertype.JobStateCompleted, Completed: 5, Total: 5}, "csrf").Render(context.Background(), &output))
	assert.False(t, strings.Contains(output.String(), "hx-trigger"), "completed enrichment status still polls: %s", output.String())
	assert.False(t, strings.Contains(output.String(), "hx-get"), "completed enrichment status still polls: %s", output.String())

	output.Reset()
	require.NoError(t, KnownVocabImportStatus(knownvocab.Status{ID: 12, State: rivertype.JobStateCancelled, Language: "de"}).Render(context.Background(), &output))
	assert.False(t, strings.Contains(output.String(), "hx-trigger"), "cancelled vocabulary import still polls: %s", output.String())
	assert.False(t, strings.Contains(output.String(), "hx-get"), "cancelled vocabulary import still polls: %s", output.String())
}

func TestKnownVocabImportTargetsVocabulary(t *testing.T) {
	assert.Equal(t, "/vocabulary/import", knownVocabImportAction())
	assert.Equal(t, "/vocabulary", knownVocabImportRecoveryTarget())
}

func TestKnownVocabTerminalStatesExplainResultsAndUseContainedTables(t *testing.T) {
	var processing bytes.Buffer
	require.NoError(t, KnownVocabImportStatus(knownvocab.Status{ID: 12, Language: "de", State: rivertype.JobStateRunning, Processed: 1, Total: 3}).Render(context.Background(), &processing))
	assert.True(t, strings.Contains(processing.String(), "Processing"), "processing status missing safe-leave contract: %s", processing.String())
	assert.True(t, strings.Contains(processing.String(), "You can leave this page"), "processing status missing safe-leave contract: %s", processing.String())
	assert.True(t, strings.Contains(processing.String(), `aria-busy="true"`), "processing status missing safe-leave contract: %s", processing.String())
	assert.True(t, strings.Contains(processing.String(), `hx-get="/vocabulary/imports/12/status"`), "processing status missing safe-leave contract: %s", processing.String())

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
			require.NoError(t, KnownVocabImportStatus(status).Render(context.Background(), &output))
			html := output.String()
			assert.True(t, strings.Contains(html, test.want), "status missing result contract: %s", html)
			assert.True(t, strings.Contains(html, "2 new"), "status missing result contract: %s", html)
			assert.True(t, strings.Contains(html, "1 duplicates"), "status missing result contract: %s", html)
			assert.Equal(t, test.table, strings.Contains(html, `class="table-region"`), "status missing result contract: %s", html)
			if test.state == rivertype.JobStateDiscarded || test.state == rivertype.JobStateCancelled {
				for _, want := range []string{
					`Selected language: <code>de</code>`,
					`href="/vocabulary"`,
					"Return to Vocabulary to retry the import",
				} {
					assert.True(t, strings.Contains(html, want), "recovery status missing %q: %s", want, html)
				}
			}
		})
	}
}

type knownVocabContextStore struct {
	StudyLanguageStore
}

func (knownVocabContextStore) ListStudyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil
}
func (knownVocabContextStore) ListKnownVocabularyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil
}

func TestKnownVocabImportLanguageUsesShellContext(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/vocabulary/import?language=it&return_to=known-vocab", nil)
	request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{ActiveLanguage: "de"}))
	assert.Equal(t, "de", knownVocabImportLanguage(request))
	request.Form = url.Values{"language": {" de "}, "return_to": {" settings "}}
	assert.Equal(t, "de", knownVocabImportLanguage(request))
}

func TestVocabularyPageUsesActiveLanguageInsteadOfURLLanguage(t *testing.T) {
	h := &Handler{services: Services{Store: StoreDependencies{StudyLanguages: knownVocabContextStore{}}}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/vocabulary?language=it", nil)
	request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{
		ActiveLanguage: "de",
		Options:        []activeStudyLanguageOption{{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true}},
	}))
	response := httptest.NewRecorder()
	h.vocabularyPage(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, strings.Contains(response.Body.String(), "Viewing <strong>German</strong> <code>de</code>"), "vocabulary page body=%s", response.Body.String())
	assert.False(t, strings.Contains(response.Body.String(), "Italian"), "vocabulary page body=%s", response.Body.String())
}

func TestKnownVocabImportParseFailuresPreserveVocabularyContext(t *testing.T) {
	h := &Handler{services: Services{Store: StoreDependencies{StudyLanguages: knownVocabContextStore{}}}}

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
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/vocabulary/import?language=it", test.body)
			request.Header.Set("Content-Type", `multipart/form-data; boundary=known-vocabulary`)
			request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{
				ActiveLanguage: "de",
				Options:        []activeStudyLanguageOption{{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true}},
			}))
			response := httptest.NewRecorder()
			h.importKnownVocab(response, request)

			assert.Equal(t, http.StatusOK, response.Code)
			for _, want := range []string{
				"The import is too large or could not be read.",
				"<strong>German</strong> <code>de</code>",
				`id="vocabulary-results"`,
			} {
				assert.True(t, strings.Contains(response.Body.String(), want), "parse failure response missing %q: %s", want, response.Body.String())
			}
		})
	}
}
