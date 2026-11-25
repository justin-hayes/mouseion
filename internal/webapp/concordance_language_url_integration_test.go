//go:build integration

package webapp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVocabularyPagesAreAddressedByStudyLanguage holds ADR 0131 for the
// Vocabulary, Concordance and Sentence study pages: the URL names the language,
// an old address redirects to the prefixed form, and the picker lands a deep
// page on the Concordance landing of the chosen language.
func TestVocabularyPagesAreAddressedByStudyLanguage(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-language-url-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	alice := createAccount(t, ctx, store, "language-url-alice", "alice-password")
	german, _, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "language-url-de", "Deutsch", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 1}})
	seedLookupSentences(t, ctx, store, german, "de", [][]lookupToken{{{"Wir", "wir", "PRON"}, {"gehen", "gehen", "VERB"}}})
	italian, italianSource, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "language-url-it", "Libro", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "andare", UPOS: "VERB", OccurrenceCount: 1}})
	setLookupBookLanguage(t, ctx, store, italian, italianSource, "it")
	seedLookupSentences(t, ctx, store, italian, "it", [][]lookupToken{{{"Andiamo", "andare", "VERB"}}})
	require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: prepareddeck.NewService(store, nil), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
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
		response := get("/it/vocabulary/concordance?term=andare")
		require.Equal(t, http.StatusOK, response.code)
		assert.Contains(t, response.body, "Andiamo")
		assert.Contains(t, response.body, `action="/it/vocabulary/concordance"`)
		assert.NotContains(t, response.body, `name="language"`)
		assert.Equal(t, "it", stored())
		require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))
	})
	t.Run("two languages are two addresses", func(t *testing.T) {
		assert.Contains(t, get("/de/vocabulary/concordance?term=gehen").body, "gehen")
		assert.NotContains(t, get("/de/vocabulary/concordance?term=andare").body, "Andiamo")
	})
	t.Run("the landing page is prefixed too", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, get("/de/vocabulary").code)
		assert.Equal(t, "/de/vocabulary", get("/vocabulary").location)
	})
	t.Run("an old address redirects to the Last study language", func(t *testing.T) {
		response := get("/vocabulary/concordance?term=gehen&as=lemma")
		assert.Equal(t, http.StatusSeeOther, response.code)
		assert.Equal(t, "/de/vocabulary/concordance?as=lemma&term=gehen", response.location)
	})
	t.Run("a valid query language names the prefix and is stripped", func(t *testing.T) {
		response := get("/vocabulary/concordance?language=it&term=andare")
		assert.Equal(t, http.StatusSeeOther, response.code)
		assert.Equal(t, "/it/vocabulary/concordance?term=andare", response.location)
	})
	t.Run("an invalid query language uses the Last study language", func(t *testing.T) {
		response := get("/vocabulary/concordance?language=fr&term=gehen")
		assert.Equal(t, "/de/vocabulary/concordance?term=gehen", response.location)
	})
	t.Run("a stale prefix on a deep page falls back to the landing page", func(t *testing.T) {
		response := get("/fr/vocabulary/concordance/sentence?sentence=0&target=1")
		assert.Equal(t, http.StatusSeeOther, response.code)
		assert.Equal(t, "/de/vocabulary/concordance", response.location)
	})
	t.Run("a language the learner does not study redirects to the Last study language", func(t *testing.T) {
		response := get("/fr/vocabulary/concordance?term=gehen")
		assert.Equal(t, http.StatusSeeOther, response.code)
		assert.Equal(t, "/de/vocabulary/concordance?term=gehen", response.location)
	})
	t.Run("Sentence study is prefixed", func(t *testing.T) {
		response := get("/vocabulary/concordance/sentence?language=de&sentence=0&target=1")
		assert.Equal(t, "/de/vocabulary/concordance/sentence?sentence=0&target=1", response.location)
		unknown := get("/de/vocabulary/concordance/sentence?sentence=0&target=1&book=missing")
		assert.Equal(t, http.StatusNotFound, unknown.code)
	})
	t.Run("results link to prefixed pages", func(t *testing.T) {
		response := get("/de/vocabulary/concordance?term=gehen")
		assert.Contains(t, response.body, `href="/de/vocabulary/concordance/sentence?`)
		assert.Contains(t, response.body, "return=%2Fde%2Fvocabulary%2Fconcordance")
	})
	t.Run("the picker lands a deep page on the Concordance landing of the chosen language", func(t *testing.T) {
		for _, kind := range []string{"sentence-study", "concordance", "vocabulary"} {
			t.Run(kind, func(t *testing.T) {
				response := get("/study-language?to=" + kind + "&lang=it")
				require.Equal(t, http.StatusSeeOther, response.code)
				require.Equal(t, "/it/vocabulary/concordance", response.location)
			})
		}
	})
}
