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
	"time"

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

type blockingCoverStore struct {
	coverStore
	saveStarted chan struct{}
	releaseSave chan struct{}
}

func (s *blockingCoverStore) SaveBookCover(ctx context.Context, owner, bookID, connectionID, sourceIdentifier string, advertisedAt time.Time, mediaType string, width, height int, contentHash string, bytes []byte) error {
	close(s.saveStarted)
	<-s.releaseSave
	return s.coverStore.SaveBookCover(ctx, owner, bookID, connectionID, sourceIdentifier, advertisedAt, mediaType, width, height, contentHash, bytes)
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

	// Repeated sync keeps the selected source stable while creating a new
	// retrieval generation. The old live job must not suppress the replacement.
	require.NoError(t, f.sync(f.connection.ID))
	repeatArgs := f.pendingCoverArgs()
	require.Len(t, repeatArgs, 2)
	assert.NotEqual(t, repeatArgs[0].AdvertisedAt, repeatArgs[1].AdvertisedAt)
	var currentArgs, staleArgs CoverArgs
	for _, args := range repeatArgs {
		if args.AdvertisedAt.After(cover.AdvertisedAt) {
			currentArgs = args
		} else {
			staleArgs = args
		}
	}
	require.NotEmpty(t, currentArgs.AdvertisedAt)
	require.NotEmpty(t, staleArgs.AdvertisedAt)
	require.NoError(t, f.runCoverJob(staleArgs))
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
	replacementArgs := f.pendingCoverArgs()
	require.Len(t, replacementArgs, 3)
	var newestArgs CoverArgs
	for _, args := range replacementArgs {
		if args.AdvertisedAt.After(newestArgs.AdvertisedAt) {
			newestArgs = args
		}
	}
	require.NoError(t, f.runCoverJob(currentArgs))
	intermediate, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, first.ContentHash, intermediate.ContentHash, "stale replacement completed after a newer reconciliation")
	require.NoError(t, f.runCoverJob(newestArgs))
	afterReplacement, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.NotEqual(t, first.ContentHash, afterReplacement.ContentHash)
}

func TestCatalogueSyncCoverDuplicateFailureAfterSuccessIsIgnored(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})
	args := f.pendingCoverArgs()
	require.Len(t, args, 1)

	// A rescued failure from the successful initial job must not add a
	// diagnostic or alter the retained image.
	f.catalog.setImage(entry.imageHref, coverImage{body: []byte("not an image"), contentType: "image/png"})
	require.Error(t, f.runCoverJob(args[0]))
	require.Error(t, f.runCoverJob(args[0]))
	cover := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.Empty(t, cover.FailureReason)
	preserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)

	// A new selected-source generation is still allowed to record its own
	// failure while retaining the prior validated image.
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 200), contentType: "image/png"})
	require.NoError(t, f.sync(f.connection.ID))
	replacementArgs := f.pendingCoverArgs()
	require.Len(t, replacementArgs, 2)
	var current CoverArgs
	for _, candidate := range replacementArgs {
		if candidate.AdvertisedAt.After(current.AdvertisedAt) {
			current = candidate
		}
	}
	require.NotEmpty(t, current.AdvertisedAt)
	f.catalog.setImage(entry.imageHref, coverImage{body: []byte("still not an image"), contentType: "image/png"})
	require.Error(t, f.runCoverJob(current))
	failed := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, failed.State)
	assert.NotEmpty(t, failed.FailureReason)
	require.Error(t, f.runCoverJob(current))
	repeated := f.coverState(book.ID)
	assert.Equal(t, failed.FailureReason, repeated.FailureReason)
	preserved, err = f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)

	// The same replacement candidate can recover, and a later duplicate failure
	// remains fenced after that success as well.
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 220), contentType: "image/png"})
	require.NoError(t, f.runCoverJob(current))
	recovered := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, recovered.State)
	assert.Empty(t, recovered.FailureReason)
	f.catalog.setImage(entry.imageHref, coverImage{body: []byte("late duplicate"), contentType: "image/png"})
	require.Error(t, f.runCoverJob(current))
	late := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, late.State)
	assert.Empty(t, late.FailureReason)
}

