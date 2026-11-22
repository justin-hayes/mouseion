//go:build integration

package cataloguesync

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCapabilities struct{ value analyzer.Capabilities }

func (f fakeCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return f.value, nil
}

type fakeReader struct {
	feeds                map[string]opds.Feed
	connectionFeedTitles map[string]string
	visited              []string
	err                  error
}

func (f *fakeReader) Languages(context.Context, string, string) (opds.Feed, error) {
	if f.err != nil {
		return opds.Feed{}, f.err
	}
	return opds.Feed{Entries: []opds.Entry{
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/7"}}},
		{Title: "English", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/8"}}},
		{Title: "Italian", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/9"}}},
		{Title: "Greek", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/11"}}},
		{Title: "French", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/10"}}},
	}}, nil
}

func (f *fakeReader) BrowseLanguage(_ context.Context, _ string, connection, id string) (opds.Feed, error) {
	if f.err != nil {
		return opds.Feed{}, f.err
	}
	f.visited = append(f.visited, id)
	if title, ok := f.connectionFeedTitles[connection]; ok {
		return opds.Feed{Entries: []opds.Entry{testEntry("same-entry", title)}}, nil
	}
	return f.feeds[id], nil
}

func testEntry(id, title string) opds.Entry {
	return opds.Entry{ID: id, Title: title, Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/opds/epub/" + id}}}
}

func testEntryWithAuthor(id, title, author string) opds.Entry {
	entry := testEntry(id, title)
	entry.Author = author
	return entry
}

func TestSyncWorkerCreatesNoBooksAndUpdatesOnlyLinkedOnes(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync store", store.Close)
	alice, err := store.CreateUser(ctx, "sync-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "sync-bob", false)
	require.NoError(t, err)
	for _, language := range []struct{ code, name string }{{"de", "German"}, {"en", "English"}, {"it", "Italian"}, {"el", "Greek"}, {"fr", "French"}} {
		_, err = store.PutSupportedLanguage(ctx, language.code, language.name)
		require.NoError(t, err)
	}
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Alice catalog", URL: "https://catalog.example/opds", Password: "catalog-secret"})
	require.NoError(t, err)
	reader := &fakeReader{feeds: map[string]opds.Feed{
		"7":  {Entries: []opds.Entry{testEntryWithAuthor("entry-1", "First title", "First author")}},
		"8":  {Entries: []opds.Entry{testEntry("english-entry", "Do not sync")}},
		"9":  {Entries: []opds.Entry{testEntry("italian-entry", "Not ready")}},
		"11": {Entries: []opds.Entry{testEntry("greek-entry", "Greek title")}},
		"10": {Entries: []opds.Entry{testEntry("french-entry", "French title")}},
	}}
	worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}, {Language: "en", DisplayName: "English", Ready: true}, {Language: "it", DisplayName: "Italian", Ready: false}, {Language: "el", DisplayName: "Greek", Ready: true}, {Language: "fr", DisplayName: "French", Ready: true}}}}}
	job := &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: alice.ID, ConnectionID: connection.ID}}

	require.NoError(t, worker.Work(ctx, job))

	t.Run("fresh sync leaves My Books empty", func(t *testing.T) {
		books, err := store.ListMyBooks(ctx, alice.ID)
		require.NoError(t, err)
		require.Empty(t, books, "sync created Books from the catalog")
	})
	t.Run("fresh sync maintains the Catalog index", func(t *testing.T) {
		entries, err := store.ListCatalogueEntries(ctx, alice.ID, connection.ID)
		require.NoError(t, err)
		require.Len(t, entries, 3, "fresh sync did not maintain the Catalog index")
	})
	t.Run("fresh sync writes no membership, disposition, or alias", func(t *testing.T) {
		for _, table := range []string{"book_membership", "book_dispositions", "book_aliases"} {
			var count int
			err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE owner_id=$1`, alice.ID).Scan(&count)
			require.NoError(t, err)
			require.Zero(t, count, "fresh sync wrote %s rows", table)
		}
	})
	t.Run("fresh sync reports no Book updates", func(t *testing.T) {
		status, err := store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
		require.NoError(t, err)
		require.Equal(t, domain.CatalogueSyncSynced, status.State)
		require.NotNil(t, status.LastSyncedAt)
		require.Empty(t, status.LastError)
		require.Zero(t, status.LastUpsertedCount, "fresh sync reported Book updates")
	})

	// The learner chooses one catalog entry. Only then does a Book exist.
	added, err := store.AddCatalogueEntry(ctx, alice.ID, connection.ID, "entry-1")
	require.NoError(t, err)
	require.True(t, added.Created)
	bookID := added.Book.ID
	require.Equal(t, domain.BookDispositionToRead, mustCatalogueDisposition(t, store, alice.ID, bookID))

	// An unchanged resync is idempotent and changes no Book.
	require.NoError(t, worker.Work(ctx, job))
	t.Run("unchanged resync reports no updates and keeps metadata", func(t *testing.T) {
		status, err := store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
		require.NoError(t, err)
		require.Zero(t, status.LastUpsertedCount)
		unchanged, err := store.GetBook(ctx, alice.ID, bookID)
		require.NoError(t, err)
		require.Equal(t, "First title", unchanged.Title)
	})

	// An upstream metadata change reaches the linked Book without touching its
	// language, membership or disposition.
	reader.feeds["7"] = opds.Feed{Entries: []opds.Entry{testEntryWithAuthor("entry-1", "Updated title", "Updated author")}}
	require.NoError(t, worker.Work(ctx, job))
	t.Run("upstream metadata updates the linked Book", func(t *testing.T) {
		updated, err := store.GetBook(ctx, alice.ID, bookID)
		require.NoError(t, err)
		require.Equal(t, "Updated title", updated.Title)
		require.Equal(t, "Updated author", updated.Author)
		status, err := store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
		require.NoError(t, err)
		require.Equal(t, 1, status.LastUpsertedCount)
	})
	t.Run("sync leaves language and disposition as Add left them", func(t *testing.T) {
		updated, err := store.GetBook(ctx, alice.ID, bookID)
		require.NoError(t, err)
		require.Equal(t, "de", updated.LanguageTag)
		require.Equal(t, domain.BookDispositionToRead, mustCatalogueDisposition(t, store, alice.ID, bookID))
	})
	t.Run("metadata sync starts no analysis", func(t *testing.T) {
		for _, table := range []string{"analysis_runs", "analysis_jobs"} {
			var count int
			err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE owner_id=$1`, alice.ID).Scan(&count)
			require.NoError(t, err)
			require.Zero(t, count, "metadata sync wrote %s rows", table)
		}
	})

	// A Book removed from My Books is not put back by sync, even when its entry
	// still exists and changes upstream.
	require.NoError(t, store.RemoveBookFromMyBooks(ctx, alice.ID, bookID))
	reader.feeds["7"] = opds.Feed{Entries: []opds.Entry{testEntryWithAuthor("entry-1", "After removal", "After author")}}
	require.NoError(t, worker.Work(ctx, job))
	t.Run("removed Book is not restored", func(t *testing.T) {
		books, err := store.ListMyBooks(ctx, alice.ID)
		require.NoError(t, err)
		require.Empty(t, books, "sync restored a Book that left My Books")
		_, err = store.GetBookDetail(ctx, alice.ID, bookID)
		require.ErrorIs(t, err, persistence.ErrNotFound)
	})

	// A complete run deletes index entries it no longer sees; the chosen Book
	// and its metadata remain.
	reader.feeds["7"] = opds.Feed{}
	require.NoError(t, worker.Work(ctx, job))
	t.Run("complete run deletes index entries it no longer sees", func(t *testing.T) {
		entries, err := store.ListCatalogueEntries(ctx, alice.ID, connection.ID)
		require.NoError(t, err)
		require.Len(t, entries, 2)
	})

	// Another owner cannot read Alice's status or connection.
	require.NoError(t, worker.Work(ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: bob.ID, ConnectionID: connection.ID}}))
	t.Run("another owner's sync creates no Books", func(t *testing.T) {
		bobBooks, err := store.ListMyBooks(ctx, bob.ID)
		require.NoError(t, err)
		require.Empty(t, bobBooks)
	})
	t.Run("bob cannot read Alice's sync status", func(t *testing.T) {
		_, err := store.GetCatalogueSyncStatus(ctx, bob.ID, connection.ID)
		require.ErrorIs(t, err, persistence.ErrNotFound)
	})
	t.Run("bob cannot read Alice's connection", func(t *testing.T) {
		_, err := store.GetOpdsConnection(ctx, bob.ID, connection.ID)
		require.ErrorIs(t, err, persistence.ErrNotFound)
	})
	t.Run("Alice's connection retains its credential", func(t *testing.T) {
		got, err := store.GetOpdsConnection(ctx, alice.ID, connection.ID)
		require.NoError(t, err)
		require.Equal(t, "catalog-secret", got.Password)
	})
}

