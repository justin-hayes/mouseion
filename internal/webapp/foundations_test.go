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

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayoutUsesSingleBundledFrontendFoundation(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, Layout("Foundations", nil, "").Render(context.Background(), &output))

	html := output.String()
	for _, want := range []string{
		`href="/static/app.css"`,
		`src="/static/vendor/htmx-4.0.0.min.js"`,
	} {
		assert.True(t, strings.Contains(html, want), "layout missing bundled asset %q", want)
	}
	assert.False(t, strings.Contains(html, "cdn.jsdelivr.net"), "layout must not depend on the jsDelivr CDN")

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()
	StaticHandler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	for _, want := range []string{"--mouseion-color-surface:", ".login-screen", ".library-search__controls", ".btn-primary", ".vocabulary-shell", "*::before"} {
		assert.Contains(t, response.Body.String(), want, "unified asset is missing %q", want)
	}
	picoRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/vendor/pico-2.1.1.min.css", nil)
	picoResponse := httptest.NewRecorder()
	StaticHandler().ServeHTTP(picoResponse, picoRequest)
	assert.Equal(t, http.StatusNotFound, picoResponse.Code, "retired Pico asset must not be served")

	for _, path := range []string{"/static/login.css", "/static/my-books.css", "/static/catalog-ops.css", "/static/vocabulary.css"} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		StaticHandler().ServeHTTP(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code, "retired split stylesheet %s must not be served", path)
	}
}

func TestLoginStylesAreRouteScopedAndDoNotResetUnmigratedPages(t *testing.T) {
	var login, library bytes.Buffer
	require.NoError(t, LoginPage("csrf", "", "/library", false).Render(context.Background(), &login))
	require.NoError(t, Layout("My Books", &domain.User{Username: "learner"}, "csrf").Render(context.Background(), &library))

	assert.Contains(t, login.String(), `href="/static/app.css"`)
	assert.Contains(t, login.String(), `rel="preload" href="/static/vendor/fonts/commissioner-latin-wght-normal.woff2"`)
	assert.Contains(t, login.String(), `class="login-screen`)
	assert.Contains(t, library.String(), `href="/static/app.css"`)
	assert.Contains(t, library.String(), `href="/static/app.css"`)
	assert.NotContains(t, library.String(), `pico-2.1.1.min.css`)
}

func TestMyBooksAndReadingUseOwnedStylesWithoutPico(t *testing.T) {
	var myBooks, reading bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &myBooks))
	require.NoError(t, JourneyPage(domain.User{Username: "learner"}, "csrf", journeyPageView{}, "", "").Render(context.Background(), &reading))

	assert.Contains(t, myBooks.String(), `href="/static/app.css"`)
	assert.Contains(t, myBooks.String(), `class="site-nav__link site-nav__link--current"`)
	assert.NotContains(t, myBooks.String(), `site-nav__link site-nav__link--current btn-primary`)
	assert.NotContains(t, myBooks.String(), `href="/static/vendor/pico-2.1.1.min.css"`)
	assert.Contains(t, reading.String(), `class="reading-shell"`)
	assert.NotContains(t, reading.String(), `href="/static/vendor/pico-2.1.1.min.css"`)
}

func TestOperationalAndTaskPagesLoadCompiledStylesOnTheirHostPages(t *testing.T) {
	var catalogs, jobs, reading bytes.Buffer
	require.NoError(t, ConnectionsPage(domain.User{Username: "learner"}, "csrf", nil, "", nil).Render(context.Background(), &catalogs))
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", nil, "", nil).Render(context.Background(), &jobs))
	require.NoError(t, JourneyPage(domain.User{Username: "learner"}, "csrf", journeyPageView{}, "", "").Render(context.Background(), &reading))

	for _, html := range []string{catalogs.String(), jobs.String(), reading.String()} {
		assert.Contains(t, html, `href="/static/app.css"`)
		assert.NotContains(t, html, `href="/static/catalog-ops.css"`)
	}
}

