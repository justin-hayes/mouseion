//go:build integration

package cataloguesync

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coverImage is one configurable image response.
type coverImage struct {
	body        []byte
	contentType string
	status      int
}

// coverEntry is one configurable Catalog entry. An empty imageHref means the
// entry advertises no cover.
type coverEntry struct {
	id, title, imageHref string
}

// coverCatalog is a minimal HTTP OPDS catalog and image host with mutable
// state so a test can change feeds and images between syncs.
type coverCatalog struct {
	t          *testing.T
	server     *httptest.Server
	mu         sync.Mutex
	entries    map[string][]coverEntry
	images     map[string]coverImage
	feedErrors map[string]bool
}

func newCoverCatalog(t *testing.T) *coverCatalog {
	t.Helper()
	catalog := &coverCatalog{
		t:          t,
		entries:    map[string][]coverEntry{},
		images:     map[string]coverImage{},
		feedErrors: map[string]bool{},
	}
	catalog.server = httptest.NewServer(http.HandlerFunc(catalog.handle))
	return catalog
}

func (c *coverCatalog) Close() { c.server.Close() }

func (c *coverCatalog) baseURL(prefix string) string { return c.server.URL + prefix + "/opds" }

func (c *coverCatalog) setEntries(prefix string, entries ...coverEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[prefix] = entries
}

func (c *coverCatalog) setImage(href string, img coverImage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.images[href] = img
}

func (c *coverCatalog) setFeedError(prefix string, fail bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.feedErrors[prefix] = fail
}

func (c *coverCatalog) handle(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := r.URL.Path
	if img, ok := c.images[path]; ok {
		if img.status != 0 {
			w.WriteHeader(img.status)
			return
		}
		w.Header().Set("Content-Type", img.contentType)
		testwrite.Bytes(c.t, w, img.body)
		return
	}
	if base, ok := strings.CutSuffix(path, "/language/7"); ok {
		prefix := strings.TrimSuffix(base, "/opds")
		if c.feedErrors[prefix] {
			http.Error(w, "feed unavailable", http.StatusInternalServerError)
			return
		}
		c.writeLanguageFeed(w, base, prefix)
		return
	}
	if base, ok := strings.CutSuffix(path, "/language"); ok {
		c.writeLanguagesFeed(w, base)
		return
	}
	http.NotFound(w, r)
}

func (c *coverCatalog) writeLanguagesFeed(w http.ResponseWriter, base string) {
	w.Header().Set("Content-Type", "application/atom+xml")
	testwrite.Fprintf(c.t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Languages</title><entry><id>%s/language/7</id><title>German</title><link rel="subsection" type="application/atom+xml" href="%s/language/7"/></entry></feed>`, base, base)
}

func (c *coverCatalog) writeLanguageFeed(w http.ResponseWriter, base, prefix string) {
	w.Header().Set("Content-Type", "application/atom+xml")
	var feed strings.Builder
	feed.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title>`)
	for _, entry := range c.entries[prefix] {
		testwrite.Fprintf(c.t, &feed, `<entry><id>%s</id><title>%s</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="%s/%s.epub"/>`, entry.id, entry.title, base, entry.id)
		if entry.imageHref != "" {
			testwrite.Fprintf(c.t, &feed, `<link rel="http://opds-spec.org/image" type="image/png" href="%s"/>`, entry.imageHref)
		}
		feed.WriteString(`</entry>`)
	}
	feed.WriteString(`</feed>`)
	testwrite.String(c.t, w, feed.String())
}

// testPNG returns a small valid PNG whose pixels depend on value, so a changed
// image produces a different normalized content hash.
func testPNG(t *testing.T, value uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	for y := range 6 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: value, G: 255 - value, B: value, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

type coverFixture struct {
	t           *testing.T
	ctx         context.Context
	store       *persistence.PostgresStore
	client      *river.Client[pgx.Tx]
	catalog     *coverCatalog
	opdsService *opds.Service
	syncWorker  *Worker
	coverWorker *CoverWorker
	ownerID     string
	connection  domain.OpdsConnection
}

func newCoverFixture(t *testing.T) *coverFixture {
	t.Helper()
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "cover sync store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, store.Pool()))
	owner, err := store.CreateUser(ctx, "cover-sync-owner", false)
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "de", "German")
	require.NoError(t, err)
	catalog := newCoverCatalog(t)
	t.Cleanup(catalog.Close)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Catalog A", URL: catalog.baseURL("/a")})
	require.NoError(t, err)
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := client.Stop(context.Background()); err != nil {
			t.Logf("stop river client: %v", err)
		}
	})
	opdsService := opds.NewService(store, nil, catalog.server.Client())
	capabilities := fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
	return &coverFixture{
		t: t, ctx: ctx, store: store, client: client, catalog: catalog, opdsService: opdsService,
		syncWorker:  &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: opdsService, Capabilities: capabilities, Client: client},
		coverWorker: &CoverWorker{Connections: store, Catalogue: store, Reader: opdsService, Capabilities: capabilities},
		ownerID:     owner.ID, connection: connection,
	}
}