func mustCatalogueDisposition(t *testing.T, store *persistence.PostgresStore, owner, bookID string) domain.BookDisposition {
	t.Helper()
	disposition, err := store.GetBookDisposition(context.Background(), owner, bookID)
	require.NoError(t, err)
	return disposition
}

func TestSyncWorkerSameEntryIDAcrossConnectionsUpdatesDistinctBooks(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync store", store.Close)
	owner, err := store.CreateUser(ctx, "sync-collision-owner", false)
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "de", "German")
	require.NoError(t, err)
	first, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "First catalog", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second catalog", URL: "https://second.example/opds"})
	require.NoError(t, err)
	reader := &fakeReader{feeds: map[string]opds.Feed{
		"7": {Entries: []opds.Entry{testEntry("same-entry", "First catalog title")}},
	}, connectionFeedTitles: map[string]string{first.ID: "First catalog title", second.ID: "Second catalog title"}}
	capabilities := fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
	worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
	connections := []domain.OpdsConnection{first, second}
	for _, connection := range connections {
		require.NoError(t, worker.Work(ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}))
	}
	bookIDs := make(map[string]string, len(connections))
	for _, connection := range connections {
		added, addErr := store.AddCatalogueEntry(ctx, owner.ID, connection.ID, "same-entry")
		require.NoError(t, addErr)
		bookIDs[connection.ID] = added.Book.ID
	}
	require.NotEqual(t, bookIDs[first.ID], bookIDs[second.ID], "same entry ID reused one Book across connections")

	// Each connection's next sync updates only its own linked Book.
	reader.connectionFeedTitles = map[string]string{first.ID: "First updated", second.ID: "Second updated"}
	for _, connection := range connections {
		require.NoError(t, worker.Work(ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}))
	}
	firstBook, err := store.GetBook(ctx, owner.ID, bookIDs[first.ID])
	require.NoError(t, err)
	secondBook, err := store.GetBook(ctx, owner.ID, bookIDs[second.ID])
	require.NoError(t, err)
	assert.Equal(t, "First updated", firstBook.Title)
	assert.Equal(t, "Second updated", secondBook.Title)
	books, err := store.ListMyBooks(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, books, 2)
	var aliasCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND value='same-entry'`, owner.ID).Scan(&aliasCount)
	require.NoError(t, err)
	assert.Equal(t, 2, aliasCount, "same entry ID aliases=%d, want 2", aliasCount)
}

func TestSyncWorkerLeavesBookLanguageAndCurrentReadingUnchanged(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "catalogue-sync-language-correction-secret")
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync store", store.Close)
	owner, err := store.CreateUser(ctx, "sync-language-correction", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Language catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)
	reader := &fakeReader{feeds: map[string]opds.Feed{
		"7": {Entries: []opds.Entry{testEntry("language-entry", "German title")}},
		"9": {},
	}}
	capabilities := fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", DisplayName: "German", Ready: true},
		{Language: "it", DisplayName: "Italian", Ready: true},
	}}}
	worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
	job := &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}
	require.NoError(t, worker.Work(ctx, job))
	added, err := store.AddCatalogueEntry(ctx, owner.ID, connection.ID, "language-entry")
	require.NoError(t, err)
	bookID := added.Book.ID
	require.Equal(t, "de", added.Book.LanguageTag)
	// Seed the active slot directly so this worker-level test focuses on the
	// catalogue reconciliation path.
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, owner.ID, bookID)
	require.NoError(t, err)

	reader.feeds["7"] = opds.Feed{}
	reader.feeds["9"] = opds.Feed{Entries: []opds.Entry{testEntry("language-entry", "Italian title")}}
	require.NoError(t, worker.Work(ctx, job))
	unchanged, err := store.GetBook(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, "de", unchanged.LanguageTag, "sync changed the Book's language")
	assert.Equal(t, domain.BookDispositionToRead, mustCatalogueDisposition(t, store, owner.ID, bookID))
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, bookID, current.BookID, "sync ended the current reading")
	italian, err := store.GetCurrentReading(ctx, owner.ID, "it")
	require.NoError(t, err)
	assert.Empty(t, italian.BookID)
	indexed, err := store.GetCatalogueEntry(ctx, owner.ID, connection.ID, "language-entry")
	require.NoError(t, err)
	assert.Equal(t, "it", indexed.LanguageTag, "the Catalog index did not follow the entry")
}

func TestSyncWorkerSafeFailurePreservesSecret(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync store", store.Close)
	owner, err := store.CreateUser(ctx, "sync-failure", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Private catalog", URL: "https://catalog.example/opds", Password: "super-secret"})
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "de", "German")
	require.NoError(t, err)
	worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: &fakeReader{err: errors.New("opds: HTTP 401 Unauthorized: super-secret")}, Capabilities: fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}}
	job := &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}
	err = worker.Work(ctx, job)
	require.Error(t, err)
	assert.Equal(t, "authentication failed for connection Private catalog", err.Error())
	status, err := store.GetCatalogueSyncStatus(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, "authentication failed for connection Private catalog", status.LastError)
	assert.NotEqual(t, "super-secret", status.LastError)
}

func TestListCatalogueSyncStatusesReconcilesStaleDurableSyncing(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync store", store.Close)
	require.NoError(t, migrateRiverForTest(store.Pool()))
	owner, err := store.CreateUser(ctx, "sync-status-owner", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Status catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)
	require.NoError(t, store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner.ID, ConnectionID: connection.ID, State: domain.CatalogueSyncSyncing}))
	service := NewService(StoreDependencies{Connections: store, Catalogue: store, Statuses: store, Pool: store.Pool()}, nil, nil, nil)
	statuses, err := service.ListCatalogueSyncStatuses(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, domain.CatalogueSyncFailed, statuses[0].State)

	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "catalogue sync client", client.Stop)
	service.client = client
	first, err := service.Enqueue(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	second, err := service.Enqueue(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, second, first, "duplicate enqueue handles differ")
	var liveJobs int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'connection_id'=$3 AND state::text=ANY($4::text[])`, Kind, owner.ID, connection.ID, liveRiverStates()).Scan(&liveJobs)
	require.NoError(t, err)
	assert.Equal(t, 1, liveJobs, "duplicate enqueue created %d live jobs", liveJobs)
	statuses, err = service.ListCatalogueSyncStatuses(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, domain.CatalogueSyncSyncing, statuses[0].State)
	_, err = client.JobCancel(ctx, first.ID)
	require.NoError(t, err)
	jobs, err := service.List(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "cancelled", jobs[0].State)
	assert.Equal(t, domain.JobLifecycleFailed, jobs[0].Lifecycle)
	assert.Equal(t, statusCancelledReason, jobs[0].Error)
	assert.Equal(t, connection.ID, jobs[0].ConnectionID)
	status, err := store.GetCatalogueSyncStatus(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.CatalogueSyncFailed, status.State)
}

