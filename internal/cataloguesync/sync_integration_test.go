//go:build integration

package cataloguesync

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
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
func (f *fakeReader) BrowseLanguageUnfiltered(ctx context.Context, owner, connection, id string) (opds.Feed, error) {
	return f.BrowseLanguage(ctx, owner, connection, id)
}

func testEntry(id, title string) opds.Entry {
	return opds.Entry{ID: id, Title: title, Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/opds/epub/" + id}}}
}

func testEntryWithAuthor(id, title, author string) opds.Entry {
	entry := testEntry(id, title)
	entry.Author = author
	return entry
}

func TestSyncWorkerIdempotentMetadataOnlyAndOwnerScoped(t *testing.T) {
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
	_, err = store.PutSupportedLanguage(ctx, "de", "German")
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "en", "English")
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "it", "Italian")
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "el", "Greek")
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "fr", "French")
	require.NoError(t, err)
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
	books, err := store.ListMyBooks(ctx, alice.ID)
	require.NoError(t, err)
	assert.Len(t, books, 3)
	tags := make(map[string]string, len(books))
	for _, book := range books {
		tags[book.Title] = book.LanguageTag
		assert.Equal(t, domain.LanguageChosen, book.LanguageState, "first sync book=%+v", book)
		assert.Equal(t, domain.MetadataProvenanceCatalogueSync, book.MetadataProvenance, "first sync book=%+v", book)
	}
	assert.Equal(t, "de", tags["First title"])
	for _, book := range books {
		if book.Title == "First title" {
			assert.Equal(t, "First author", book.Author)
		}
	}
	assert.Equal(t, "el", tags["Greek title"])
	assert.Equal(t, "fr", tags["French title"])
	var journeyBookID, retryBookID string
	for _, book := range books {
		if book.Title == "First title" {
			journeyBookID = book.ID
			assert.Equal(t, domain.BookDispositionInbox, mustCatalogueDisposition(t, store, alice.ID, book.ID), "first discovery did not enter Inbox")
		}
		if book.Title == "Greek title" {
			retryBookID = book.ID
			assert.Equal(t, domain.BookDispositionInbox, mustCatalogueDisposition(t, store, alice.ID, book.ID), "first discovery did not enter Inbox")
		}
	}
	require.NotEmpty(t, journeyBookID, "first synced book was not found for Journey lifecycle check")
	require.NotEmpty(t, retryBookID, "Greek synced book was not found for resync check")
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", journeyBookID, journey.Revision)
	require.NoError(t, err)
	require.Len(t, reader.visited, 3)
	assert.Equal(t, "7", reader.visited[0])
	assert.Equal(t, "11", reader.visited[1])
	assert.Equal(t, "10", reader.visited[2])
	status, err := store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.CatalogueSyncSynced, status.State)
	assert.NotNil(t, status.LastSyncedAt)
	assert.Equal(t, "", status.LastError)
	assert.Equal(t, 3, status.LastUpsertedCount)
	require.NoError(t, worker.Work(ctx, job))
	status, err = store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, status.LastUpsertedCount)
	assert.Equal(t, domain.BookDispositionInbox, mustCatalogueDisposition(t, store, alice.ID, retryBookID), "resync reset first-discovery disposition")
	reader.feeds["7"] = opds.Feed{Entries: []opds.Entry{testEntryWithAuthor("entry-1", "Updated title", "Updated author")}}
	require.NoError(t, worker.Work(ctx, job))
	books, err = store.ListMyBooks(ctx, alice.ID)
	require.NoError(t, err)
	assert.Len(t, books, 3)
	updatedTitles := make(map[string]string, len(books))
	for _, book := range books {
		updatedTitles[book.Title] = book.LanguageTag
	}
	assert.Equal(t, "de", updatedTitles["Updated title"])
	assert.Equal(t, "el", updatedTitles["Greek title"])
	assert.Equal(t, "fr", updatedTitles["French title"])
	for _, book := range books {
		if book.Title == "Updated title" {
			assert.Equal(t, "Updated author", book.Author)
		}
	}
	status, err = store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, status.LastUpsertedCount)
	assert.Equal(t, domain.BookDispositionToRead, mustCatalogueDisposition(t, store, alice.ID, journeyBookID), "metadata resync reset To Read")
	var analysisRuns, analysisJobs int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, alice.ID).Scan(&analysisRuns)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, alice.ID).Scan(&analysisJobs)
	require.NoError(t, err)
	assert.Equal(t, 0, analysisRuns)
	assert.Equal(t, 0, analysisJobs)
	var aliases, memberships int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1`, alice.ID).Scan(&aliases)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1`, alice.ID).Scan(&memberships)
	require.NoError(t, err)
	assert.Equal(t, 3, aliases)
	assert.Equal(t, 3, memberships)
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, alice.ID, "de", journeyBookID, journey.Revision)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, mustCatalogueDisposition(t, store, alice.ID, journeyBookID))
	_, err = store.ImportPreviouslyRead(ctx, alice.ID, journeyBookID)
	require.NoError(t, err)
	reader.feeds["7"] = opds.Feed{}
	require.NoError(t, worker.Work(ctx, job))
	books, err = store.ListMyBooks(ctx, alice.ID)
	require.NoError(t, err)
	assert.Len(t, books, 3)
	assert.Equal(t, domain.BookDispositionSetAside, mustCatalogueDisposition(t, store, alice.ID, journeyBookID), "upstream disappearance changed disposition")
	var historyCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, alice.ID, journeyBookID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 1, historyCount, "upstream disappearance erased reading history")
	reader.feeds["7"] = opds.Feed{Entries: []opds.Entry{testEntryWithAuthor("entry-1", "Reappeared title", "Reappeared author")}}
	require.NoError(t, worker.Work(ctx, job))
	require.NoError(t, worker.Work(ctx, job), "replayed catalogue discovery failed")
	books, err = store.ListMyBooks(ctx, alice.ID)
	require.NoError(t, err)
	assert.Len(t, books, 3, "reappearing entry created a duplicate Book")
	for _, book := range books {
		if book.ID == journeyBookID {
			assert.Equal(t, "Reappeared title", book.Title)
			assert.Equal(t, "Reappeared author", book.Author)
		}
	}
	assert.Equal(t, domain.BookDispositionSetAside, mustCatalogueDisposition(t, store, alice.ID, journeyBookID), "reappearing entry reset disposition")
	require.NoError(t, worker.Work(ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: bob.ID, ConnectionID: connection.ID}}))
	bobBooks, listErr := store.ListMyBooks(ctx, bob.ID)
	require.NoError(t, listErr)
	assert.Empty(t, bobBooks)
	_, err = store.GetCatalogueSyncStatus(ctx, bob.ID, connection.ID)
	assert.ErrorIs(t, err, persistence.ErrNotFound) //nolint:testifylint // Independent owner-isolation check; the following lookup is separate.
	_, err = store.GetOpdsConnection(ctx, bob.ID, connection.ID)
	assert.ErrorIs(t, err, persistence.ErrNotFound) //nolint:testifylint // Independent owner-isolation check; the later assertion verifies Alice's access.
	got, err := store.GetOpdsConnection(ctx, alice.ID, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, "catalog-secret", got.Password)
}

