package webapp

import (
	"bytes"
	"context"
	"errors"
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
		`src="/static/vendor/htmx-4.0.0.min.js"`,
	} {
		assert.True(t, strings.Contains(html, want), "layout missing bundled asset %q", want)
	}
	assert.False(t, strings.Contains(html, "cdn.jsdelivr.net"), "layout must not depend on the jsDelivr CDN")

	for _, asset := range []struct {
		path string
		want string
	}{
		{path: "/static/vendor/pico-2.1.1.min.css", want: "Pico CSS"},
		{path: "/static/vendor/htmx-4.0.0.min.js", want: "htmx"},
		{path: "/static/login.css", want: ".login-screen"},
	} {
		t.Run(asset.path, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, asset.path, nil)
			response := httptest.NewRecorder()
			StaticHandler().ServeHTTP(response, request)

			assert.Equal(t, http.StatusOK, response.Code)
			assert.True(t, strings.Contains(response.Body.String(), asset.want), "asset body missing %q", asset.want)
		})
	}

	cssRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/login.css", nil)
	cssResponse := httptest.NewRecorder()
	StaticHandler().ServeHTTP(cssResponse, cssRequest)
	css := cssResponse.Body.String()
	assert.NotContains(t, css, "*,:before,:after,::backdrop{box-sizing:border-box", "Tailwind Preflight must not reset Pico-owned pages")
	assert.NotContains(t, css, "*,::after,::before,::backdrop{box-sizing:border-box", "Tailwind Preflight must not reset Pico-owned pages")
}

func TestLoginStylesAreRouteScopedAndDoNotResetUnmigratedPages(t *testing.T) {
	var login, library bytes.Buffer
	require.NoError(t, LoginPage("csrf", "", "/library", false).Render(context.Background(), &login))
	require.NoError(t, Layout("My Books", &domain.User{Username: "learner"}, "csrf").Render(context.Background(), &library))

	assert.Contains(t, login.String(), `href="/static/login.css"`)
	assert.Contains(t, login.String(), `class="login-screen`)
	assert.NotContains(t, library.String(), `href="/static/login.css"`)
	assert.Contains(t, library.String(), `href="/static/vendor/pico-2.1.1.min.css"`)
}

func TestMyBooksAndReadingUseOwnedStylesWithoutPico(t *testing.T) {
	var myBooks, reading bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &myBooks))
	require.NoError(t, JourneyPage(domain.User{Username: "learner"}, "csrf", journeyPageView{}, "", "").Render(context.Background(), &reading))

	assert.Contains(t, myBooks.String(), `href="/static/app.css"`)
	assert.NotContains(t, myBooks.String(), `href="/static/vendor/pico-2.1.1.min.css"`)
	assert.Contains(t, reading.String(), `class="reading-shell"`)
	assert.NotContains(t, reading.String(), `href="/static/vendor/pico-2.1.1.min.css"`)
}

func TestLoginOnboardingAndRecoveryKeepNativeFormsAndAccessibleFeedback(t *testing.T) {
	for _, test := range []struct {
		name       string
		message    string
		onboarding bool
		want       []string
	}{
		{
			name:    "invalid credentials",
			message: "Invalid credentials",
			want:    []string{`role="alert"`, `class="alert alert-error login-screen__error"`, `action="/login"`, `autocomplete="current-password"`, `name="next" value="/library"`},
		},
		{
			name:       "first account",
			onboarding: true,
			want:       []string{`action="/onboarding"`, `autocomplete="new-password"`, `minlength="8"`, "Use at least 8 characters."},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, LoginPage("csrf", test.message, "/library", test.onboarding).Render(context.Background(), &output))
			for _, want := range test.want {
				assert.Contains(t, output.String(), want)
			}
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
	for _, pattern := range []string{`class="skip-link"`, `href="#main-content"`, `aria-label="Primary navigation"`, `id="main-content"`, `tabindex="-1"`, `aria-label="Deck preparation progress"`} {
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
	assert.Equal(t, "/vocabulary/import", knownVocabImportRecoveryTarget())
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
					`href="/vocabulary/import"`,
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
	CurrentReadingStore
}

func (knownVocabContextStore) ListStudyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil
}
func (knownVocabContextStore) ListKnownVocabularyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil
}
func (knownVocabContextStore) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return domain.CurrentReading{BookID: "book-1"}, nil
}
func (knownVocabContextStore) ListVocabularyBrowsePage(_ context.Context, _, _ string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{Page: query.Page}, nil
}

