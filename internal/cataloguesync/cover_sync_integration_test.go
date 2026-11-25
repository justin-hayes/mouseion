//go:build integration

package cataloguesync

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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
	body            []byte
	contentType     string
	omitContentType bool
	status          int
}

// coverEntry is one configurable Catalog entry. An empty imageHref means the
// entry advertises no cover.
type coverEntry struct {
	id, title, imageHref, imageType string
	omitImageType                   bool
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

func (c *coverCatalog) setFeedError(fail bool) {
	const prefix = "/a"
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
		if !img.omitContentType {
			w.Header().Set("Content-Type", img.contentType)
		}
		w.WriteHeader(http.StatusOK)
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
			if entry.omitImageType {
				testwrite.Fprintf(c.t, &feed, `<link rel="http://opds-spec.org/image" href="%s"/>`, entry.imageHref)
			} else {
				imageType := entry.imageType
				if imageType == "" {
					imageType = "image/png"
				}
				testwrite.Fprintf(c.t, &feed, `<link rel="http://opds-spec.org/image" type="%s" href="%s"/>`, imageType, entry.imageHref)
			}
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

func testJPEG(t *testing.T, width, height int, pixel color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetRGBA(x, y, pixel)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func assertCoverPixelNear(t *testing.T, got color.Color, want color.NRGBA, tolerance uint8) {
	t.Helper()
	actual, ok := color.NRGBAModel.Convert(got).(color.NRGBA)
	require.True(t, ok, "color model converts to color.NRGBA")
	assert.InDelta(t, int(want.R), int(actual.R), float64(tolerance), "red channel")
	assert.InDelta(t, int(want.G), int(actual.G), float64(tolerance), "green channel")
	assert.InDelta(t, int(want.B), int(actual.B), float64(tolerance), "blue channel")
	assert.InDelta(t, int(want.A), int(actual.A), float64(tolerance), "alpha channel")
}

type coverFixture struct {
	t           *testing.T
	ctx         context.Context
	store       *persistence.PostgresStore
	client      *river.Client[pgx.Tx]
	catalog     *coverCatalog
	opdsService *opds.Service
	syncWorker  *Worker
	ownerID     string
	connection  domain.OpdsConnection
}

// blockingEntryCoverStore pauses a retrieval completion at the persistence
// boundary so a test can change the Catalog around it.
type blockingEntryCoverStore struct {
	entryCoverStore
	saveStarted chan struct{}
	releaseSave chan struct{}
}

func (s *blockingEntryCoverStore) SaveCatalogEntryCover(ctx context.Context, key domain.CatalogEntryKey, advertisedAt time.Time, mediaType string, width, height int, contentHash string, bytes []byte) error {
	close(s.saveStarted)
	<-s.releaseSave
	return s.entryCoverStore.SaveCatalogEntryCover(ctx, key, advertisedAt, mediaType, width, height, contentHash, bytes)
}

func newCoverFixture(t *testing.T) *coverFixture {
	t.Helper()
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "cover sync store", store.Close)
	require.NoError(t, migrateRiverForTest(store.Pool()))
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
	testutil.StopOnCleanup(t, "River client", client.Stop)
	opdsService := opds.NewService(store, nil, catalog.server.Client())
	capabilities := fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
	return &coverFixture{
		t: t, ctx: ctx, store: store, client: client, catalog: catalog, opdsService: opdsService,
		syncWorker: &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: opdsService, Capabilities: capabilities, Client: client},
		ownerID:    owner.ID, connection: connection,
	}
}

func (f *coverFixture) sync(connectionID string) error {
	return f.syncWorker.Work(f.ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: f.ownerID, ConnectionID: connectionID}})
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

// coverState is the Book's byte-free cover presentation state.
func (f *coverFixture) coverState(bookID string) string {
	f.t.Helper()
	cover, err := f.store.GetBookCover(f.ctx, f.ownerID, bookID)
	require.NoError(f.t, err)
	return cover.State
}

// selectedEntry reports the Catalog entry a Book's cover resolves through.
func (f *coverFixture) selectedEntry(bookID string) (connectionID, entryIdentifier string) {
	f.t.Helper()
	err := f.store.Pool().QueryRow(f.ctx, `SELECT selected_connection_id::text, selected_source_identifier FROM book_covers WHERE owner_id=$1 AND book_id=$2`, f.ownerID, bookID).Scan(&connectionID, &entryIdentifier)
	if err != nil {
		return "", ""
	}
	return connectionID, entryIdentifier
}

// runAllEntryCovers completes every live entry retrieval job.
func (f *coverFixture) runAllEntryCovers() {
	f.t.Helper()
	for _, args := range f.pendingEntryCoverArgs() {
		require.NoError(f.t, f.runEntryCoverJob(args))
	}
}