func TestCatalogueSyncDetailUsesSharedLifecycleAndActions(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync store", store.Close)
	require.NoError(t, migrateRiverForTest(store.Pool()))
	owner, err := store.CreateUser(ctx, "sync-detail-owner", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Detail catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)

	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "catalogue sync client", client.Stop)
	service := NewService(StoreDependencies{Connections: store, Catalogue: store, Statuses: store, Pool: store.Pool()}, nil, nil, nil)
	service.client = client

	completed, err := client.Insert(ctx, SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}, &river.InsertOpts{Queue: Queue})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE river_job SET state='completed',finalized_at=now() WHERE id=$1`, completed.Job.ID)
	require.NoError(t, err)

	status, err := service.Get(ctx, owner.ID, completed.Job.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleCompleted, status.Lifecycle)
	assert.Equal(t, 100, status.Progress)

	unchanged, err := service.Cancel(ctx, owner.ID, completed.Job.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleCompleted, unchanged.Lifecycle)

	_, err = service.Retry(ctx, owner.ID, completed.Job.ID)
	require.ErrorContains(t, err, "not retryable")

	failed, err := client.Insert(ctx, SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}, &river.InsertOpts{Queue: Queue})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE river_job SET state='discarded',finalized_at=now(),errors=ARRAY[jsonb_build_object('error',$2::text,'attempt',1,'at',now())]::jsonb[] WHERE id=$1`, failed.Job.ID, "metadata sync failed")
	require.NoError(t, err)
	require.NoError(t, store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner.ID, ConnectionID: connection.ID, State: domain.CatalogueSyncFailed, LastError: "metadata sync failed"}))

	status, err = service.Get(ctx, owner.ID, failed.Job.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleFailed, status.Lifecycle)
	assert.Equal(t, "metadata sync failed", status.Error)
}

