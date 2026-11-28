//go:build integration

package webapp

import (
	"context"
	"html"
	"net/http"
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

// TestCorpusPagesAreAddressedByStudyLanguage holds ADR 0131 for the
// Corpus, Concordance and Sentence study pages: the URL names the language,
// an old address redirects to the prefixed form, and the picker lands a deep
// page on the Concordance landing of the chosen language.
func TestCorpusPagesAreAddressedByStudyLanguage(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-language-url-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	alice := createAccount(t, ctx, store, "language-url-alice", "alice-password")
	german, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "language-url-de", "Deutsch", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 1}})
	seedLookupSentences(t, ctx, store, german, "de", [][]lookupToken{{{"Wir", "wir", "PRON"}, {"gehen", "gehen", "VERB"}}})
	italian, italianSource, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "language-url-it", "Libro", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "andare", UPOS: "VERB", OccurrenceCount: 1}})
	setLookupBookLanguage(t, ctx, store, italian, italianSource, "it")
	seedLookupSentences(t, ctx, store, italian, "it", [][]lookupToken{{{"Andiamo", "andare", "VERB"}}})
	require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "language-url-alice", "alice-password")
	type result struct {
		code           int
		body, location string
	}
	get := func(path string) result {
		response := perform(t, h, http.MethodGet, path, nil, cookies)
		return result{code: response.Code, body: response.Body.String(), location: response.Header().Get("Location")}
	}
	stored := func() string {
		language, err := store.GetStoredActiveStudyLanguage(ctx, alice.ID)
		require.NoError(t, err)
		return language
	}

	t.Run("a prefixed address renders that language and records it", func(t *testing.T) {
		response := get("/it/corpus/concordance?term=andare")
		require.Equal(t, http.StatusOK, response.code)
		assert.Contains(t, response.body, "Andiamo")
		assert.Contains(t, response.body, `action="/it/corpus/concordance"`)
		assert.NotContains(t, response.body, `name="language"`)
		assert.Equal(t, "it", stored())
		require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))
	})
	t.Run("two languages are two addresses", func(t *testing.T) {
		assert.Contains(t, get("/de/corpus/concordance?term=gehen").body, "gehen")
		assert.NotContains(t, get("/de/corpus/concordance?term=andare").body, "Andiamo")
	})
	t.Run("the landing page is prefixed too", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, get("/de/corpus").code)
		assert.Equal(t, "/de/corpus", get("/corpus").location)
	})
	t.Run("an old address redirects to the Last study language", func(t *testing.T) {
		response := get("/corpus/concordance?term=gehen&as=lemma")
		assert.Equal(t, http.StatusSeeOther, response.code)
		assert.Equal(t, "/de/corpus/concordance?as=lemma&term=gehen", response.location)
	})
	t.Run("a valid query language names the prefix and is stripped", func(t *testing.T) {
		response := get("/corpus/concordance?language=it&term=andare")
		assert.Equal(t, http.StatusSeeOther, response.code)
		assert.Equal(t, "/it/corpus/concordance?term=andare", response.location)
	})
	t.Run("an invalid query language uses the Last study language", func(t *testing.T) {
		response := get("/corpus/concordance?language=fr&term=gehen")
		assert.Equal(t, "/de/corpus/concordance?term=gehen", response.location)
	})
	t.Run("a stale prefix on a deep page falls back to the landing page", func(t *testing.T) {
		response := get("/fr/corpus/concordance/sentence?sentence=0&target=1")
		assert.Equal(t, http.StatusSeeOther, response.code)
		assert.Equal(t, "/de/corpus/concordance", response.location)
	})
	t.Run("a language the learner does not study redirects to the Last study language", func(t *testing.T) {
		response := get("/fr/corpus/concordance?term=gehen")
		assert.Equal(t, http.StatusSeeOther, response.code)
		assert.Equal(t, "/de/corpus/concordance?term=gehen", response.location)
	})
	t.Run("Sentence study is prefixed", func(t *testing.T) {
		response := get("/corpus/concordance/sentence?language=de&sentence=0&target=1")
		assert.Equal(t, "/de/corpus/concordance/sentence?sentence=0&target=1", response.location)
		unknown := get("/de/corpus/concordance/sentence?sentence=0&target=1&book=missing")
		assert.Equal(t, http.StatusNotFound, unknown.code)
	})
	t.Run("a retired Vocabulary address redirects to Corpus in one hop", func(t *testing.T) {
		for _, tc := range []struct{ from, to string }{
			{"/vocabulary", "/de/corpus"},
			{"/vocabulary/concordance?term=gehen&as=lemma", "/de/corpus/concordance?as=lemma&term=gehen"},
			{"/vocabulary/concordance?language=it&term=andare", "/it/corpus/concordance?term=andare"},
			{"/vocabulary/concordance/sentence?language=de&sentence=0&target=1", "/de/corpus/concordance/sentence?sentence=0&target=1"},
			{"/de/vocabulary", "/de/corpus"},
			{"/de/vocabulary/concordance?term=gehen&as=lemma", "/de/corpus/concordance?term=gehen&as=lemma"},
			{"/de/vocabulary/concordance/sentence?sentence=0&target=1", "/de/corpus/concordance/sentence?sentence=0&target=1"},
			{"/fr/vocabulary/concordance?term=gehen", "/de/corpus/concordance?term=gehen"},
			{"/fr/vocabulary/concordance/sentence?sentence=0&target=1", "/de/corpus/concordance"},
		} {
			response := get(tc.from)
			assert.Equal(t, http.StatusSeeOther, response.code, tc.from)
			assert.Equal(t, tc.to, response.location, tc.from)
		}
	})
	t.Run("a retired Sentence study link keeps its saved search and focus", func(t *testing.T) {
		results := get("/de/corpus/concordance?term=gehen&as=lemma")
		link := regexp.MustCompile(`href="(/de/corpus/concordance/sentence\?[^"]+)"`).FindStringSubmatch(results.body)
		require.NotNil(t, link, "a result links to Sentence study")
		current, err := url.Parse(html.UnescapeString(link[1]))
		require.NoError(t, err)
		query := current.Query()
		back := query.Get("return")
		require.Contains(t, back, "/de/corpus/concordance?", "the current return path")
		query.Set("return", strings.Replace(back, "/de/corpus/", "/de/vocabulary/", 1))
		retired := get("/de/vocabulary/concordance/sentence?" + query.Encode())
		require.Equal(t, http.StatusSeeOther, retired.code)
		followed := get(retired.location)
		require.Equal(t, http.StatusOK, followed.code)
		assert.Contains(t, followed.body, html.EscapeString(back), "the way back is the saved search, focus included")
	})
	t.Run("results link to prefixed pages", func(t *testing.T) {
		response := get("/de/corpus/concordance?term=gehen")
		assert.Contains(t, response.body, `href="/de/corpus/concordance/sentence?`)
		assert.Contains(t, response.body, "return=%2Fde%2Fcorpus%2Fconcordance")
	})
	t.Run("the picker lands a deep page on the Concordance landing of the chosen language", func(t *testing.T) {
		for _, kind := range []string{"sentence-study", "concordance", "corpus"} {
			t.Run(kind, func(t *testing.T) {
				response := get("/study-language?to=" + kind + "&lang=it")
				require.Equal(t, http.StatusSeeOther, response.code)
				require.Equal(t, "/it/corpus/concordance", response.location)
			})
		}
	})
}
