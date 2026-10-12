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

// concordanceContextHandler opens a migrated store and returns a handler and
// signed-in cookies for one freshly created account.
func concordanceContextHandler(t *testing.T, ctx context.Context, username, password string) (*Handler, []*http.Cookie) {
	t.Helper()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	createAccount(t, ctx, store, username, password)
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: prepareddeck.NewService(store, nil), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, username, password)
	return h, cookies
}

// TestConcordanceWithoutALearnerLanguageRendersTheInvitation covers the page's
// language resolution: a learner who has no study language yet sees the blank
// lookup rather than a lookup against an unavailable language.
func TestConcordanceWithoutALearnerLanguageRendersTheInvitation(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-no-language-secret-0123456789")
	ctx := context.Background()
	h, cookies := concordanceContextHandler(t, ctx, "no-language-learner", "password")

	response := perform(t, h, http.MethodGet, "/vocabulary/concordance?term=gehen", nil, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	t.Run("offers the no-study-language empty state", func(t *testing.T) {
		assert.Contains(t, body, "No study languages yet")
	})
	t.Run("renders no results", func(t *testing.T) {
		assert.NotContains(t, body, `id="concordance-results"`)
	})
}

// TestConcordanceDropsAnOriginBookInAnotherStudyLanguage covers the origin
// validation: a carried Browse origin that names the owner's Book in a
// different study language is dropped rather than exposed.
func TestConcordanceDropsAnOriginBookInAnotherStudyLanguage(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-origin-language-secret")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "origin-language-alice", "alice-password")
	german, _, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "origin-language-de", "Deutsch", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 1}})
	seedLookupSentences(t, ctx, store, german, "de", [][]lookupToken{{{"Wir", "wir", "PRON"}, {"gehen", "gehen", "VERB"}}})
	italian, italianSource, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "origin-language-it", "Libro", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 1}})
	setLookupBookLanguage(t, ctx, store, italian, italianSource, "it")
	require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: prepareddeck.NewService(store, nil), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "origin-language-alice", "alice-password")

	response := perform(t, h, http.MethodGet, "/vocabulary/concordance?language=de&term=gehen&from_book="+italian.ID+"&from_snap=snapshot", nil, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	require.Contains(t, body, ">Forms of gehen</h2>", "the lookup still renders")
	t.Run("drops the origin pointer", func(t *testing.T) {
		assert.NotContains(t, body, "Back to book vocabulary")
	})
	t.Run("exposes nothing from the other Book", func(t *testing.T) {
		assert.NotContains(t, body, "Libro")
	})
}