func mustCatalogueDisposition(t *testing.T, store *persistence.PostgresStore, owner, bookID string) domain.BookDisposition {
	t.Helper()
	disposition, err := store.GetBookDisposition(context.Background(), owner, bookID)
	require.NoError(t, err)
	return disposition
}

func TestSyncWorkerSameEntryIDAcrossConnectionsCreatesDistinctBooks(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync store", store.Close)
	owner, err := store.CreateUser(ctx, "sync-collision-owner", false)
	require.NoError(t, err)
	for _, language := range []string{"de"} {
		_, err = store.PutSupportedLanguage(ctx, language, "German")
		require.NoError(t, err)
	}
	first, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "First catalog", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second catalog", URL: "https://second.example/opds"})
	require.NoError(t, err)
	reader := &fakeReader{feeds: map[string]opds.Feed{
		"7": {Entries: []opds.Entry{testEntry("same-entry", "First catalog title")}},
	}, connectionFeedTitles: map[string]string{first.ID: "First catalog title", second.ID: "Second catalog title"}}
	capabilities := fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
	worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
	for _, connection := range []domain.OpdsConnection{first, second} {
		require.NoError(t, worker.Work(ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}))
	}
	books, err := store.ListMyBooks(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, books, 2, "same entry ID produced %d books, want 2: %+v", len(books), books)
	titles := map[string]bool{}
	for _, book := range books {
		titles[book.Title] = true
	}
	assert.True(t, titles["First catalog title"])
	assert.True(t, titles["Second catalog title"])
	var aliasCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND value='same-entry'`, owner.ID).Scan(&aliasCount)
	require.NoError(t, err)
	assert.Equal(t, 2, aliasCount, "same entry ID aliases=%d, want 2", aliasCount)
}

func TestSyncWorkerLanguageCorrectionEndsIncompatibleCurrentReading(t *testing.T) {
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
	books, err := store.ListMyBooks(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, books, 1)
	book := books[0]
	require.Equal(t, "de", book.LanguageTag)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	// Seed the active slot directly so this worker-level test focuses on the
	// catalogue reconciliation path; the lifecycle test verifies full snapshot
	// reservation and release behavior.
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, owner.ID, book.ID)
	require.NoError(t, err)

	reader.feeds["7"] = opds.Feed{}
	reader.feeds["9"] = opds.Feed{Entries: []opds.Entry{testEntry("language-entry", "Italian title")}}
	require.NoError(t, worker.Work(ctx, job))
	corrected, err := store.GetBook(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, "it", corrected.LanguageTag)
	assert.Equal(t, domain.BookDispositionToRead, mustCatalogueDisposition(t, store, owner.ID, book.ID))
	for _, language := range []string{"de", "it"} {
		current, getErr := store.GetCurrentReading(ctx, owner.ID, language)
		require.NoError(t, getErr)
		assert.Empty(t, current.BookID, "sync left the corrected Book in a %s current-reading role", language)
	}
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
	require.NoError(t, analysis.MigrateRiver(ctx, store.Pool()))
	owner, err := store.CreateUser(ctx, "sync-status-owner", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Status catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)
	require.NoError(t, store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner.ID, ConnectionID: connection.ID, State: domain.CatalogueSyncSyncing}))
	service := NewService(StoreDependencies{Connections: store, Catalogue: store, Aliases: store, Statuses: store, Pool: store.Pool()}, nil, nil, nil)
	statuses, err := service.ListCatalogueSyncStatuses(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, domain.CatalogueSyncFailed, statuses[0].State)

	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{})
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue sync client", func() error {
		return client.Stop(context.Background())
	})
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
	_, err = service.List(ctx, owner.ID)
	require.NoError(t, err)
	status, err := store.GetCatalogueSyncStatus(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.CatalogueSyncFailed, status.State)
}