func TestReadingPagesUseCompiledFoundationAndRetainMouseionStyles(t *testing.T) {
	pages := []struct {
		name      string
		component templ.Component
	}{
		{name: "current reading", component: JourneyPage(domain.User{Username: "learner"}, "csrf", journeyPageView{}, "", "")},
		{name: "chooser", component: ReadingChooserPage(domain.User{Username: "learner"}, "csrf", readingChooserPageView{}, "", "")},
		{name: "completion receipt", component: PrimaryGoalFinishPage(domain.User{Username: "learner"}, "csrf", primaryGoalFinishView{BookTitle: "A finished Book"})},
	}
	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, page.component.Render(context.Background(), &output))
			html := output.String()
			assert.Contains(t, html, `href="/static/app.css"`)
			assert.NotContains(t, html, `href="/static/catalog-ops.css"`)
			assert.Contains(t, html, `class="reading-shell"`)
			assert.NotContains(t, html, `href="/static/vendor/pico-`)
		})
	}

	var current, chooser bytes.Buffer
	goal := testJourneyBook("book-1", "Current book", "ready")
	require.NoError(t, JourneyPage(domain.User{}, "csrf", journeyPageView{Goal: &goal}, "", "").Render(context.Background(), &current))
	candidate := readingChooserBookView{
		Book:     domain.MyBook{Book: domain.Book{ID: "candidate-1", Title: "Candidate book"}},
		Coverage: &domain.AnalysisCoverage{AnalyzableTokenCount: 1},
	}
	require.NoError(t, ReadingChooserBook(candidate, "csrf", "", "").Render(context.Background(), &chooser))
	assert.Contains(t, current.String(), `<button class="button" type="submit">Mark reading finished</button>`)
	assert.Contains(t, current.String(), `<a class="button button--quiet" href="/reading/switch">Switch current reading</a>`)
	assert.NotContains(t, current.String(), `role="button"`)
	assert.Contains(t, chooser.String(), `<button class="button" type="submit">Confirm start reading</button>`)
	assert.Contains(t, chooser.String(), `class="confirmation"`)
	assert.NotContains(t, chooser.String(), `role="button"`)

	var fragment bytes.Buffer
	require.NoError(t, PrimaryGoalFinish(primaryGoalFinishView{BookTitle: "A finished Book"}).Render(context.Background(), &fragment))
	assert.Contains(t, fragment.String(), `class="journey-goal journey-finish-outcome"`)
	assert.NotContains(t, fragment.String(), `<html`)
}

func renderReadingBrowse(t *testing.T, language string, page domain.VocabularyBrowsePage, prefix string) string {
	t.Helper()
	var output bytes.Buffer
	view := readingBrowseView{Language: language, Prefix: prefix, Page: page, Query: domain.VocabularyBrowseQuery{Language: language, CurrentBookID: page.CurrentBookID, ReadingBookID: page.CurrentBookID, Prefix: prefix, Page: page.Page}}
	require.NoError(t, ReadingBrowse(view).Render(context.Background(), &output))
	return output.String()
}

// loadBrowseForTest drives the Reading Browse loader with a Reading URL and
// renders the result the way the Reading page does.
func loadBrowseForTest(t *testing.T, services Services, target, bookID string) (readingBrowseView, int, string) {
	t.Helper()
	h := &Handler{services: services}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	view, status := h.loadReadingBrowse(request.Context(), request, "owner-1", "de", bookID)
	var output bytes.Buffer
	require.NoError(t, ReadingBrowse(view).Render(context.Background(), &output))
	return view, status, output.String()
}