func TestCatalogueSyncCoverStaleWorkerCompletionAfterReplacementReconciliation(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})
	oldArgs := f.pendingCoverArgs()[0]
	blocking := &blockingCoverStore{coverStore: f.store, saveStarted: make(chan struct{}), releaseSave: make(chan struct{})}
	worker := &CoverWorker{Connections: f.store, Catalogue: blocking, Reader: f.opdsService, Capabilities: fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}}
	completed := make(chan error, 1)
	go func() {
		completed <- worker.Work(f.ctx, &river.Job[CoverArgs]{Args: oldArgs})
	}()

	// The old worker has fetched and normalized its image, but has not yet
	// persisted it. Reconciliation now advances the selected-source generation.
	<-blocking.saveStarted
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 200), contentType: "image/png"})
	require.NoError(t, f.sync(f.connection.ID))
	args := f.pendingCoverArgs()
	require.Len(t, args, 2)
	var currentArgs CoverArgs
	for _, candidate := range args {
		if candidate.AdvertisedAt.After(currentArgs.AdvertisedAt) {
			currentArgs = candidate
		}
	}
	require.True(t, currentArgs.AdvertisedAt.After(oldArgs.AdvertisedAt))
	close(blocking.releaseSave)
	require.NoError(t, <-completed)

	intermediate, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, intermediate.Bytes)
	require.NoError(t, f.runCoverJob(currentArgs))
	updated, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.NotEqual(t, retained, updated.Bytes)
}

func TestCatalogueSyncCoverCompletionAfterConnectionDeletionPreservesRetainedCover(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	// Reconciliation creates a current replacement candidate while retaining the
	// validated image. Delete the connection after retrieval has reached the
	// persistence boundary, then let the completion continue.
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 200), contentType: "image/png"})
	require.NoError(t, f.sync(f.connection.ID))
	args := f.pendingCoverArgs()
	require.Len(t, args, 2)
	var current CoverArgs
	for _, candidate := range args {
		if candidate.AdvertisedAt.After(current.AdvertisedAt) {
			current = candidate
		}
	}
	require.NotEmpty(t, current.BookID)

	blocking := &blockingCoverStore{coverStore: f.store, saveStarted: make(chan struct{}), releaseSave: make(chan struct{})}
	worker := &CoverWorker{Connections: f.store, Catalogue: blocking, Reader: f.opdsService, Capabilities: fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}}
	completed := make(chan error, 1)
	go func() {
		completed <- worker.Work(f.ctx, &river.Job[CoverArgs]{Args: current})
	}()
	<-blocking.saveStarted

	require.NoError(t, f.store.DeleteOpdsConnection(f.ctx, f.ownerID, f.connection.ID))
	close(blocking.releaseSave)
	require.NoError(t, <-completed)

	cover := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	resource, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, resource.Bytes)
}

func TestCatalogueSyncCoverValidatesEveryMediaTypeClaim(t *testing.T) {
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
			require.NoError(t, f.sync(f.connection.ID), "cover validation must not fail catalog reconciliation")
			status, err := f.store.GetCatalogueSyncStatus(f.ctx, f.ownerID, f.connection.ID)
			require.NoError(t, err)
			assert.Equal(t, domain.CatalogueSyncSynced, status.State)

			book := f.bookByTitle(entry.title)
			args := f.pendingCoverArgs()
			require.Len(t, args, 1)
			err = f.runCoverJob(args[0])
			if tt.wantAvailable {
				require.NoError(t, err)
				assert.Equal(t, domain.BookCoverAvailable, f.coverState(book.ID).State)
				return
			}

			require.Error(t, err)
			cover := f.coverState(book.ID)
			assert.Equal(t, domain.BookCoverUnavailable, cover.State)
			assert.NotEmpty(t, cover.FailureReason)
			_, err = f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
			assert.ErrorIs(t, err, persistence.ErrNotFound)
		})
	}
}

func TestCatalogueSyncCoverFailedInitialCandidateLeavesAliasFallbackEligible(t *testing.T) {
	f := newCoverFixture(t)
	second, err := f.store.CreateOpdsConnection(f.ctx, f.ownerID, domain.OpdsConnection{Name: "Catalog B", URL: f.catalog.baseURL("/b")})
	require.NoError(t, err)
	entryA := coverEntry{id: "entry-a", title: "Fallback Book", imageHref: "/a/image/a.png"}
	f.catalog.setImage(entryA.imageHref, coverImage{body: []byte("not an image"), contentType: "image/png"})
	f.catalog.setEntries("/a", entryA)
	require.NoError(t, f.sync(f.connection.ID))
	book := f.bookByTitle(entryA.title)
	_, err = f.store.Pool().Exec(f.ctx, `INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value) VALUES($1,$2,$3,$4,$5,$6)`,
		f.ownerID, book.ID, second.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, "entry-b")
	require.NoError(t, err)

	entryB := coverEntry{id: "entry-b", title: entryA.title, imageHref: "/b/image/b.png"}
	f.catalog.setImage(entryB.imageHref, coverImage{body: testPNG(t, 220), contentType: "image/png"})
	f.catalog.setEntries("/b", entryB)
	require.NoError(t, f.sync(second.ID))
	// Re-advertising the provisional source must refresh its retrieval without
	// fencing out the fallback candidate from the same generation.
	require.NoError(t, f.sync(f.connection.ID))

	var first, fallback CoverArgs
	for _, candidate := range f.pendingCoverArgs() {
		if candidate.BookID != book.ID {
			continue
		}
		switch candidate.SourceIdentifier {
		case entryA.id:
			first = candidate
		case entryB.id:
			fallback = candidate
		}
	}
	require.NotEmpty(t, first.BookID)
	require.NotEmpty(t, fallback.BookID)
	require.Error(t, f.runCoverJob(first))
	assert.Equal(t, domain.BookCoverPending, f.coverState(book.ID).State)
	require.NoError(t, f.runCoverJob(fallback))
	cover := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.Equal(t, second.ID, cover.SelectedConnectionID)
	assert.Equal(t, entryB.id, cover.SelectedSourceIdentifier)
}

