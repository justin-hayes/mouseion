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
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellLayoutUsesSingleBundledFrontendFoundation(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, ShellLayout("Foundations", nil, "", NavigationReading).Render(context.Background(), &output))

	html := output.String()
	for _, want := range []string{
		`href="/static/app.css"`,
		`src="/static/vendor/htmx-4.0.0.min.js"`,
	} {
		assert.Contains(t, html, want, "layout missing bundled asset %q", want)
	}
	assert.NotContains(t, html, "cdn.jsdelivr.net", "layout must not depend on the jsDelivr CDN")

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
	require.NoError(t, LoginPage("csrf", "", "/de/reading", false).Render(context.Background(), &login))
	require.NoError(t, ShellLayout("Reading", &domain.User{Username: "learner"}, "csrf", NavigationReading).Render(context.Background(), &library))

	assert.Contains(t, login.String(), `href="/static/app.css"`)
	assert.Contains(t, login.String(), `rel="preload" href="/static/vendor/fonts/commissioner-latin-wght-normal.woff2"`)
	assert.Contains(t, login.String(), `class="login-screen`)
	assert.Contains(t, library.String(), `href="/static/app.css"`)
	assert.Contains(t, library.String(), `href="/static/app.css"`)
	assert.NotContains(t, library.String(), `pico-2.1.1.min.css`)
}

func TestMyBooksAndReadingUseOwnedStylesWithoutPico(t *testing.T) {
	var myBooks, reading bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", MyBooksBrowseState{}).Render(context.Background(), &myBooks))
	require.NoError(t, readingPageForTest(domain.User{Username: "learner"}, "csrf", readingPageView{}).Render(context.Background(), &reading))

	assert.Contains(t, myBooks.String(), `href="/static/app.css"`)
	assert.Contains(t, myBooks.String(), `class="site-nav__link site-nav__link--current"`)
	assert.NotContains(t, myBooks.String(), `site-nav__link site-nav__link--current btn-primary`)
	assert.NotContains(t, myBooks.String(), `href="/static/vendor/pico-2.1.1.min.css"`)
	assert.Contains(t, reading.String(), `class="my-books-shell"`)
	assert.NotContains(t, reading.String(), `href="/static/vendor/pico-2.1.1.min.css"`)
}

func TestOperationalAndTaskPagesLoadCompiledStylesOnTheirHostPages(t *testing.T) {
	var catalogs, jobs, reading bytes.Buffer
	require.NoError(t, CatalogPage(domain.User{Username: "learner"}, "csrf", nil, "", "", nil, catalogBrowseView{}).Render(context.Background(), &catalogs))
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", nil, nil, "", "").Render(context.Background(), &jobs))
	require.NoError(t, readingPageForTest(domain.User{Username: "learner"}, "csrf", readingPageView{}).Render(context.Background(), &reading))

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
		{name: "current reading", component: readingPageForTest(domain.User{Username: "learner"}, "csrf", readingPageView{})},
		{name: "completion receipt", component: CurrentReadingFinishPage(domain.User{Username: "learner"}, "csrf", currentReadingFinishView{BookTitle: "A finished Book"})},
	}
	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, page.component.Render(context.Background(), &output))
			html := output.String()
			assert.Contains(t, html, `href="/static/app.css"`)
			assert.NotContains(t, html, `href="/static/catalog-ops.css"`)
			assert.Regexp(t, `class="(my-books|reading)-shell"`, html)
			assert.NotContains(t, html, `href="/static/vendor/pico-`)
		})
	}

	var current bytes.Buffer
	currentReading := testReadingBook("book-1", "Current book", testNoContent)
	require.NoError(t, readingPageForTest(domain.User{}, "csrf", readingPageView{CurrentReading: &currentReading}).Render(context.Background(), &current))
	assert.Contains(t, current.String(), `<button class="button" type="submit">Mark reading finished</button>`)
	assert.NotContains(t, current.String(), "/reading/switch", "Read next replaces Switch")

	var fragment bytes.Buffer
	require.NoError(t, CurrentReadingFinish(currentReadingFinishView{BookTitle: "A finished Book"}).Render(context.Background(), &fragment))
	assert.Contains(t, fragment.String(), `class="journey-goal journey-finish-outcome"`)
	assert.NotContains(t, fragment.String(), `<html`)
}