func TestVocabularyBrowseUsesOwnedStylesAndHasNoRetiredWorkflow(t *testing.T) {
	goal := testJourneyBook("book-1", "Current Book", "ready")
	var reading bytes.Buffer
	view := journeyPageView{Goal: &goal, Browse: readingBrowseView{Language: "de", Query: domain.VocabularyBrowseQuery{Language: "de", CurrentBookID: "book-1"}, Page: domain.VocabularyBrowsePage{CurrentBookID: "book-1"}}}
	require.NoError(t, JourneyPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &reading))
	html := reading.String()
	assert.Contains(t, html, `href="/static/app.css"`)
	assert.NotContains(t, html, `href="/static/vocabulary.css"`)
	assert.NotContains(t, html, `href="/static/catalog-ops.css"`)
	assert.Contains(t, html, `class="reading-shell"`)
	assert.NotContains(t, html, `href="/static/vendor/pico-2.1.1.min.css"`)
	for _, retired := range []string{"Browse selection", "Custom deck", "/vocabulary/selection", "/vocabulary/decks"} {
		assert.NotContains(t, html, retired)
	}

	var concordance bytes.Buffer
	require.NoError(t, VocabularyConcordancePageView(domain.User{Username: "learner"}, "csrf", "de", domain.ConcordanceLookup{}, domain.ConcordanceResult{}, false, false, "", "", browseOrigin{}).Render(context.Background(), &concordance))
	assert.Contains(t, concordance.String(), `class="vocabulary-shell"`)
	assert.NotContains(t, concordance.String(), `href="/static/vendor/pico-2.1.1.min.css"`)

	var sentenceStudy, lemmaReview, concordanceError, concordanceChanged bytes.Buffer
	require.NoError(t, VocabularySentenceStudyPageView(domain.User{Username: "learner"}, "csrf", domain.SentenceStudy{}, "/vocabulary/concordance").Render(context.Background(), &sentenceStudy))
	require.NoError(t, LemmaReviewPage(domain.User{Username: "learner"}, "csrf", "book", "Book", "", "", true, nil, nil, lemmaReviewRecovery{}, nil).Render(context.Background(), &lemmaReview))
	require.NoError(t, VocabularyConcordanceErrorPageView(domain.User{Username: "learner"}, "csrf", "de", domain.ConcordanceLookup{}, false, browseOrigin{}).Render(context.Background(), &concordanceError))
	require.NoError(t, VocabularyConcordanceChangedPageView(domain.User{Username: "learner"}, "csrf", "de", domain.ConcordanceLookup{}, browseOrigin{}).Render(context.Background(), &concordanceChanged))
	for _, html := range []string{sentenceStudy.String(), lemmaReview.String(), concordanceError.String(), concordanceChanged.String()} {
		assert.Contains(t, html, `href="/static/app.css"`)
		assert.NotContains(t, html, `href="/static/vocabulary.css"`)
		assert.NotContains(t, html, `href="/static/vendor/pico-2.1.1.min.css"`)
	}
	assert.Contains(t, sentenceStudy.String(), `class="vocabulary-shell"`)
	assert.Contains(t, lemmaReview.String(), `class="reading-shell"`)
	assert.NotContains(t, lemmaReview.String(), `href="/static/catalog-ops.css"`)
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