func (f *coverFixture) sync(connectionID string) error {
	return f.syncWorker.Work(f.ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: f.ownerID, ConnectionID: connectionID}})
}

// pendingCoverArgs returns the live cover jobs River currently holds.
func (f *coverFixture) pendingCoverArgs() []CoverArgs {
	f.t.Helper()
	rows, err := f.store.Pool().Query(f.ctx, `SELECT args FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND state::text=ANY($3::text[]) ORDER BY id`, CoverKind, f.ownerID, liveRiverStates())
	require.NoError(f.t, err)
	defer rows.Close()
	var args []CoverArgs
	for rows.Next() {
		var raw []byte
		require.NoError(f.t, rows.Scan(&raw))
		var decoded CoverArgs
		require.NoError(f.t, json.Unmarshal(raw, &decoded))
		args = append(args, decoded)
	}
	require.NoError(f.t, rows.Err())
	return args
}

func (f *coverFixture) runCoverJob(args CoverArgs) error {
	return f.coverWorker.Work(f.ctx, &river.Job[CoverArgs]{Args: args})
}

func (f *coverFixture) bookByTitle(title string) domain.Book {
	f.t.Helper()
	books, err := f.store.ListMyBooks(f.ctx, f.ownerID)
	require.NoError(f.t, err)
	for _, book := range books {
		if book.Title == title {
			return book
		}
	}
	f.t.Fatalf("book %q not found in My Books", title)
	return domain.Book{}
}

func (f *coverFixture) coverState(bookID string) domain.BookCoverRetrieval {
	f.t.Helper()
	cover, err := f.store.GetBookCoverForRetrieval(f.ctx, f.ownerID, bookID)
	require.NoError(f.t, err)
	return cover
}

func (f *coverFixture) establishCover(t *testing.T, entry coverEntry, image coverImage) (domain.Book, []byte) {
	t.Helper()
	f.catalog.setImage(entry.imageHref, image)
	f.catalog.setEntries("/a", entry)
	require.NoError(t, f.sync(f.connection.ID))
	book := f.bookByTitle(entry.title)
	args := f.pendingCoverArgs()
	require.Len(t, args, 1)
	require.NoError(t, f.runCoverJob(args[0]))
	resource, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	require.NotEmpty(t, resource.Bytes)
	return book, resource.Bytes
}

func TestCatalogueSyncCoverInitialRetrievalRepeatAndReplacement(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, _ := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	cover := f.coverState(book.ID)
	require.Equal(t, domain.BookCoverAvailable, cover.State)
	require.Equal(t, f.connection.ID, cover.SelectedConnectionID)
	require.Equal(t, "entry-1", cover.SelectedSourceIdentifier)
	first, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)

	// Repeated sync is deduplicated and repeated retrieval converges on one
	// retained display image rather than creating history.
	require.NoError(t, f.sync(f.connection.ID))
	require.Len(t, f.pendingCoverArgs(), 1)
	require.NoError(t, f.runCoverJob(f.pendingCoverArgs()[0]))
	repeated, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, first.ContentHash, repeated.ContentHash)

	// A changed image from the selected entry leaves the prior image visible
	// until the replacement validates and swaps atomically.
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 200), contentType: "image/png"})
	require.NoError(t, f.sync(f.connection.ID))
	beforeReplacement, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, first.ContentHash, beforeReplacement.ContentHash, "prior image was replaced before the new image validated")
	require.NoError(t, f.runCoverJob(f.pendingCoverArgs()[0]))
	afterReplacement, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.NotEqual(t, first.ContentHash, afterReplacement.ContentHash)
}

func TestCatalogueSyncCoverAbsencePreservesOnUncertainty(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	// A missing entry preserves the retained image.
	f.catalog.setEntries("/a")
	require.NoError(t, f.sync(f.connection.ID))
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID).State)

	// A failed feed traversal fails sync but preserves the retained image.
	f.catalog.setEntries("/a", entry)
	f.catalog.setFeedError("/a", true)
	require.Error(t, f.sync(f.connection.ID))
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID).State)
	f.catalog.setFeedError("/a", false)

	// An invalid replacement preserves the prior image and stays diagnosable
	// without failing catalog sync.
	f.catalog.setImage(entry.imageHref, coverImage{body: []byte("not an image"), contentType: "image/png"})
	require.NoError(t, f.sync(f.connection.ID))
	require.Error(t, f.runCoverJob(f.pendingCoverArgs()[0]))
	cover := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.NotEmpty(t, cover.FailureReason)
	status, err := f.store.GetCatalogueSyncStatus(f.ctx, f.ownerID, f.connection.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.CatalogueSyncSynced, status.State)
	preserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)

	// A complete successful reconciliation that finds the selected entry with
	// no recognized image link removes that selected cover.
	f.catalog.setEntries("/a", coverEntry{id: "entry-1", title: "Book One"})
	require.NoError(t, f.sync(f.connection.ID))
	assert.Equal(t, domain.BookCoverNone, f.coverState(book.ID).State)
	_, err = f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.ErrorIs(t, err, persistence.ErrNotFound)
}