// seedBook creates the learner's chosen Book for an entry, so sync has a
// linked Book. Sync itself never creates Books.
func (f *coverFixture) seedBook(t *testing.T, connectionID string, entry coverEntry) {
	t.Helper()
	require.NoError(t, f.store.ReconcileCatalogueIndex(f.ctx, f.ownerID, connectionID, []domain.CatalogueIndexEntry{{
		ConnectionID: connectionID, EntryIdentifier: entry.id, Title: entry.title,
		LanguageTag: "de", EPUBHref: f.catalog.server.URL + "/" + entry.id + ".epub",
	}}, time.Now()))
	_, err := f.store.AddCatalogueEntry(f.ctx, f.ownerID, connectionID, entry.id)
	require.NoError(t, err)
}

// establishCover adds the entry as a Book, syncs, completes the entry's
// retrieval, and returns the Book with the bytes its cover resolves to.
func (f *coverFixture) establishCover(t *testing.T, entry coverEntry, image coverImage) (domain.Book, []byte) {
	t.Helper()
	f.catalog.setImage(entry.imageHref, image)
	f.catalog.setEntries("/a", entry)
	f.seedBook(t, f.connection.ID, entry)
	require.NoError(t, f.sync(f.connection.ID))
	book := f.bookByTitle(entry.title)
	args := f.pendingEntryCoverArgs()
	require.Len(t, args, 1)
	require.NoError(t, f.runEntryCoverJob(args[0]))
	resource, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	require.NotEmpty(t, resource.Bytes)
	return book, resource.Bytes
}

// addAlias gives a Book a second Catalog-entry alias on another connection.
func (f *coverFixture) addAlias(bookID, connectionID, entryIdentifier string) {
	f.t.Helper()
	_, err := f.store.Pool().Exec(f.ctx, `INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value) VALUES($1,$2,$3,$4,$5,$6)`,
		f.ownerID, bookID, connectionID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, entryIdentifier)
	require.NoError(f.t, err)
}

func TestCatalogueSyncBookCoverResolvesThroughRetainedEntryCover(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))
	connectionID, entryID := f.selectedEntry(book.ID)
	assert.Equal(t, f.connection.ID, connectionID)
	assert.Equal(t, "entry-1", entryID)
	entryCover, err := f.entryCover("entry-1")
	require.NoError(t, err)
	assert.Equal(t, retained, entryCover.Bytes, "the Book and the Catalog card share one image")

	// An already-retained advertisement is refetched on each sync, so an image
	// replaced at the same URL is picked up (ADR 0077 refresh).
	require.NoError(t, f.sync(f.connection.ID))
	assert.Len(t, f.pendingEntryCoverArgs(), 2, "retained cover was not scheduled for refresh")
}

func TestCatalogueSyncOversizedCalibreWebJPEGPreservesColorAndReplacesRetainedBlackRaster(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{
		id: "calibre-entry", title: "Calibre-Web Book", imageHref: "/a/image/calibre.jpg", imageType: "image/jpeg",
	}
	black := testJPEG(t, 1200, 1800, color.RGBA{A: 255})
	book, retained := f.establishCover(t, entry, coverImage{body: black, contentType: "image/jpeg"})
	retainedImage, err := jpeg.Decode(bytes.NewReader(retained))
	require.NoError(t, err)
	assert.Equal(t, color.NRGBA{A: 255}, color.NRGBAModel.Convert(retainedImage.At(300, 450)))

	// A changed advertised image is replaced by a normal sync; a stale
	// completion cannot overwrite the retained raster.
	colored := testJPEG(t, 1200, 1800, color.RGBA{R: 220, G: 40, B: 30, A: 255})
	f.catalog.setImage("/a/image/calibre-v2.jpg", coverImage{body: colored, contentType: "image/jpeg"})
	entry.imageHref = "/a/image/calibre-v2.jpg"
	f.catalog.setEntries("/a", entry)
	require.NoError(t, f.sync(f.connection.ID))
	args := f.pendingEntryCoverArgs()
	require.Len(t, args, 2)
	replacement := newestEntryCoverArgs(t, args)
	for _, candidate := range args {
		if candidate != replacement {
			require.NoError(t, f.runEntryCoverJob(candidate))
		}
	}
	stillRetained, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, stillRetained.Bytes, "stale completion replaced the retained raster")
	require.NoError(t, f.runEntryCoverJob(replacement))

	resource, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, 600, resource.Width)
	assert.Equal(t, 900, resource.Height)
	updated, err := jpeg.Decode(bytes.NewReader(resource.Bytes))
	require.NoError(t, err)
	assertCoverPixelNear(t, updated.At(300, 450), color.NRGBA{R: 220, G: 40, B: 30, A: 255}, 12)
}