func TestListCatalogueSyncJobsKeepsEachJobsOutcome(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync store", store.Close)
	require.NoError(t, migrateRiverForTest(store.Pool()))
	owner, err := store.CreateUser(ctx, "sync-history-owner", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "History catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)

	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "catalogue sync client", client.Stop)
	args := SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}
	completed, err := client.Insert(ctx, args, &river.InsertOpts{Queue: Queue})
	require.NoError(t, err)
	failed, err := client.Insert(ctx, args, &river.InsertOpts{Queue: Queue})
	require.NoError(t, err)

	const sanitised = "authentication failed for connection History catalog"
	_, err = store.Pool().Exec(ctx, `UPDATE river_job SET state='completed',created_at=now()-interval '2 hours',finalized_at=now()-interval '2 hours' WHERE id=$1`, completed.Job.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE river_job SET state='discarded',created_at=now()-interval '1 hour',finalized_at=now()-interval '1 hour',errors=ARRAY[jsonb_build_object('error',$2::text,'attempt',1,'at',now())]::jsonb[] WHERE id=$1`, failed.Job.ID, sanitised)
	require.NoError(t, err)
	require.NoError(t, store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner.ID, ConnectionID: connection.ID, State: domain.CatalogueSyncFailed, LastError: sanitised}))

	service := NewService(StoreDependencies{Connections: store, Catalogue: store, Statuses: store, Pool: store.Pool()}, nil, nil, nil)
	jobs, err := service.List(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	assert.Equal(t, failed.Job.ID, jobs[0].ID)
	assert.Equal(t, domain.JobLifecycleFailed, jobs[0].Lifecycle)
	assert.Equal(t, sanitised, jobs[0].Error)
	assert.Equal(t, completed.Job.ID, jobs[1].ID)
	assert.Equal(t, domain.JobLifecycleCompleted, jobs[1].Lifecycle)
	assert.Empty(t, jobs[1].Error, "an older completed job must not inherit the connection's failed status")
}