func TestCatalogueSyncCoverStableMultiAliasSelectionAndFallback(t *testing.T) {
	f := newCoverFixture(t)
	second, err := f.store.CreateOpdsConnection(f.ctx, f.ownerID, domain.OpdsConnection{Name: "Catalog B", URL: f.catalog.baseURL("/b")})
	require.NoError(t, err)
	entryA := coverEntry{id: "entry-a", title: "Aliased Book", imageHref: "/a/image/a.png"}
	book, _ := f.establishCover(t, entryA, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	// The same Book gains a Catalog-entry alias on the second connection.
	_, err = f.store.Pool().Exec(f.ctx, `INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value) VALUES($1,$2,$3,$4,$5,$6)`,
		f.ownerID, book.ID, second.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, "entry-b")
	require.NoError(t, err)
	f.catalog.setImage("/b/image/b.png", coverImage{body: testPNG(t, 220), contentType: "image/png"})
	f.catalog.setEntries("/b", coverEntry{id: "entry-b", title: "Aliased Book", imageHref: "/b/image/b.png"})

	// A later sync of another alias cannot replace the selected cover, and
	// schedules no retrieval for it.
	require.NoError(t, f.sync(second.ID))
	selected := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, selected.State)
	assert.Equal(t, f.connection.ID, selected.SelectedConnectionID)
	assert.Equal(t, "entry-a", selected.SelectedSourceIdentifier)
	require.Len(t, f.pendingCoverArgs(), 1, "another alias scheduled retrieval for an already-selected cover")

	// Explicit absence from the selected entry removes the cover.
	f.catalog.setEntries("/a", coverEntry{id: "entry-a", title: "Aliased Book"})
	require.NoError(t, f.sync(f.connection.ID))
	require.Equal(t, domain.BookCoverNone, f.coverState(book.ID).State)

	// With no selected cover remaining, the other live alias may establish the
	// fallback through the same atomic first-success rule.
	require.NoError(t, f.sync(second.ID))
	pending := f.coverState(book.ID)
	require.Equal(t, domain.BookCoverPending, pending.State)
	require.Equal(t, second.ID, pending.SelectedConnectionID)
	require.Equal(t, "entry-b", pending.SelectedSourceIdentifier)
	args := f.pendingCoverArgs()
	require.Len(t, args, 2, "expected the retained entry-a job and the new entry-b job")
	var fallback CoverArgs
	for _, candidate := range args {
		if candidate.SourceIdentifier == "entry-b" {
			fallback = candidate
		}
	}
	require.NotEmpty(t, fallback.BookID)
	require.NoError(t, f.runCoverJob(fallback))
	fallbackCover := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, fallbackCover.State)
	assert.Equal(t, second.ID, fallbackCover.SelectedConnectionID)
}

func TestCatalogueSyncCoverRetainedAcrossMembershipAndConnectionRemoval(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})
	args := f.pendingCoverArgs()
	require.Len(t, args, 1)

	// Removing active My Books membership preserves cover state and bytes.
	require.NoError(t, f.store.RemoveBookFromMyBooks(f.ctx, f.ownerID, book.ID))
	require.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID).State)
	preserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)

	// Deleting the live Catalog connection preserves the retained cover and its
	// durable Catalog-entry provenance while preventing future retrieval.
	require.NoError(t, f.store.DeleteOpdsConnection(f.ctx, f.ownerID, f.connection.ID))
	afterDelete := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, afterDelete.State)
	assert.Equal(t, f.connection.ID, afterDelete.SelectedConnectionID)
	assert.Equal(t, "entry-1", afterDelete.SelectedSourceIdentifier)
	_, err = f.store.GetBookCatalogEntryAliasForConnection(f.ctx, f.ownerID, book.ID, f.connection.ID)
	require.ErrorIs(t, err, persistence.ErrNotFound)
	require.NoError(t, f.runCoverJob(args[0]))
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID).State)
	stillPreserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, stillPreserved.Bytes)
}