func TestCatalogueSyncBookCoverCompletionAfterConnectionDeletionKeepsRetainedCover(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	f.catalog.setImage("/a/image/two.png", coverImage{body: testPNG(t, 200), contentType: "image/png"})
	entry.imageHref = "/a/image/two.png"
	f.catalog.setEntries("/a", entry)
	require.NoError(t, f.sync(f.connection.ID))
	current := newestEntryCoverArgs(t, f.pendingEntryCoverArgs())

	blocking := &blockingEntryCoverStore{entryCoverStore: f.store, saveStarted: make(chan struct{}), releaseSave: make(chan struct{})}
	worker := &EntryCoverWorker{Connections: f.store, Catalogue: blocking, Reader: f.opdsService}
	completed := make(chan error, 1)
	go func() { completed <- worker.Work(f.ctx, &river.Job[EntryCoverArgs]{Args: current}) }()
	<-blocking.saveStarted
	require.NoError(t, f.store.DeleteOpdsConnection(f.ctx, f.ownerID, f.connection.ID))
	close(blocking.releaseSave)
	require.NoError(t, <-completed)

	// The Book still resolves to an image through its stored selection.
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))
	resource, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, resource.Bytes)
	assert.NotEmpty(t, retained)
}

func TestCatalogueSyncBookCoverValidatesEveryMediaTypeClaim(t *testing.T) {
	tests := []struct {
		name            string
		imageType       string
		omitImageType   bool
		contentType     string
		omitContentType bool
		wantAvailable   bool
	}{
		{name: "all claims agree after normalization", imageType: "IMAGE/PNG; charset=binary", contentType: "image/png; charset=utf-8", wantAvailable: true},
		{name: "opds claim disagrees with decoded content", imageType: "image/jpeg", contentType: "image/png", wantAvailable: false},
		{name: "response claim disagrees with decoded content", imageType: "image/png", contentType: "image/jpeg", wantAvailable: false},
		{name: "optional claims omitted", omitImageType: true, omitContentType: true, wantAvailable: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCoverFixture(t)
			entry := coverEntry{
				id: "entry-1", title: "Book One", imageHref: "/a/image/one.png",
				imageType: tt.imageType, omitImageType: tt.omitImageType,
			}
			f.catalog.setImage(entry.imageHref, coverImage{
				body: testPNG(t, 10), contentType: tt.contentType, omitContentType: tt.omitContentType,
			})
			f.catalog.setEntries("/a", entry)
			f.seedBook(t, f.connection.ID, entry)
			require.NoError(t, f.sync(f.connection.ID), "cover validation must not fail catalog reconciliation")
			status, err := f.store.GetCatalogueSyncStatus(f.ctx, f.ownerID, f.connection.ID)
			require.NoError(t, err)
			assert.Equal(t, domain.CatalogueSyncSynced, status.State)

			book := f.bookByTitle(entry.title)
			args := f.pendingEntryCoverArgs()
			require.Len(t, args, 1)
			err = f.runEntryCoverJob(args[0])
			if tt.wantAvailable {
				require.NoError(t, err)
				assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))
				return
			}

			require.Error(t, err)
			assert.Equal(t, domain.BookCoverUnavailable, f.coverState(book.ID))
			_, err = f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
			assert.ErrorIs(t, err, persistence.ErrNotFound)
		})
	}
}

func TestCatalogueSyncBookCoverMediaTypeMismatchPreservesReplacement(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	entry.imageType = "image/png"
	entry.imageHref = "/a/image/two.png"
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 200), contentType: "image/jpeg"})
	f.catalog.setEntries("/a", entry)
	require.NoError(t, f.sync(f.connection.ID))
	require.Error(t, f.runEntryCoverJob(newestEntryCoverArgs(t, f.pendingEntryCoverArgs())))

	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))
	preserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)
}

func TestCatalogueSyncBookCoverAbsencePreservesOnUncertainty(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	// A missing entry preserves the image a Book uses.
	f.catalog.setEntries("/a")
	require.NoError(t, f.sync(f.connection.ID))
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))

	// A failed feed traversal fails sync but preserves it.
	f.catalog.setEntries("/a", entry)
	f.catalog.setFeedError(true)
	require.Error(t, f.sync(f.connection.ID))
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))
	f.catalog.setFeedError(false)

	// An invalid replacement preserves the prior image and does not fail sync.
	f.catalog.setImage("/a/image/bad.png", coverImage{body: []byte("not an image"), contentType: "image/png"})
	entry.imageHref = "/a/image/bad.png"
	f.catalog.setEntries("/a", entry)
	require.NoError(t, f.sync(f.connection.ID))
	require.Error(t, f.runEntryCoverJob(newestEntryCoverArgs(t, f.pendingEntryCoverArgs())))
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))
	status, err := f.store.GetCatalogueSyncStatus(f.ctx, f.ownerID, f.connection.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.CatalogueSyncSynced, status.State)
	preserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)

	// A complete reconciliation that finds the selected entry with no image
	// link is explicit absence: the Book's selection and the entry row go.
	f.catalog.setEntries("/a", coverEntry{id: "entry-1", title: "Book One"})
	require.NoError(t, f.sync(f.connection.ID))
	assert.Equal(t, domain.BookCoverNone, f.coverState(book.ID))
	_, err = f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.ErrorIs(t, err, persistence.ErrNotFound)
	_, err = f.entryCover("entry-1")
	require.ErrorIs(t, err, persistence.ErrNotFound)
}

