//go:build integration

package webapp

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The To Read tab is in title order by default and offers a most-readable
// sort by Mature coverage, kept in the address: the highest share of running
// words Mature first, then title, and Books without a coverage figure last
// (ADR 0136 §3).
func TestMyBooksToReadSortsByMatureCoverageOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "my-books-sort-secret-01234567890")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openWebappIntegrationStore(t, ctx)
	owner := createAccount(t, ctx, store, "my-books-sort", "learner-password")
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, "de"))

	// Alpha Low: 2 of 4 running words Mature (50%).
	alpha, alphaSource, alphaCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "sort-alpha", "Alpha Low", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 2},
		{Language: "de", CanonicalLemma: "extraa", UPOS: "NOUN", OccurrenceCount: 2},
	})
	seedBrowseEvidenceTokens(t, ctx, store, alphaSource, alphaCorpus, []string{"alpha", "alpha", "extraa", "extraa"})
	seedStudyWordIn(t, ctx, store, owner.ID, "de", "alpha", true)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, alpha.ID, domain.BookDispositionToRead))
	buildBrowseProjection(t, ctx, store, alpha.ID)

	// Zulu High: 4 of 5 running words Mature (80%).
	zulu, zuluSource, zuluCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "sort-zulu", "Zulu High", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "zulu", UPOS: "NOUN", OccurrenceCount: 4},
		{Language: "de", CanonicalLemma: "extraz", UPOS: "NOUN", OccurrenceCount: 1},
	})
	seedBrowseEvidenceTokens(t, ctx, store, zuluSource, zuluCorpus, []string{"zulu", "zulu", "zulu", "zulu", "extraz"})
	seedStudyWordIn(t, ctx, store, owner.ID, "de", "zulu", true)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, zulu.ID, domain.BookDispositionToRead))
	buildBrowseProjection(t, ctx, store, zulu.ID)

	// Mike None: a metadata-only To Read Book with no analysis, so no coverage
	// figure sorts it last.
	mike, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Mike None", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, mike.ID, domain.BookDispositionToRead))

	// The projection grounds the expected order: 50%, 80% and no figure.
	browse, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	coverage := map[string]domain.MyBook{}
	for _, book := range browse.Items {
		coverage[book.Book.Title] = book
	}
	assert.EqualValues(t, 2, coverage["Alpha Low"].CoverageMatureTokens)
	assert.EqualValues(t, 4, coverage["Alpha Low"].CoverageTotalTokens)
	assert.EqualValues(t, 4, coverage["Zulu High"].CoverageMatureTokens)
	assert.EqualValues(t, 5, coverage["Zulu High"].CoverageTotalTokens)
	assert.EqualValues(t, 0, coverage["Mike None"].CoverageTotalTokens)

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, owner.Username, "learner-password")

	titleOrder := func(body string) bool {
		return strings.Index(body, `id="book-row-`+alpha.ID+`"`) < strings.Index(body, `id="book-row-`+mike.ID+`"`) &&
			strings.Index(body, `id="book-row-`+mike.ID+`"`) < strings.Index(body, `id="book-row-`+zulu.ID+`"`)
	}
	readableOrder := func(body string) bool {
		return strings.Index(body, `id="book-row-`+zulu.ID+`"`) < strings.Index(body, `id="book-row-`+alpha.ID+`"`) &&
			strings.Index(body, `id="book-row-`+alpha.ID+`"`) < strings.Index(body, `id="book-row-`+mike.ID+`"`)
	}

	t.Run("the default is title order", func(t *testing.T) {
		page := perform(t, h, http.MethodGet, "/de/library?disposition=to_read", nil, cookies)
		require.Equal(t, http.StatusOK, page.Code, page.Body.String())
		body := page.Body.String()
		assert.True(t, titleOrder(body), "the default tab is not in title order: %s", body)
		assert.False(t, readableOrder(body), "the default tab must not be coverage-ordered")
		assert.Contains(t, body, "Most readable first")
		assert.Contains(t, body, "/de/library?disposition=to_read&amp;sort=readable", "the sort control is an address")
	})

	t.Run("the most-readable sort orders by Mature coverage then title", func(t *testing.T) {
		page := perform(t, h, http.MethodGet, "/de/library?disposition=to_read&sort=readable", nil, cookies)
		require.Equal(t, http.StatusOK, page.Code, page.Body.String())
		body := page.Body.String()
		assert.True(t, readableOrder(body), "the sorted tab is not in Mature coverage order: %s", body)
		assert.False(t, titleOrder(body), "the sorted tab must not be in title order")
		assert.Contains(t, body, "Title order", "the control offers the way back to the default")
		assert.Contains(t, body, `name="sort" value="readable"`, "a search keeps the sort in the address")
	})

	t.Run("an unknown sort value is unsupported", func(t *testing.T) {
		refused := perform(t, h, http.MethodGet, "/de/library?disposition=to_read&sort=bogus", nil, cookies)
		assert.Equal(t, http.StatusNotFound, refused.Code)
	})
}