func TestKnownVocabImportLanguageUsesShellContext(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/vocabulary/import?language=it&return_to=known-vocab", nil)
	request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{ActiveLanguage: "de"}))
	assert.Equal(t, "de", knownVocabImportLanguage(request))
	request.Form = url.Values{"language": {" de "}, "return_to": {" settings "}}
	assert.Equal(t, "de", knownVocabImportLanguage(request))
}

func TestVocabularyPageUsesActiveLanguageInsteadOfURLLanguage(t *testing.T) {
	store := knownVocabContextStore{}
	h := &Handler{services: Services{Store: StoreDependencies{StudyLanguages: store, CurrentReading: store, VocabularyBrowse: store}}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/vocabulary?language=it", nil)
	request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{
		ActiveLanguage: "de",
		Options:        []activeStudyLanguageOption{{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true}},
	}))
	response := httptest.NewRecorder()
	h.vocabularyPage(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, strings.Contains(response.Body.String(), "active study language <code>de</code>"), "vocabulary page body=%s", response.Body.String())
	assert.False(t, strings.Contains(response.Body.String(), "Italian"), "vocabulary page body=%s", response.Body.String())
}

func TestVocabularyBrowsePagerPreservesAppliedPrefixWithoutJavaScript(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, VocabularyBrowsePager(domain.VocabularyBrowsePage{Page: 2, Total: 51, CurrentBookID: "book-1", ReadingBookID: "book-1"}, "haus").Render(context.Background(), &output))
	assert.Contains(t, output.String(), `href="/vocabulary?page=1&amp;q=haus&amp;reading=book-1"`)
	assert.Contains(t, output.String(), `href="/vocabulary?page=3&amp;q=haus&amp;reading=book-1"`)
	assert.Contains(t, output.String(), `hx-get="/vocabulary?page=3&amp;q=haus&amp;reading=book-1"`)
	assert.Contains(t, output.String(), `hx-push-url="true"`)
}

func TestVocabularyBrowseSelectionEnhancesNativeFormAndPreservesPage(t *testing.T) {
	page := domain.VocabularyBrowsePage{Page: 2, Total: 51, CurrentBookID: "book-1", ReadingBookID: "book-1", CorpusRevision: "rev-1", IncludeAll: true, SelectionCount: 3,
		Books: []domain.VocabularyBrowseBook{{ID: "book-1", Title: "Current Book", HasCurrentAnalysis: true}},
		Rows:  []domain.VocabularyBrowseRow{{CanonicalLemma: "haus", UPOS: "NOUN"}}}
	var output bytes.Buffer
	require.NoError(t, VocabularyBrowsePageView(domain.User{}, "csrf", "de", page, "ha").Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `id="vocabulary-workflow"`)
	assert.Contains(t, html, `id="vocabulary-browse-results"`)
	assert.Contains(t, html, `action="/vocabulary/selection/add?all=1&amp;page=2&amp;q=ha&amp;reading=book-1&amp;rev=rev-1"`)
	assert.Contains(t, html, `hx-post="/vocabulary/selection/add?all=1&amp;page=2&amp;q=ha&amp;reading=book-1&amp;rev=rev-1"`)
	assert.Contains(t, html, `hx-select="#vocabulary-browse-results"`)
	assert.Contains(t, html, `hx-status:409="target:#vocabulary-recovery`)
	assert.Contains(t, html, `Browse selection (3)`)
}

