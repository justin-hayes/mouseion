//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// visibilityDiscoveryReader offers one German EPUB through the catalogue, so
// the real sync worker discovers a brand-new Book.
type visibilityDiscoveryReader struct{}

func (visibilityDiscoveryReader) Languages(context.Context, string, string) (opds.Feed, error) {
	return opds.Feed{Entries: []opds.Entry{
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://discovery.example/opds/language/7"}}},
	}}, nil
}

func (visibilityDiscoveryReader) BrowseLanguage(_ context.Context, _, _, languageID string) (opds.Feed, error) {
	if languageID != "7" {
		return opds.Feed{}, nil
	}
	return opds.Feed{Entries: []opds.Entry{{
		ID: "discovered-entry", Title: "Discovered visible", Author: "Discovery author",
		Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://discovery.example/opds/epub/discovered-entry"}},
	}}}, nil
}

func retiredVisibilityHandler(store *persistence.PostgresStore) http.Handler {
	authService := auth.New(store, time.Hour)
	return New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store), PreparedDeck: prepareddeck.NewService(store, nil), SessionLifetime: time.Hour,
	})
}

// seedRetiredVisibilityToReadBook creates an analyzed German To Read Book that
// the Reading chooser can assess and start.
func seedRetiredVisibilityToReadBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, suffix, title, lemma string) domain.Book {
	t.Helper()
	counts := []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", OccurrenceCount: 3}}
	book, _, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner, suffix, title, counts)
	publishProjectedCounts(t, ctx, store, owner, book.ID, counts)
	testutil.SeedCorpusWords(t, ctx, store.Pool(), owner, corpus.ID, "de", testutil.CorpusWord{Lemma: lemma, UPOS: "NOUN", Occurrences: 3})
	require.NoError(t, store.SetBookDisposition(ctx, owner, book.ID, domain.BookDispositionToRead))
	return book
}

// TestRetiredHiddenBookAppearsAsAnOrdinaryToReadBook holds the retirement: a
// book_visibility row no longer hides a Book anywhere, and Hide, Unhide and the
// Work-level actions are unsupported routes.
func TestRetiredHiddenBookAppearsAsAnOrdinaryToReadBook(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "retired-hidden-book-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openWebappIntegrationStore(t, ctx)
	owner := createAccount(t, ctx, store, "retired-hidden", "learner-password")
	book := seedRetiredVisibilityToReadBook(t, ctx, store, owner.ID, "retired-hidden", "Previously hidden", "haus")
	_, err := store.Pool().Exec(ctx, `INSERT INTO book_visibility(owner_id, book_id, hidden, revision) VALUES ($1, $2, true, 1)`, owner.ID, book.ID)
	require.NoError(t, err)
	h := retiredVisibilityHandler(store)
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	startPath := "/reading/books/" + book.ID + "/start"

	chooser := perform(t, h, http.MethodGet, "/de/reading", nil, cookies)
	require.Equal(t, http.StatusOK, chooser.Code)
	assert.Contains(t, chooser.Body.String(), startPath, "a previously hidden To Read Book is an ordinary candidate")
	assert.Contains(t, chooser.Body.String(), book.Title)

	library := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code)
	assert.Contains(t, library.Body.String(), book.Title, "a previously hidden Book is listed as an ordinary Book")
	assert.NotContains(t, library.Body.String(), "Show hidden books")
	assert.NotContains(t, library.Body.String(), "hidden book is not shown")

	for _, path := range []string{
		"/library/books/" + book.ID + "/hide",
		"/library/books/" + book.ID + "/unhide",
		"/library/works/any-work/hide",
		"/library/works/any-work/unhide",
	} {
		response := perform(t, h, http.MethodPost, path, url.Values{"csrf_token": {csrf}}, cookies)
		assert.Equal(t, http.StatusNotFound, response.Code, "retired route %s is unsupported", path)
	}

	var hidden bool
	var revision int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT hidden, revision FROM book_visibility WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&hidden, &revision))
	assert.True(t, hidden, "the retired row is left in place")
	assert.EqualValues(t, 1, revision)
}

func TestCatalogueSyncDoesNotCreateBooks(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "hidden-discovery-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openWebappIntegrationStore(t, ctx)
	owner := createAccount(t, ctx, store, "hidden-discovery", "learner-password")
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Discovery catalog", URL: "https://discovery.example/opds"})
	require.NoError(t, err)
	worker := &cataloguesync.Worker{
		Connections: store, Catalogue: store, Statuses: store, Reader: visibilityDiscoveryReader{},
		Capabilities: staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}},
	}
	require.NoError(t, worker.Work(ctx, &river.Job[cataloguesync.SyncArgs]{Args: cataloguesync.SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}))

	t.Run("sync leaves My Books empty", func(t *testing.T) {
		books, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 20)
		require.NoError(t, err)
		require.Empty(t, books.Items, "sync created a Book and placed it in My Books")
	})
	t.Run("sync maintains the Catalog index", func(t *testing.T) {
		entries, err := store.ListCatalogueEntries(ctx, owner.ID, connection.ID)
		require.NoError(t, err)
		require.Len(t, entries, 1, "sync did not maintain the Catalog index")
		require.Equal(t, "Discovered visible", entries[0].Title)
	})
	t.Run("sync activates no membership", func(t *testing.T) {
		var memberships int
		err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1`, owner.ID).Scan(&memberships)
		require.NoError(t, err)
		require.Zero(t, memberships, "sync activated My Books membership")
	})
}