func TestCatalogueSyncBookCoverStableMultiAliasSelectionAndFallback(t *testing.T) {
	f := newCoverFixture(t)
	second, err := f.store.CreateOpdsConnection(f.ctx, f.ownerID, domain.OpdsConnection{Name: "Catalog B", URL: f.catalog.baseURL("/b")})
	require.NoError(t, err)
	entryA := coverEntry{id: "entry-a", title: "Aliased Book", imageHref: "/a/image/a.png"}
	book, retainedA := f.establishCover(t, entryA, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	f.addAlias(book.ID, second.ID, "entry-b")
	f.catalog.setImage("/b/image/b.png", coverImage{body: testPNG(t, 220), contentType: "image/png"})
	f.catalog.setEntries("/b", coverEntry{id: "entry-b", title: "Aliased Book", imageHref: "/b/image/b.png"})

	// A later alias completing does not displace the selected cover.
	require.NoError(t, f.sync(second.ID))
	for _, args := range f.pendingEntryCoverArgs() {
		if args.ConnectionID == second.ID {
			require.NoError(t, f.runEntryCoverJob(args))
		}
	}
	connectionID, entryID := f.selectedEntry(book.ID)
	assert.Equal(t, f.connection.ID, connectionID)
	assert.Equal(t, "entry-a", entryID)
	resource, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retainedA, resource.Bytes)

	// Explicit absence from the selected entry demotes the Book to the other
	// alias's already-retained image in the same reconciliation.
	f.catalog.setEntries("/a", coverEntry{id: "entry-a", title: "Aliased Book"})
	require.NoError(t, f.sync(f.connection.ID))
	connectionID, entryID = f.selectedEntry(book.ID)
	assert.Equal(t, second.ID, connectionID)
	assert.Equal(t, "entry-b", entryID)
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))
	fallback, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.NotEqual(t, retainedA, fallback.Bytes)
}

func TestCatalogueSyncBookCoverRetainedAcrossEntryLeavingIndexMembershipAndConnectionRemoval(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})
	late := f.pendingEntryCoverArgs()[0]

	// Removing active My Books membership preserves the cover.
	require.NoError(t, f.store.RemoveBookFromMyBooks(f.ctx, f.ownerID, book.ID))
	preserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)

	// An entry that leaves the index keeps the image a Book uses.
	f.catalog.setEntries("/a")
	require.NoError(t, f.sync(f.connection.ID))
	preserved, err = f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)

	// Deleting the connection keeps it too, without the alias, and a late
	// retrieval job for the deleted connection changes nothing.
	require.NoError(t, f.store.DeleteOpdsConnection(f.ctx, f.ownerID, f.connection.ID))
	connectionID, entryID := f.selectedEntry(book.ID)
	assert.Equal(t, f.connection.ID, connectionID)
	assert.Equal(t, "entry-1", entryID)
	_, err = f.store.GetBookCatalogEntryAliasForConnection(f.ctx, f.ownerID, book.ID, f.connection.ID)
	require.ErrorIs(t, err, persistence.ErrNotFound)
	require.NoError(t, f.runEntryCoverJob(late))
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID))
	stillPreserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, stillPreserved.Bytes)
}

func TestCatalogueSyncAddingEntryWithRetainedCoverShowsItWithoutRetrieval(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 10), contentType: "image/png"})
	f.catalog.setEntries("/a", entry)
	require.NoError(t, f.sync(f.connection.ID))
	f.runAllEntryCovers()
	jobsBefore := len(f.pendingEntryCoverArgs())

	_, err := f.store.AddCatalogueEntry(f.ctx, f.ownerID, f.connection.ID, "entry-1")
	require.NoError(t, err)
	book := f.bookByTitle("Book One")
	assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID), "the retained image shows at once")
	resource, err := f.store.GetActiveBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, resource.Bytes)
	assert.Len(t, f.pendingEntryCoverArgs(), jobsBefore, "adding a Book scheduled a retrieval")
}