func TestVocabularyBrowseRendersScopedAndAcrossBookCountsAndDisplayLemma(t *testing.T) {
	page := domain.VocabularyBrowsePage{
		CurrentBookID: "book-1", Total: 2, InventoryTotal: 2, ScopedInventoryTotal: 2,
		Books: []domain.VocabularyBrowseBook{{ID: "book-1", Title: "Current Book", HasCurrentAnalysis: true, HasVocabularyEvidence: true}},
		Rows: []domain.VocabularyBrowseRow{
			{CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 2, AcrossBooksOccurrenceCount: 5},
			{CanonicalLemma: "garten", UPOS: "NOUN", OccurrenceCount: 3, AcrossBooksOccurrenceCount: 3},
			{CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 1, AcrossBooksOccurrenceCount: 4},
		},
	}
	var output bytes.Buffer
	require.NoError(t, VocabularyBrowsePageView(domain.User{}, "csrf", "de", page, "").Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		"Occurrence counts", "In this Book: 2; Across analyzed books: 5", "In this Book: 3; Across analyzed books: 3",
		">Haus</a>", ">gehen</a>", `name="lemma" value="haus"`,
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, `name="lemma" value="Haus"`, "display casing must not alter selection identity")

	output.Reset()
	require.NoError(t, VocabularyBrowsePageView(domain.User{}, "csrf", "it", page, "").Render(context.Background(), &output))
	assert.Contains(t, output.String(), ">haus</a>", "non-German noun display retains canonical casing")
}

func TestVocabularyBrowsePagerPreservesPrefixReadingScopeAndRevision(t *testing.T) {
	page := domain.VocabularyBrowsePage{Page: 2, Total: 51, CurrentBookID: "current-book", ReadingBookID: "current-book", SelectedBooks: []string{"book-1", "book-2"}, SelectedUPOS: []string{"NOUN"}, KnownFilter: "not-known", ReservedFilter: "not-reserved", Sort: "occurrences", IncludeAll: true, CorpusRevision: "revision-1"}
	url := vocabularyBrowsePageURL(3, page, "Haus")
	for _, value := range []string{"q=Haus", "reading=current-book", "page=3", "all=1", "rev=revision-1"} {
		assert.Contains(t, url, value)
	}
	for _, obsolete := range []string{"book=", "pos=", "known=", "reserved=", "sort="} {
		assert.NotContains(t, url, obsolete)
	}
}

func TestVocabularyBrowseRestartDropsOldRevisionAndObsoleteControls(t *testing.T) {
	page := domain.VocabularyBrowsePage{CurrentBookID: "book-1", ReadingBookID: "book-1", SelectedBooks: []string{"book-1"}, SelectedUPOS: []string{"NOUN"}, KnownFilter: "not-known", ReservedFilter: "not-reserved", Sort: "books"}
	url := vocabularyBrowsePageURL(1, page, "Haus")
	for _, value := range []string{"q=Haus", "reading=book-1", "page=1"} {
		assert.Contains(t, url, value)
	}
	for _, obsolete := range []string{"book=", "pos=", "known=", "reserved=", "sort="} {
		assert.NotContains(t, url, obsolete)
	}
	assert.NotContains(t, url, "rev=")
}

type failedVocabularyBrowseStore struct{ knownVocabContextStore }

func (failedVocabularyBrowseStore) ListVocabularyBrowsePage(context.Context, string, string, domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{}, errors.New("database unavailable")
}

func TestVocabularyBrowseFailureOffersRetryWithAppliedControls(t *testing.T) {
	store := failedVocabularyBrowseStore{}
	h := &Handler{services: Services{Store: StoreDependencies{StudyLanguages: store, CurrentReading: store, VocabularyBrowse: store}}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/vocabulary?q=Haus&all=1&book=book-1&pos=NOUN&known=not-known-or-reserved&reserved=not-reserved&sort=books&page=2", nil)
	request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{
		ActiveLanguage: "de",
		Options:        []activeStudyLanguageOption{{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true}},
	}))
	response := httptest.NewRecorder()
	h.vocabularyPage(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Contains(t, response.Body.String(), `id="vocabulary-recovery"`)
	for _, want := range []string{
		"Browse could not be loaded", "Your search was not applied", "Retry Browse",
		`name="q" value="Haus"`, `name="reading" value="book-1"`, `name="page" value="2"`,
		`name="all" value="1"`,
	} {
		assert.Contains(t, response.Body.String(), want)
	}
	assert.NotContains(t, response.Body.String(), `name="book"`)
}