func TestCatalogueSyncCoverFailedGenerationRecoversOnFallbackRetry(t *testing.T) {
	f := newCoverFixture(t)
	second, err := f.store.CreateOpdsConnection(f.ctx, f.ownerID, domain.OpdsConnection{Name: "Catalog B", URL: f.catalog.baseURL("/b")})
	require.NoError(t, err)
	entryA := coverEntry{id: "entry-a", title: "Retry Book", imageHref: "/a/image/a.png"}
	entryB := coverEntry{id: "entry-b", title: entryA.title, imageHref: "/b/image/b.png"}
	f.catalog.setImage(entryA.imageHref, coverImage{body: []byte("not an image"), contentType: "image/png"})
	f.catalog.setEntries("/a", entryA)
	require.NoError(t, f.sync(f.connection.ID))
	book := f.bookByTitle(entryA.title)
	_, err = f.store.Pool().Exec(f.ctx, `INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value) VALUES($1,$2,$3,$4,$5,$6)`,
		f.ownerID, book.ID, second.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, entryB.id)
	require.NoError(t, err)
	f.catalog.setImage(entryB.imageHref, coverImage{body: []byte("not an image either"), contentType: "image/png"})
	f.catalog.setEntries("/b", entryB)
	require.NoError(t, f.sync(second.ID))

	var first, fallback CoverArgs
	for _, candidate := range f.pendingCoverArgs() {
		if candidate.BookID != book.ID {
			continue
		}
		switch candidate.SourceIdentifier {
		case entryA.id:
			first = candidate
		case entryB.id:
			fallback = candidate
		}
	}
	require.NotEmpty(t, first.BookID)
	require.NotEmpty(t, fallback.BookID)
	require.Error(t, f.runCoverJob(first))
	require.Error(t, f.runCoverJob(fallback))
	assert.Equal(t, domain.BookCoverUnavailable, f.coverState(book.ID).State)

	// Retrying the unchanged fallback job after validation succeeds recovers
	// the Book without a new advertisement generation.
	f.catalog.setImage(entryB.imageHref, coverImage{body: testPNG(t, 220), contentType: "image/png"})
	require.NoError(t, f.runCoverJob(fallback))
	cover := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.Equal(t, second.ID, cover.SelectedConnectionID)
	assert.Equal(t, entryB.id, cover.SelectedSourceIdentifier)
}

func TestCatalogueSyncCoverMediaTypeMismatchPreservesReplacement(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Book One", imageHref: "/a/image/one.png"}
	book, retained := f.establishCover(t, entry, coverImage{body: testPNG(t, 10), contentType: "image/png"})

	// Both declarations are supplied, but the response disagrees with the
	// selected OPDS source and decoded PNG. The retained image remains visible.
	entry.imageType = "image/png"
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 200), contentType: "image/jpeg"})
	f.catalog.setEntries("/a", entry)
	require.NoError(t, f.sync(f.connection.ID))
	args := f.pendingCoverArgs()
	require.Len(t, args, 2)
	var currentArgs CoverArgs
	for _, candidate := range args {
		if candidate.AdvertisedAt.After(currentArgs.AdvertisedAt) {
			currentArgs = candidate
		}
	}
	require.Error(t, f.runCoverJob(currentArgs))

	cover := f.coverState(book.ID)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.NotEmpty(t, cover.FailureReason)
	preserved, err := f.store.GetBookCoverResource(f.ctx, f.ownerID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, retained, preserved.Bytes)
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
	failedArgs := f.pendingCoverArgs()
	require.Len(t, failedArgs, 2)
	var currentArgs CoverArgs
	for _, candidate := range failedArgs {
		if candidate.AdvertisedAt.After(currentArgs.AdvertisedAt) {
			currentArgs = candidate
		}
	}
	require.Error(t, f.runCoverJob(currentArgs))
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