func TestTypographyAssetsAreEmbeddedAndServedLocally(t *testing.T) {
	for _, path := range []string{
		"/static/vendor/fonts/commissioner-latin-wght-normal.woff2",
		"/static/vendor/fonts/commissioner-latin-ext-wght-normal.woff2",
		"/static/vendor/fonts/commissioner-greek-wght-normal.woff2",
		"/static/vendor/fonts/literata-latin-opsz-normal.woff2",
		"/static/vendor/fonts/literata-latin-ext-opsz-normal.woff2",
		"/static/vendor/fonts/literata-greek-opsz-normal.woff2",
	} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			StaticHandler().ServeHTTP(response, request)
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, "font/woff2", response.Header().Get("Content-Type"))
			assert.NotEmpty(t, response.Body.Bytes())
		})
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()
	StaticHandler().ServeHTTP(response, request)
	css := response.Body.String()
	assert.Contains(t, css, `font-family: "Commissioner Variable"`)
	assert.Contains(t, css, `font-family: "Literata Variable"`)
	assert.Contains(t, css, `url("/static/vendor/fonts/commissioner-greek-wght-normal.woff2")`)
	assert.Contains(t, css, `url("/static/vendor/fonts/literata-greek-opsz-normal.woff2")`)
	assert.NotRegexp(t, `url\(https?://`, css, "font CSS must not make third-party requests")
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
	for _, pattern := range []string{`class="library-masthead"`, `class="library-books"`, `class="library-book__identity-link"`} {
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
	status := knownvocab.Status{ID: 7, Processed: 2, Total: 5, State: rivertype.JobStateRunning, Language: "de"}
	require.NoError(t, KnownVocabImportStatus(status).Render(context.Background(), &progress))
	for _, want := range []string{
		`data-workflow="known-vocabulary import"`,
		`hx-get="/vocabulary/imports/7/status"`,
		`hx-trigger="every 2s"`,
		`2 of 5 rows processed.`,
	} {
		assert.True(t, strings.Contains(progress.String(), want), "import status missing %q: %s", want, progress.String())
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

func TestVocabularyHeaderKeepsPeerViewsUnderOneDestinationHeading(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, VocabularyPageHeader("import").Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `<header class="vocabulary-page-header"><h1>Vocabulary</h1><nav aria-label="Vocabulary views">`)
	assert.Contains(t, html, `<a href="/vocabulary/import" aria-current="page">Import Known words</a>`)
	assert.NotContains(t, html, `Vocabulary ·`)
	assert.Less(t, strings.Index(html, `<h1>Vocabulary</h1>`), strings.Index(html, `aria-label="Vocabulary views"`))
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

func TestVocabularyImportUsesOwnedShellAndKeepsNativeForm(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, VocabularyPageWithResult(domain.User{}, "csrf", []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil, "de", nil, "").Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `<body class="vocabulary-shell">`)
	assert.NotContains(t, html, `href="/static/vocabulary.css"`)
	assert.NotContains(t, html, `href="/static/catalog-ops.css"`)
	assert.NotContains(t, html, "pico-2.1.1.min.css")
	assert.Contains(t, html, `<form class="vocabulary-import-form" method="post" action="/vocabulary/import" enctype="multipart/form-data"`)
	assert.Contains(t, html, `<label for="known-vocabulary-file">UTF-8 lemma file</label><input id="known-vocabulary-file" type="file" name="vocabulary_file"`)
	assert.Contains(t, html, `<button class="btn btn-primary" type="submit">Import known vocabulary</button>`)
	assert.Contains(t, html, `id="vocabulary-import-pending" class="htmx-indicator" role="status"`)
	assert.Contains(t, html, `hx-post="/vocabulary/import"`)

	output.Reset()
	status := knownvocab.Status{ID: 8, Language: "de", State: rivertype.JobStateCompleted, Rejected: []knownvocab.Rejection{{Row: 2, Original: "bad\tline", Error: "expected exactly one lemma"}}}
	require.NoError(t, KnownVocabImportStatusPage(domain.User{}, "csrf", status).Render(context.Background(), &output))
	html = output.String()
	assert.Contains(t, html, `<body class="vocabulary-shell">`)
	assert.NotContains(t, html, `href="/static/vocabulary.css"`)
	assert.NotContains(t, html, `href="/static/catalog-ops.css"`)
	assert.NotContains(t, html, "pico-2.1.1.min.css")
	assert.Contains(t, html, `aria-label="Rejected vocabulary rows"`)
	assert.Contains(t, html, `href="/vocabulary/import"`)
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
	require.NoError(t, KnownVocabImportStatus(knownvocab.Status{ID: 12, State: rivertype.JobStateCompleted, Language: "de", Imported: 5}).Render(context.Background(), &output))
	assert.False(t, strings.Contains(output.String(), "hx-trigger"), "completed vocabulary import still polls: %s", output.String())
	assert.False(t, strings.Contains(output.String(), "hx-get"), "completed vocabulary import still polls: %s", output.String())

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

func TestVocabularyBrowsePagerPreservesAppliedPrefixWithoutJavaScript(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, VocabularyBrowsePager(domain.VocabularyBrowsePage{Page: 2, Total: 51, CurrentBookID: "book-1", ReadingBookID: "book-1"}, "haus").Render(context.Background(), &output))
	assert.Contains(t, output.String(), `href="/reading?page=1&amp;q=haus&amp;reading=book-1"`)
	assert.Contains(t, output.String(), `href="/reading?page=3&amp;q=haus&amp;reading=book-1"`)
	assert.Contains(t, output.String(), `hx-get="/reading?page=3&amp;q=haus&amp;reading=book-1"`)
	assert.Contains(t, output.String(), `hx-push-url="true"`)
}

func TestVocabularyBrowseHasNoSelectionControls(t *testing.T) {
	page := domain.VocabularyBrowsePage{Page: 2, Total: 51, CurrentBookID: "book-1", ReadingBookID: "book-1", CorpusRevision: "rev-1", IncludeAll: true,
		Books: []domain.VocabularyBrowseBook{{ID: "book-1", Title: "Current Book", HasCurrentAnalysis: true}},
		Rows:  []domain.VocabularyBrowseRow{{CanonicalLemma: "haus", UPOS: "NOUN"}}}
	html := renderReadingBrowse(t, "de", page, "ha")
	assert.Contains(t, html, `id="vocabulary-workflow"`)
	assert.Contains(t, html, `id="vocabulary-browse-results"`)
	assert.Contains(t, html, `href="/vocabulary/concordance?`)
	assert.Contains(t, html, `hx-select="#vocabulary-browse-results"`)
	for _, retired := range []string{"Select</button>", "Remove</button>", "selected identities", "Browse selection", "Custom deck", "/vocabulary/selection", "/vocabulary/decks"} {
		assert.NotContains(t, html, retired)
	}
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
	html := renderReadingBrowse(t, "de", page, "")
	for _, want := range []string{
		"Occurrence counts", "In this Book: 2; Across analyzed books: 5", "In this Book: 3; Across analyzed books: 3",
		">Haus</a>", ">gehen</a>",
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, `name="lemma"`, "Browse is read-only and no longer posts selection changes")

	assert.Contains(t, renderReadingBrowse(t, "it", page, ""), ">haus</a>", "non-German noun display retains canonical casing")
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

func browseTestServices(store VocabularyBrowseStore) Services {
	return Services{Store: StoreDependencies{VocabularyBrowse: store}}
}

func TestVocabularyBrowseFailureOffersRetryWithAppliedControls(t *testing.T) {
	_, status, html := loadBrowseForTest(t, browseTestServices(failedVocabularyBrowseStore{}), "/reading?q=Haus&all=1&book=book-1&pos=NOUN&known=not-known-or-reserved&reserved=not-reserved&sort=books&page=2", "book-1")

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Contains(t, html, `id="vocabulary-recovery"`)
	for _, want := range []string{
		"Browse could not be loaded", "Your search was not applied", "Retry Browse",
		`name="q" value="Haus"`, `name="reading" value="book-1"`, `name="page" value="2"`,
		`name="all" value="1"`,
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, `name="book"`)
}

type timedOutVocabularyBrowseStore struct{ knownVocabContextStore }

func (timedOutVocabularyBrowseStore) ListVocabularyBrowsePage(ctx context.Context, _, _ string, _ domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	<-ctx.Done()
	return domain.VocabularyBrowsePage{}, ctx.Err()
}

func TestVocabularyBrowseTimeoutReturnsRecoverableGatewayTimeout(t *testing.T) {
	services := browseTestServices(timedOutVocabularyBrowseStore{})
	services.InteractiveReadTimeout = 100 * time.Millisecond
	started := time.Now()
	_, status, html := loadBrowseForTest(t, services, "/reading?q=Haus&book=book-1&page=3", "book-1")

	assert.Equal(t, http.StatusGatewayTimeout, status)
	assert.Less(t, time.Since(started), 5*time.Second)
	for _, want := range []string{"Retry Browse", "shorter one", `name="q" value="Haus"`, `name="reading" value="book-1"`, `name="page" value="3"`} {
		assert.Contains(t, html, want)
	}
}

type changedVocabularyBrowseStore struct{ knownVocabContextStore }

func (changedVocabularyBrowseStore) ListVocabularyBrowsePage(_ context.Context, _, _ string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{Page: query.Page, CorpusRevision: "revision-new"}, nil
}

func TestVocabularyBrowseChangedEvidenceOffersRestartInsteadOfStalePage(t *testing.T) {
	_, status, html := loadBrowseForTest(t, browseTestServices(changedVocabularyBrowseStore{}), "/reading?q=Haus&reading=book-1&book=obsolete-book&pos=NOUN&known=not-known&reserved=not-reserved&sort=books&page=2&rev=revision-old", "book-1")

	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, html, `id="vocabulary-recovery"`)
	assert.Contains(t, html, "Current evidence changed")
	assert.Contains(t, html, "Refresh vocabulary")
	assert.Contains(t, html, "reading=book-1")
	assert.NotContains(t, html, "obsolete-book")
	assert.Contains(t, html, `q=Haus`)
	assert.NotContains(t, html, "reserved=")
	assert.Contains(t, html, `page=1`)
	assert.NotContains(t, html, "revision-new")
}

func TestVocabularyBrowseRefusesToApplyARequestAfterCurrentBookChanges(t *testing.T) {
	_, status, html := loadBrowseForTest(t, browseTestServices(changedVocabularyBrowseStore{}), "/reading?q=Haus&reading=book-1&page=2&rev=old-revision", "book-2")

	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, html, "were not applied")
	assert.Contains(t, html, "q=Haus")
	assert.Contains(t, html, "reading=book-2")
	assert.NotContains(t, html, "old-revision")
}