func renderBookWords(t *testing.T, language string, page domain.VocabularyBrowsePage, prefix string) string {
	t.Helper()
	var output bytes.Buffer
	view := bookWordsView{BookID: page.CurrentBookID, Language: language, Prefix: prefix, Page: page, Query: domain.VocabularyBrowseQuery{Language: language, CurrentBookID: page.CurrentBookID, ReadingBookID: page.CurrentBookID, Prefix: prefix, Page: page.Page}}
	for _, row := range page.Rows {
		view.Rows = append(view.Rows, bookWordRow{VocabularyBrowseRow: row, State: bookWordNotIntroduced})
	}
	require.NoError(t, BookWords(view).Render(context.Background(), &output))
	return output.String()
}

// loadBrowseForTest drives the Book word-list loader with a Book detail URL and
// renders the result the way the Book detail page does.
func loadBrowseForTest(t *testing.T, services Services, target, bookID string) (bookWordsView, int, string) {
	t.Helper()
	h := &Handler{services: services}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	view, status := h.loadBookWords(request.Context(), request, "owner-1", "de", bookID)
	var output bytes.Buffer
	require.NoError(t, BookWords(view).Render(context.Background(), &output))
	return view, status, output.String()
}

func TestVocabularyBrowseUsesOwnedStylesAndHasNoRetiredWorkflow(t *testing.T) {
	var reading bytes.Buffer
	words := bookWordsView{BookID: "book-1", Language: "de", Query: domain.VocabularyBrowseQuery{Language: "de", CurrentBookID: "book-1"}, Page: domain.VocabularyBrowsePage{CurrentBookID: "book-1"}}
	detail := bookDetailView{Book: domain.MyBook{Book: domain.Book{ID: "book-1", Title: "Current Book", LanguageState: domain.LanguageChosen, LanguageTag: "de"}}, Language: "de", Words: &words}
	require.NoError(t, BookDetailPage(domain.User{Username: "learner"}, "csrf", detail, "", "").Render(context.Background(), &reading))
	html := reading.String()
	assert.Contains(t, html, `href="/static/app.css"`)
	assert.NotContains(t, html, `href="/static/vocabulary.css"`)
	assert.NotContains(t, html, `href="/static/catalog-ops.css"`)
	assert.Contains(t, html, `class="reading-shell"`)
	assert.NotContains(t, html, `href="/static/vendor/pico-2.1.1.min.css"`)
	for _, retired := range []string{"Browse selection", "Custom deck", "/corpus/selection", "/corpus/decks"} {
		assert.NotContains(t, html, retired)
	}

	var concordance bytes.Buffer
	require.NoError(t, VocabularyConcordancePageView(domain.User{Username: "learner"}, "csrf", "de", domain.ConcordanceLookup{}, domain.ConcordanceResult{}, false, false, "", "", browseOrigin{}).Render(context.Background(), &concordance))
	assert.Contains(t, concordance.String(), `class="vocabulary-shell"`)
	assert.NotContains(t, concordance.String(), `href="/static/vendor/pico-2.1.1.min.css"`)

	var sentenceStudy, concordanceError, concordanceChanged bytes.Buffer
	require.NoError(t, VocabularySentenceStudyPageView(domain.User{Username: "learner"}, "csrf", domain.SentenceStudy{}, "/corpus/concordance", "").Render(context.Background(), &sentenceStudy))
	require.NoError(t, VocabularyConcordanceErrorPageView(domain.User{Username: "learner"}, "csrf", "de", domain.ConcordanceLookup{}, false, browseOrigin{}).Render(context.Background(), &concordanceError))
	require.NoError(t, VocabularyConcordanceChangedPageView(domain.User{Username: "learner"}, "csrf", "de", domain.ConcordanceLookup{}, browseOrigin{}).Render(context.Background(), &concordanceChanged))
	for _, html := range []string{sentenceStudy.String(), concordanceError.String(), concordanceChanged.String()} {
		assert.Contains(t, html, `href="/static/app.css"`)
		assert.NotContains(t, html, `href="/static/vocabulary.css"`)
		assert.NotContains(t, html, `href="/static/vendor/pico-2.1.1.min.css"`)
	}
	assert.Contains(t, sentenceStudy.String(), `class="vocabulary-shell"`)
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
			want:    []string{`role="alert"`, `class="alert alert-error login-screen__error"`, `action="/login"`, `autocomplete="current-password"`, `name="next" value="/de/reading"`},
		},
		{
			name:       "first account",
			onboarding: true,
			want:       []string{`action="/onboarding"`, `autocomplete="new-password"`, `minlength="8"`, "Use at least 8 characters."},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, LoginPage("csrf", test.message, "/de/reading", test.onboarding).Render(context.Background(), &output))
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
		assert.Contains(t, css, want, "application CSS missing foundation %q", want)
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
		"/static/vendor/fonts/literata-greek-ext-opsz-normal.woff2",
		"/static/vendor/fonts/literata-greek-ext-opsz-italic.woff2",
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
	assert.Contains(t, css, `url("/static/vendor/fonts/literata-greek-ext-opsz-normal.woff2")`)
	assert.Contains(t, css, `url("/static/vendor/fonts/literata-greek-ext-opsz-italic.woff2")`)
	assert.Equal(t, 2, strings.Count(css, "unicode-range: U+1F00-1FFF;"), "both Greek Extended faces must be range-restricted")
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
		Signals: testNoContent,
	}
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{{Book: domain.Book{ID: "book-1", Title: "Die Verwandlung", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, Acquired: &book}}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))

	html := output.String()
	for _, pattern := range []string{`class="library-masthead"`, `class="library-books"`, `class="library-book__identity-link"`} {
		assert.Contains(t, html, pattern, "library missing shared pattern %q", pattern)
	}
	assert.Contains(t, html, `class="bibliographic-title">`, "book title must use the bibliographic typography role")
	assert.NotContains(t, html, `role="grid"`, "library must use native list semantics")
}

