//go:build integration

package webapp

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	originBackPattern   = regexp.MustCompile(`id="concordance-origin-return" href="([^"]+)"`)
	originStudyPattern  = regexp.MustCompile(`class="concordance-study-link"[^>]* href="([^"]+)"`)
	originReturnPattern = regexp.MustCompile(`<a href="([^"]+)">Return to Concordance results</a>`)
)

// TestBrowseConcordanceStudyRoundTripRestoresOrExplainsOverHTTP walks a Reading
// Browse excursion through exact Concordance lookup, a new search, Study, and
// back, then shows each way the origin can stop being valid.
func TestBrowseConcordanceStudyRoundTripRestoresOrExplainsOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "browse-origin-secret-0123456789abcdef")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "origin-alice", "alice-password")
	bob := createAccount(t, ctx, store, "origin-bob", "bob-password")
	lemmas := make([]string, 30)
	for i := range lemmas {
		lemmas[i] = fmt.Sprintf("wort%02d", i)
	}
	seed := func(owner, key, title string) (domain.Book, domain.SourceMaterial, domain.Corpus) {
		book, source, corpus := seedMigrationAnalyzedBook(t, ctx, store, owner, key, title, []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "wort00", UPOS: "NOUN", OccurrenceCount: 1}})
		seedBrowseEvidenceTokens(t, ctx, store, source, corpus, lemmas)
		return book, source, corpus
	}
	first, _, _ := seed(alice.ID, "origin-first", "Erstes Buch")
	second, _, _ := seed(alice.ID, "origin-second", "Zweites Buch")
	bobBook, _, _ := seed(bob.ID, "origin-bob", "Bobs Buch")
	for _, book := range []domain.Book{first, second} {
		buildBrowseProjection(t, ctx, store, book.ID)
		require.NoError(t, store.SetBookDisposition(ctx, alice.ID, book.ID, domain.BookDispositionToRead))
	}
	buildBrowseProjection(t, ctx, store, bobBook.ID)
	// A second study language makes a deliberate switch possible.
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Italian catalog", URL: "https://italian.example/opds"})
	require.NoError(t, err)
	err = seedWebappCatalogueBook(t, ctx, store, alice.ID, connection.ID, "it-entry", "Un libro", "Autrice", "it")
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))
	_, err = store.StartCurrentReading(ctx, alice.ID, "de", first.ID)
	require.NoError(t, err)

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "origin-alice", "alice-password")
	bobCookies, _ := loginCookies(t, h, "origin-bob", "bob-password")
	get := func(path string) *httptest.ResponseRecorder { return perform(t, h, http.MethodGet, path, nil, cookies) }
	link := func(pattern *regexp.Regexp, body string) string {
		t.Helper()
		match := pattern.FindStringSubmatch(body)
		require.NotNil(t, match, "no link matching %s", pattern)
		return html.UnescapeString(match[1])
	}
	rowID := browseRowElementID(browseRowKey("NOUN", "wort26", ""))

	// Browse page 2 → exact Concordance for one row.
	browse := get(bookDetailURL(first.ID) + "?page=2")
	require.Equal(t, http.StatusOK, browse.Code, browse.Header().Get("Location"))
	assert.Contains(t, browse.Body.String(), "Page 2 of 2")
	concordanceHref := link(regexp.MustCompile(`href="(/de/corpus/concordance\?[^"]*term=wort26[^"]*)"`), browse.Body.String())
	parsed, err := url.Parse(concordanceHref)
	require.NoError(t, err)
	assert.Equal(t, "lemma", parsed.Query().Get("as"))
	assert.Equal(t, "NOUN", parsed.Query().Get("upos"))
	assert.Equal(t, first.ID, parsed.Query().Get(originKeyBook))
	assert.Equal(t, "2", parsed.Query().Get(originKeyPage))
	assert.Equal(t, "NOUN:wort26", parsed.Query().Get(originKeyRow))
	assert.NotEmpty(t, parsed.Query().Get(originKeyRevision))
	assert.Empty(t, parsed.Query().Get("priority"), "the origin never supplies the captured priority")

	exact := get(concordanceHref)
	require.Equal(t, http.StatusOK, exact.Code)
	assert.Contains(t, exact.Body.String(), ">Forms of wort26 · noun</h2>")
	back := link(originBackPattern, exact.Body.String())
	assert.Contains(t, exact.Body.String(), "Back to book vocabulary")

	t.Run("a new independent search keeps the origin but not the exact identity", func(t *testing.T) {
		search := url.Values{"term": {"wort27"}}
		for key, value := range parsed.Query() {
			if _, ok := map[string]bool{originKeyBook: true, originKeyPage: true, originKeyRow: true, originKeyRevision: true}[key]; ok {
				search[key] = value
			}
		}
		response := get("/de/corpus/concordance?" + search.Encode())
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, back, link(originBackPattern, response.Body.String()))
		assert.Contains(t, response.Body.String(), `name="from_book" value="`+first.ID+`"`, "the search form carries the origin separately from the typed term")
	})

	t.Run("Study returns to the applied Concordance state and Back restores the row", func(t *testing.T) {
		study := get(link(originStudyPattern, exact.Body.String()))
		require.Equal(t, http.StatusOK, study.Code)
		returned := link(originReturnPattern, study.Body.String())
		assert.Contains(t, returned, "from_book="+first.ID)
		assert.Contains(t, returned, "focus=occurrence-")
		// A browser never sends the fragment; it only positions the page.
		again := get(strings.SplitN(returned, "#", 2)[0])
		require.Equal(t, http.StatusOK, again.Code)
		assert.NotContains(t, again.Body.String(), "concordance-focus-missing")
		assert.Equal(t, back, link(originBackPattern, again.Body.String()))

		restored := get(back)
		require.Equal(t, http.StatusOK, restored.Code)
		assert.Contains(t, restored.Body.String(), "Page 2 of 2")
		assert.Regexp(t, `<tr id="`+rowID+`" tabindex="-1" autofocus`, restored.Body.String())
		assert.NotContains(t, restored.Body.String(), `<h3 id="vocabulary-results-heading" tabindex="-1" autofocus`)
		assert.NotContains(t, restored.Body.String(), "vocabulary-focus-row-missing")
	})

	t.Run("a missing focus target alone explains and focuses the heading", func(t *testing.T) {
		missingRow := get(returnWithParam(t, back, "row", "NOUN:fehlt"))
		require.Equal(t, http.StatusOK, missingRow.Code)
		assert.Contains(t, missingRow.Body.String(), "vocabulary-focus-row-missing")
		assert.Contains(t, missingRow.Body.String(), `<h3 id="vocabulary-results-heading" tabindex="-1" autofocus`)
		assert.NotContains(t, missingRow.Body.String(), `<tr id="`+rowID+`" tabindex="-1" autofocus`)

		missingOccurrence := get(returnWithFocus(t, strings.SplitN(link(originReturnPattern, get(link(originStudyPattern, exact.Body.String())).Body.String()), "#", 2)[0], "occurrence-gone"))
		require.Equal(t, http.StatusOK, missingOccurrence.Code)
		assert.Contains(t, missingOccurrence.Body.String(), "concordance-focus-missing")
		assert.Regexp(t, `<h2 id="concordance-summary" tabindex="-1" autofocus`, missingOccurrence.Body.String())
	})

	t.Run("malformed and out-of-range paging explain First page instead of clamping", func(t *testing.T) {
		malformed := get(returnWithParam(t, back, "page", "abc"))
		assert.Equal(t, http.StatusBadRequest, malformed.Code)
		assert.Contains(t, malformed.Body.String(), "First page")
		outOfRange := get(returnWithParam(t, back, "page", "9"))
		assert.Equal(t, http.StatusNotFound, outOfRange.Code)
		assert.Contains(t, outOfRange.Body.String(), "First page")
		assert.NotContains(t, outOfRange.Body.String(), "No eligible vocabulary")
	})

	t.Run("invalid or foreign origins are dropped and expose nothing", func(t *testing.T) {
		for name, mutate := range map[string]func(url.Values){
			"page zero":       func(v url.Values) { v.Set(originKeyPage, "0") },
			"missing book":    func(v url.Values) { v.Del(originKeyBook) },
			"oversized value": func(v url.Values) { v.Set(originKeyPrefix, string(make([]byte, maxOriginValueLength+1))) },
		} {
			values := parsed.Query()
			mutate(values)
			response := get("/de/corpus/concordance?" + values.Encode())
			assert.Equal(t, http.StatusOK, response.Code, name)
			assert.NotContains(t, response.Body.String(), "Back to book vocabulary", name)
		}
		// Another owner copying the results sees the lookup but no origin.
		copied := perform(t, h, http.MethodGet, concordanceHref, nil, bobCookies)
		assert.Equal(t, http.StatusOK, copied.Code)
		assert.NotContains(t, copied.Body.String(), "Back to book vocabulary")
		assert.NotContains(t, copied.Body.String(), first.ID)
		// A return pointer at another owner's Book is not found, never opened.
		foreign := perform(t, h, http.MethodGet, back, nil, bobCookies)
		assert.Equal(t, http.StatusNotFound, foreign.Code)
		assert.NotContains(t, foreign.Body.String(), "Erstes Buch")
		assert.NotContains(t, foreign.Body.String(), `id="vocabulary-workflow"`)
	})

	t.Run("changed evidence needs an explicit Refresh words that keeps the prefix", func(t *testing.T) {
		changed := get(returnWithParam(t, returnWithParam(t, back, "q", "wort"), "rev", "stale-revision"))
		require.Equal(t, http.StatusConflict, changed.Code)
		assert.Contains(t, changed.Body.String(), "Refresh words")
		assert.NotContains(t, changed.Body.String(), "browse-row-")
		refresh := link(regexp.MustCompile(`role="button" href="([^"]+)">Refresh words`), changed.Body.String())
		refreshed := get(refresh)
		require.Equal(t, http.StatusOK, refreshed.Code)
		refreshedURL, err := url.Parse(refresh)
		require.NoError(t, err)
		assert.Equal(t, bookDetailURL(first.ID), refreshedURL.Path)
		assert.Equal(t, "wort", refreshedURL.Query().Get("q"))
		assert.Empty(t, refreshedURL.Query().Get("page"), "refresh restarts at the first page")
		assert.Empty(t, refreshedURL.Query().Get("rev"), "refresh drops the stale revision")
		assert.Empty(t, refreshedURL.Query().Get("snapshot"), "refresh never carries a retired commitment token")
		assert.Contains(t, refreshed.Body.String(), "Page 1 of 2")
	})

	t.Run("the Current reading never gates a Book's words", func(t *testing.T) {
		require.NoError(t, store.EndCurrentReading(ctx, alice.ID, "de"))
		ended := get(back)
		assert.Equal(t, http.StatusOK, ended.Code)
		assert.Contains(t, ended.Body.String(), "Page 2 of 2")

		_, err = store.StartCurrentReading(ctx, alice.ID, "de", second.ID)
		require.NoError(t, err)
		switched := get(back)
		assert.Equal(t, http.StatusOK, switched.Code)
		assert.Contains(t, switched.Body.String(), "Erstes Buch")
		assert.NotContains(t, switched.Body.String(), "Zweites Buch", "another Book's controls and words are never transferred")
	})

	t.Run("a Book-addressed page runs in the Book's language", func(t *testing.T) {
		response := get(back)
		assert.Equal(t, http.StatusOK, response.Code)
		stored, err := store.GetStoredActiveStudyLanguage(ctx, alice.ID)
		require.NoError(t, err)
		assert.Equal(t, "de", stored)
		// Concordance names its language in the URL, so the page renders in it
		// and the visit records it as the Last study language (ADR 0131).
		concordance := get(concordanceHref)
		assert.Equal(t, http.StatusOK, concordance.Code, concordanceHref)
		stored, err = store.GetStoredActiveStudyLanguage(ctx, alice.ID)
		require.NoError(t, err)
		assert.Equal(t, "de", stored)
	})
}

func returnWithParam(t *testing.T, raw, key, value string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func returnWithFocus(t *testing.T, raw, focus string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("focus", focus)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