type totalVocabularyBrowseStore struct {
	knownVocabContextStore
	total int64
}

func (s totalVocabularyBrowseStore) ListVocabularyBrowsePage(_ context.Context, _, _ string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{Page: query.Page, Total: s.total, Books: []domain.VocabularyBrowseBook{{ID: query.CurrentBookID, Title: "Current", HasCurrentAnalysis: true}}}, nil
}

func TestVocabularyBrowseExplainsMalformedAndOutOfRangePagesWithFirstPage(t *testing.T) {
	for _, page := range []string{"abc", "0", "-3", "1.5"} {
		_, status, html := loadBrowseForTest(t, browseTestServices(knownVocabContextStore{}), "/reading?q=ha&page="+page, "book-1")
		assert.Equal(t, http.StatusBadRequest, status, page)
		assert.Contains(t, html, "That page is not available", page)
		assert.Contains(t, html, `>First page</a>`, page)
		assert.Contains(t, html, `href="/reading?language=de&amp;page=1&amp;q=ha&amp;reading=book-1"`, page)
		assert.NotContains(t, html, "Browse results", page)
	}
	view, status, html := loadBrowseForTest(t, browseTestServices(totalVocabularyBrowseStore{total: 51}), "/reading?page=4", "book-1")
	assert.Equal(t, readingBrowsePageOutOfRange, view.Problem)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, html, "Page 4 is past the end of these results, which have 3.")
	assert.Contains(t, html, `>First page</a>`)
	assert.NotContains(t, html, "Browse results")
	_, status, _ = loadBrowseForTest(t, browseTestServices(totalVocabularyBrowseStore{total: 51}), "/reading?page=3", "book-1")
	assert.Equal(t, http.StatusOK, status, "the last page is in range")
	_, status, _ = loadBrowseForTest(t, browseTestServices(totalVocabularyBrowseStore{}), "/reading", "book-1")
	assert.Equal(t, http.StatusOK, status, "an empty first page is not out of range")
	_, status, _ = loadBrowseForTest(t, browseTestServices(totalVocabularyBrowseStore{}), "/reading?page=2", "book-1")
	assert.Equal(t, http.StatusNotFound, status)
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