func TestLibraryUsesSharedFeedbackAndEmptyState(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "Book added", "", MyBooksBrowseState{}).Render(context.Background(), &output))

	html := output.String()
	for _, pattern := range []string{
		`class="feedback feedback--success"`,
		`class="empty-state"`,
		`class="empty-state__actions"`,
	} {
		assert.Contains(t, html, pattern, "library missing shared pattern %q", pattern)
	}
}

func TestShellLayoutExposesAccessibleApplicationShell(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, ShellLayout("Mouseion", nil, "csrf-token", NavigationReading).Render(context.Background(), &output))
	for _, pattern := range []string{`class="skip-link"`, `href="#main-content"`, `aria-label="Primary navigation"`, `id="main-content"`, `tabindex="-1"`} {
		assert.Contains(t, output.String(), pattern, "application shell missing %q: %s", pattern, output.String())
	}
}

func TestVocabularyHeaderHasNoPeerViews(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, VocabularyPageHeader().Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `<header class="vocabulary-page-header"><h1>Corpus</h1></header>`)
	assert.NotContains(t, html, "<nav", "Concordance is the only Vocabulary view, so no peer navigation is rendered")
	assert.NotContains(t, html, "/corpus/import", "the retired import surface is not linked")
}

type studyProgressContextStore struct {
	*fixtures.Store
}

func (studyProgressContextStore) ListStudyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil
}
func (studyProgressContextStore) ListStudyProgressLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return []domain.StudyLanguage{{Language: "de", DisplayName: "German"}}, nil
}
func (studyProgressContextStore) ListStudyProgress(context.Context, string, string) ([]domain.StudyProgress, error) {
	return nil, nil
}
func (studyProgressContextStore) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return domain.CurrentReading{BookID: "book-1"}, nil
}
func (studyProgressContextStore) ListVocabularyBrowsePage(_ context.Context, _, _ string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{Page: query.Page}, nil
}

func TestVocabularyBrowsePagerPreservesAppliedPrefixWithoutJavaScript(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, BookWordsPager(bookWordsView{Prefix: "haus", Page: domain.VocabularyBrowsePage{Page: 2, Total: 51, CurrentBookID: "book-1", ReadingBookID: "book-1"}}).Render(context.Background(), &output))
	assert.Contains(t, output.String(), `href="/books/book-1?q=haus"`)
	assert.Contains(t, output.String(), `href="/books/book-1?page=3&amp;q=haus"`)
	assert.Contains(t, output.String(), `hx-get="/books/book-1?page=3&amp;q=haus"`)
	assert.Contains(t, output.String(), `hx-push-url="true"`)
}