func TestCatalogueSyncCoverExplicitAbsenceRetiresCurrentGenerationCandidate(t *testing.T) {
	prepare := func(t *testing.T, f *coverFixture, title, firstID, secondID string) (domain.Book, CoverArgs, CoverArgs) {
		t.Helper()
		firstEntry := coverEntry{id: firstID, title: title, imageHref: "/a/image/" + firstID + ".png"}
		secondEntry := coverEntry{id: secondID, title: title, imageHref: "/b/image/" + secondID + ".png"}
		f.catalog.setImage(firstEntry.imageHref, coverImage{body: testPNG(t, 10), contentType: "image/png"})
		f.catalog.setImage(secondEntry.imageHref, coverImage{body: testPNG(t, 20), contentType: "image/png"})
		f.catalog.setEntries("/a", firstEntry)
		require.NoError(t, f.sync(f.connection.ID))
		book := f.bookByTitle(title)
		second, err := f.store.CreateOpdsConnection(f.ctx, f.ownerID, domain.OpdsConnection{Name: "Catalog B", URL: f.catalog.baseURL("/b")})
		require.NoError(t, err)
		_, err = f.store.Pool().Exec(f.ctx, `INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value) VALUES($1,$2,$3,$4,$5,$6)`,
			f.ownerID, book.ID, second.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, secondID)
		require.NoError(t, err)
		f.catalog.setEntries("/b", secondEntry)
		require.NoError(t, f.sync(second.ID))
		args := f.pendingCoverArgs()
		require.Len(t, args, 2)
		var firstArgs, secondArgs CoverArgs
		for _, candidate := range args {
			switch candidate.SourceIdentifier {
			case firstID:
				firstArgs = candidate
			case secondID:
				secondArgs = candidate
			}
		}
		require.NotEmpty(t, firstArgs.BookID)
		require.NotEmpty(t, secondArgs.BookID)
		return book, firstArgs, secondArgs
	}

	t.Run("absent provisional keeps fallback live", func(t *testing.T) {
		f := newCoverFixture(t)
		book, firstArgs, fallbackArgs := prepare(t, f, "Absent provisional", "entry-a", "entry-b")
		f.catalog.setEntries("/a", coverEntry{id: "entry-a", title: "Absent provisional"})
		require.NoError(t, f.sync(f.connection.ID))
		pending := f.coverState(book.ID)
		assert.Equal(t, domain.BookCoverPending, pending.State)
		assert.True(t, firstArgs.AdvertisedAt.Equal(pending.AdvertisedAt))
		// The retired worker is harmless, while the already-advertised fallback
		// can complete without a second sync of its Catalog.
		require.NoError(t, f.runCoverJob(firstArgs))
		assert.Equal(t, domain.BookCoverPending, f.coverState(book.ID).State)
		require.NoError(t, f.runCoverJob(fallbackArgs))
		available := f.coverState(book.ID)
		assert.Equal(t, domain.BookCoverAvailable, available.State)
		assert.Equal(t, fallbackArgs.ConnectionID, available.SelectedConnectionID)
	})

	t.Run("absent fallback settles after final failure", func(t *testing.T) {
		f := newCoverFixture(t)
		book, firstArgs, fallbackArgs := prepare(t, f, "Absent fallback", "entry-c", "entry-d")
		f.catalog.setEntries("/b", coverEntry{id: "entry-d", title: "Absent fallback"})
		require.NoError(t, f.sync(fallbackArgs.ConnectionID))
		f.catalog.setImage("/a/image/entry-c.png", coverImage{body: []byte("not an image"), contentType: "image/png"})
		require.NoError(t, f.sync(f.connection.ID))
		require.Error(t, f.runCoverJob(firstArgs))
		failed := f.coverState(book.ID)
		assert.Equal(t, domain.BookCoverUnavailable, failed.State)
		assert.NotEmpty(t, failed.FailureReason)
		// A stale completion from the retired fallback cannot change the settled
		// state or its diagnostic.
		require.NoError(t, f.runCoverJob(fallbackArgs))
		stillFailed := f.coverState(book.ID)
		assert.Equal(t, domain.BookCoverUnavailable, stillFailed.State)
		assert.Equal(t, failed.FailureReason, stillFailed.FailureReason)
	})
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