type timedOutVocabularyBrowseStore struct{ knownVocabContextStore }

func (timedOutVocabularyBrowseStore) ListVocabularyBrowsePage(ctx context.Context, _, _ string, _ domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	<-ctx.Done()
	return domain.VocabularyBrowsePage{}, ctx.Err()
}

func TestVocabularyBrowseTimeoutReturnsRecoverableGatewayTimeout(t *testing.T) {
	store := timedOutVocabularyBrowseStore{}
	h := &Handler{services: Services{Store: StoreDependencies{StudyLanguages: store, CurrentReading: store, VocabularyBrowse: store}}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/vocabulary?q=Haus&book=book-1&page=3", nil)
	request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{
		ActiveLanguage: "de",
		Options:        []activeStudyLanguageOption{{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true}},
	}))
	response := httptest.NewRecorder()
	started := time.Now()
	h.vocabularyPage(response, request)

	assert.Equal(t, http.StatusGatewayTimeout, response.Code)
	assert.Less(t, time.Since(started), 10*time.Second)
	for _, want := range []string{"Retry Browse", "shorter one", "name=\"q\" value=\"Haus\"", "name=\"reading\" value=\"book-1\"", "name=\"page\" value=\"3\""} {
		assert.Contains(t, response.Body.String(), want)
	}
}

type changedVocabularyBrowseStore struct{ knownVocabContextStore }

func (changedVocabularyBrowseStore) ListVocabularyBrowsePage(_ context.Context, _, _ string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{Page: query.Page, CorpusRevision: "revision-new"}, nil
}

func TestVocabularyBrowseChangedEvidenceOffersRestartInsteadOfStalePage(t *testing.T) {
	store := changedVocabularyBrowseStore{}
	h := &Handler{services: Services{Store: StoreDependencies{StudyLanguages: store, CurrentReading: store, VocabularyBrowse: store}}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/vocabulary?q=Haus&reading=book-1&book=obsolete-book&pos=NOUN&known=not-known&reserved=not-reserved&sort=books&page=2&rev=revision-old", nil)
	request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{
		ActiveLanguage: "de",
		Options:        []activeStudyLanguageOption{{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true}},
	}))
	response := httptest.NewRecorder()
	h.vocabularyPage(response, request)

	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), `id="vocabulary-recovery"`)
	assert.Contains(t, response.Body.String(), "Current evidence changed")
	assert.Contains(t, response.Body.String(), "Restart in the Current reading")
	assert.Contains(t, response.Body.String(), "reading=book-1")
	assert.NotContains(t, response.Body.String(), "obsolete-book")
	assert.Contains(t, response.Body.String(), `q=Haus`)
	assert.NotContains(t, response.Body.String(), "reserved=")
	assert.Contains(t, response.Body.String(), `page=1`)
	assert.NotContains(t, response.Body.String(), "revision-new")
}

type switchedCurrentReadingStore struct{ changedVocabularyBrowseStore }

func (switchedCurrentReadingStore) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return domain.CurrentReading{BookID: "book-2"}, nil
}

func TestVocabularyBrowseRefusesToApplyARequestAfterCurrentBookChanges(t *testing.T) {
	store := switchedCurrentReadingStore{}
	h := &Handler{services: Services{Store: StoreDependencies{StudyLanguages: store, CurrentReading: store, VocabularyBrowse: store}}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/vocabulary?q=Haus&reading=book-1&page=2&rev=old-revision", nil)
	request = request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, &shellView{
		ActiveLanguage: "de",
		Options:        []activeStudyLanguageOption{{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true}},
	}))
	response := httptest.NewRecorder()
	h.vocabularyPage(response, request)

	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "These results were not applied")
	assert.Contains(t, response.Body.String(), "q=Haus")
	assert.Contains(t, response.Body.String(), "reading=book-2")
	assert.NotContains(t, response.Body.String(), "old-revision")
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