func TestVocabularyBrowseHasNoSelectionControls(t *testing.T) {
	page := domain.VocabularyBrowsePage{Page: 2, Total: 51, CurrentBookID: "book-1", ReadingBookID: "book-1", CorpusRevision: "rev-1",
		Books: []domain.VocabularyBrowseBook{{ID: "book-1", Title: "Current Book", HasCurrentAnalysis: true}},
		Rows:  []domain.VocabularyBrowseRow{{CanonicalLemma: "haus", UPOS: "NOUN"}}}
	html := renderBookWords(t, "de", page, "ha")
	assert.Contains(t, html, `id="vocabulary-workflow"`)
	assert.Contains(t, html, `id="vocabulary-browse-results"`)
	assert.Contains(t, html, `href="/de/corpus/concordance?`)
	assert.Contains(t, html, `hx-select="#vocabulary-browse-results"`)
	for _, retired := range []string{"Select</button>", "Remove</button>", "selected identities", "Browse selection", "Custom deck", "/corpus/selection", "/corpus/decks"} {
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
	html := renderBookWords(t, "de", page, "")
	for _, want := range []string{
		"Occurrence counts", "In this Book: 2; Across analyzed books: 5", "In this Book: 3; Across analyzed books: 3",
		">Haus</a>", ">gehen</a>",
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, `name="lemma"`, "Browse is read-only and no longer posts selection changes")

	assert.Contains(t, renderBookWords(t, "it", page, ""), ">haus</a>", "non-German noun display retains canonical casing")
}

func TestVocabularyBrowsePagerPreservesPrefixReadingScopeAndRevision(t *testing.T) {
	page := domain.VocabularyBrowsePage{Page: 2, Total: 51, CurrentBookID: "current-book", ReadingBookID: "current-book", SelectedBooks: []string{"book-1", "book-2"}, SelectedUPOS: []string{"NOUN"}, Sort: "occurrences", CorpusRevision: "revision-1"}
	url := bookWordsPageURL(3, page, "Haus")
	for _, value := range []string{"q=Haus", "/books/current-book", "page=3", "rev=revision-1"} {
		assert.Contains(t, url, value)
	}
	for _, obsolete := range []string{"book=", "pos=", "known=", "reserved=", "sort=", "all=", "reading="} {
		assert.NotContains(t, url, obsolete)
	}
}

func TestVocabularyBrowseRestartDropsOldRevisionAndObsoleteControls(t *testing.T) {
	page := domain.VocabularyBrowsePage{CurrentBookID: "book-1", ReadingBookID: "book-1", SelectedBooks: []string{"book-1"}, SelectedUPOS: []string{"NOUN"}, Sort: "books"}
	url := bookWordsPageURL(1, page, "Haus")
	for _, value := range []string{"q=Haus", "/books/book-1"} {
		assert.Contains(t, url, value)
	}
	for _, obsolete := range []string{"book=", "pos=", "known=", "reserved=", "sort="} {
		assert.NotContains(t, url, obsolete)
	}
	assert.NotContains(t, url, "rev=")
}

type failedVocabularyBrowseStore struct{ studyProgressContextStore }

func (failedVocabularyBrowseStore) ListVocabularyBrowsePage(context.Context, string, string, domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{}, errors.New("database unavailable")
}

func browseTestServices(store allStoreCapabilities) Services {
	return Services{Store: storeDependencies(store)}
}

func TestVocabularyBrowseFailureOffersRetryWithAppliedControls(t *testing.T) {
	_, status, html := loadBrowseForTest(t, browseTestServices(failedVocabularyBrowseStore{}), "/books/book-1?q=Haus&page=2", "book-1")

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Contains(t, html, `id="vocabulary-recovery"`)
	for _, want := range []string{
		"Words could not be loaded", "Your search was not applied", "Retry",
		`name="q" value="Haus"`, `name="page" value="2"`,
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, `name="book"`)
}

type timedOutVocabularyBrowseStore struct{ studyProgressContextStore }

func (timedOutVocabularyBrowseStore) ListVocabularyBrowsePage(ctx context.Context, _, _ string, _ domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	<-ctx.Done()
	return domain.VocabularyBrowsePage{}, ctx.Err()
}

func TestVocabularyBrowseTimeoutReturnsRecoverableGatewayTimeout(t *testing.T) {
	services := browseTestServices(timedOutVocabularyBrowseStore{})
	services.InteractiveReadTimeout = 100 * time.Millisecond
	started := time.Now()
	_, status, html := loadBrowseForTest(t, services, "/books/book-1?q=Haus&page=3", "book-1")

	assert.Equal(t, http.StatusGatewayTimeout, status)
	assert.Less(t, time.Since(started), 5*time.Second)
	for _, want := range []string{"Retry", "shorter one", `name="q" value="Haus"`, `name="page" value="3"`} {
		assert.Contains(t, html, want)
	}
}

type changedVocabularyBrowseStore struct{ studyProgressContextStore }

func (changedVocabularyBrowseStore) ListVocabularyBrowsePage(_ context.Context, _, _ string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{Page: query.Page, CorpusRevision: "revision-new"}, nil
}

func TestVocabularyBrowseChangedEvidenceOffersRestartInsteadOfStalePage(t *testing.T) {
	_, status, html := loadBrowseForTest(t, browseTestServices(changedVocabularyBrowseStore{}), "/books/book-1?q=Haus&page=2&rev=revision-old", "book-1")

	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, html, `id="vocabulary-recovery"`)
	assert.Contains(t, html, "Current evidence changed")
	assert.Contains(t, html, "Refresh words")
	assert.Contains(t, html, "/books/book-1")
	assert.Contains(t, html, `q=Haus`)
	assert.NotContains(t, html, "reserved=")
	assert.NotContains(t, html, "revision-new")
	assert.NotContains(t, html, "rev=")
}

func TestVocabularyBrowseRefusesToApplyARequestAfterCurrentBookChanges(t *testing.T) {
	_, status, html := loadBrowseForTest(t, browseTestServices(changedVocabularyBrowseStore{}), "/reading?q=Haus&reading=book-1&page=2&rev=old-revision", "book-2")

	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, html, "were not applied")
	assert.Contains(t, html, "q=Haus")
	assert.Contains(t, html, "/books/book-2")
	assert.NotContains(t, html, "old-revision")
}

type totalVocabularyBrowseStore struct {
	studyProgressContextStore
	total int64
}

func (s totalVocabularyBrowseStore) ListVocabularyBrowsePage(_ context.Context, _, _ string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{Page: query.Page, Total: s.total, Books: []domain.VocabularyBrowseBook{{ID: query.CurrentBookID, Title: "Current", HasCurrentAnalysis: true}}}, nil
}

// rowVocabularyBrowseStore answers every Browse read with one row, so the
// restored focus row can be checked against a page that contains it.
type rowVocabularyBrowseStore struct{ studyProgressContextStore }

func (rowVocabularyBrowseStore) ListVocabularyBrowsePage(_ context.Context, _, _ string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	return domain.VocabularyBrowsePage{Page: query.Page, Rows: []domain.VocabularyBrowseRow{{UPOS: "NOUN", CanonicalLemma: "haus"}}}, nil
}

func TestVocabularyBrowseRestoresAFocusRowOnlyWhenThePageHasIt(t *testing.T) {
	row := browseRowKey("NOUN", "haus", "")
	view, status, _ := loadBrowseForTest(t, browseTestServices(rowVocabularyBrowseStore{}), "/books/book-1?q=ha&row="+url.QueryEscape(row), "book-1")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, row, view.FocusRow)
	assert.True(t, view.FocusRowFound, "the loaded page contains the named row")
	assert.False(t, view.FocusRowMissing())

	view, status, _ = loadBrowseForTest(t, browseTestServices(rowVocabularyBrowseStore{}), "/books/book-1?q=ha&row="+url.QueryEscape(browseRowKey("VERB", "gehen", "")), "book-1")
	require.Equal(t, http.StatusOK, status)
	assert.False(t, view.FocusRowFound, "a row the page lacks is not reported as found")
	assert.True(t, view.FocusRowMissing())
}

func TestVocabularyBrowseExplainsMalformedAndOutOfRangePagesWithFirstPage(t *testing.T) {
	for _, page := range []string{"abc", "0", "-3", "1.5"} {
		_, status, html := loadBrowseForTest(t, browseTestServices(studyProgressContextStore{}), "/books/book-1?q=ha&page="+page, "book-1")
		assert.Equal(t, http.StatusBadRequest, status, page)
		assert.Contains(t, html, "That page is not available", page)
		assert.Contains(t, html, `>First page</a>`, page)
		assert.Contains(t, html, `href="/books/book-1?q=ha"`, page)
		assert.NotContains(t, html, "Word list", page)
	}
	view, status, html := loadBrowseForTest(t, browseTestServices(totalVocabularyBrowseStore{total: 51}), "/books/book-1?page=4", "book-1")
	assert.Equal(t, bookWordsPageOutOfRange, view.Problem)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, html, "Page 4 is past the end of these results, which have 3.")
	assert.Contains(t, html, `>First page</a>`)
	assert.NotContains(t, html, "Word list")
	_, status, _ = loadBrowseForTest(t, browseTestServices(totalVocabularyBrowseStore{total: 51}), "/books/book-1?page=3", "book-1")
	assert.Equal(t, http.StatusOK, status, "the last page is in range")
	_, status, _ = loadBrowseForTest(t, browseTestServices(totalVocabularyBrowseStore{}), "/books/book-1", "book-1")
	assert.Equal(t, http.StatusOK, status, "an empty first page is not out of range")
	_, status, _ = loadBrowseForTest(t, browseTestServices(totalVocabularyBrowseStore{}), "/books/book-1?page=2", "book-1")
	assert.Equal(t, http.StatusNotFound, status)
}
