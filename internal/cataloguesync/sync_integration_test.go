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
)

type fakeCapabilities struct{ value analyzer.Capabilities }

func (f fakeCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return f.value, nil
}

type fakeReader struct {
	feeds   map[string]opds.Feed
	visited []string
	err     error
}

func (f *fakeReader) Languages(context.Context, string, string) (opds.Feed, error) {
	if f.err != nil {
		return opds.Feed{}, f.err
	}
	return opds.Feed{Entries: []opds.Entry{
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/7"}}},
		{Title: "English", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/8"}}},
		{Title: "Italian", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/9"}}},
		{Title: "French", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/10"}}},
	}}, nil
}

func (f *fakeReader) BrowseLanguage(_ context.Context, _ string, _ string, id string) (opds.Feed, error) {
	if f.err != nil {
		return opds.Feed{}, f.err
	}
	f.visited = append(f.visited, id)
	return f.feeds[id], nil
}

func testEntry(id, title string) opds.Entry {
	return opds.Entry{ID: id, Title: title, Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/opds/epub/" + id}}}
}

func TestSyncWorkerIdempotentMetadataOnlyAndOwnerScoped(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "sync-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "sync-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "en", "English"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "it", "Italian"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "fr", "French"); err != nil {
		t.Fatal(err)
	}
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Alice catalog", URL: "https://catalog.example/opds", Password: "catalog-secret"})
	if err != nil {
		t.Fatal(err)
	}
	reader := &fakeReader{feeds: map[string]opds.Feed{
		"7":  {Entries: []opds.Entry{testEntry("entry-1", "First title")}},
		"8":  {Entries: []opds.Entry{testEntry("english-entry", "Do not sync")}},
		"9":  {Entries: []opds.Entry{testEntry("italian-entry", "Not ready")}},
		"10": {Entries: []opds.Entry{testEntry("french-entry", "French title")}},
	}}
	worker := &Worker{Store: store, Reader: reader, Capabilities: fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}, {Language: "en", DisplayName: "English", Ready: true}, {Language: "it", DisplayName: "Italian", Ready: false}, {Language: "fr", DisplayName: "French", Ready: true}}}}}
	job := &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: alice.ID, ConnectionID: connection.ID}}
	if err = worker.Work(ctx, job); err != nil {
		t.Fatal(err)
	}
	books, err := store.ListMyBooks(ctx, alice.ID)
	if err != nil || len(books) != 2 {
		t.Fatalf("first sync books=%+v err=%v", books, err)
	}
	tags := make(map[string]string, len(books))
	for _, book := range books {
		tags[book.Title] = book.LanguageTag
		if book.LanguageState != domain.LanguageChosen || book.MetadataProvenance != domain.MetadataProvenanceCatalogueSync {
			t.Fatalf("first sync book=%+v", book)
		}
	}
	if tags["First title"] != "de" || tags["French title"] != "fr" {
		t.Fatalf("first sync language tags=%v", tags)
	}
	if len(reader.visited) != 2 || reader.visited[0] != "7" || reader.visited[1] != "10" {
		t.Fatalf("visited catalog language IDs=%v", reader.visited)
	}
	status, err := store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
	if err != nil || status.State != domain.CatalogueSyncSynced || status.LastSyncedAt == nil || status.LastError != "" || status.LastUpsertedCount != 2 {
		t.Fatalf("sync status=%+v err=%v", status, err)
	}
	if err = worker.Work(ctx, job); err != nil {
		t.Fatal(err)
	}
	status, err = store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
	if err != nil || status.LastUpsertedCount != 0 {
		t.Fatalf("unchanged rerun sync status=%+v err=%v", status, err)
	}
	reader.feeds["7"] = opds.Feed{Entries: []opds.Entry{testEntry("entry-1", "Updated title")}}
	if err = worker.Work(ctx, job); err != nil {
		t.Fatal(err)
	}
	books, err = store.ListMyBooks(ctx, alice.ID)
	if err != nil || len(books) != 2 {
		t.Fatalf("rerun books=%+v err=%v", books, err)
	}
	updatedTitles := make(map[string]string, len(books))
	for _, book := range books {
		updatedTitles[book.Title] = book.LanguageTag
	}
	if updatedTitles["Updated title"] != "de" || updatedTitles["French title"] != "fr" {
		t.Fatalf("rerun language tags=%v", updatedTitles)
	}
	status, err = store.GetCatalogueSyncStatus(ctx, alice.ID, connection.ID)
	if err != nil || status.LastUpsertedCount != 1 {
		t.Fatalf("updated rerun sync status=%+v err=%v", status, err)
	}
	var aliases, memberships int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1`, alice.ID).Scan(&aliases); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1`, alice.ID).Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if aliases != 2 || memberships != 2 {
		t.Fatalf("rerun aliases=%d memberships=%d", aliases, memberships)
	}
	reader.feeds["7"] = opds.Feed{}
	if err = worker.Work(ctx, job); err != nil {
		t.Fatal(err)
	}
	books, err = store.ListMyBooks(ctx, alice.ID)
	if err != nil || len(books) != 2 {
		t.Fatalf("upstream removal changed My Books=%+v err=%v", books, err)
	}
	if err = worker.Work(ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: bob.ID, ConnectionID: connection.ID}}); err != nil {
		t.Fatal(err)
	}
	if bobBooks, listErr := store.ListMyBooks(ctx, bob.ID); listErr != nil || len(bobBooks) != 0 {
		t.Fatalf("cross-owner books=%+v err=%v", bobBooks, listErr)
	}
	if _, err = store.GetCatalogueSyncStatus(ctx, bob.ID, connection.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("cross-owner status err=%v", err)
	}
	if _, err = store.GetOpdsConnection(ctx, bob.ID, connection.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("cross-owner connection err=%v", err)
	}
	if got, err := store.GetOpdsConnection(ctx, alice.ID, connection.ID); err != nil || got.Password != "catalog-secret" {
		t.Fatalf("credential load got=%+v err=%v", got, err)
	}
}

func TestSyncWorkerSafeFailurePreservesSecret(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "sync-failure", false)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Private catalog", URL: "https://catalog.example/opds", Password: "super-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Store: store, Reader: &fakeReader{err: errors.New("opds: HTTP 401 Unauthorized: super-secret")}, Capabilities: fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}}
	job := &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}
	if err = worker.Work(ctx, job); err == nil || err.Error() != "authentication failed for connection Private catalog" {
		t.Fatalf("safe worker error=%v", err)
	}
	status, err := store.GetCatalogueSyncStatus(ctx, owner.ID, connection.ID)
	if err != nil || status.LastError != "authentication failed for connection Private catalog" || status.LastError == "super-secret" {
		t.Fatalf("safe failure status=%+v err=%v", status, err)
	}
}

func TestListCatalogueSyncStatusesReconcilesStaleDurableSyncing(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = analysis.MigrateRiver(ctx, store.Pool()); err != nil {
		t.Fatal(err)
	}
	owner, err := store.CreateUser(ctx, "sync-status-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Status catalog", URL: "https://catalog.example/opds"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner.ID, ConnectionID: connection.ID, State: domain.CatalogueSyncSyncing}); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil, nil, nil)
	statuses, err := service.ListCatalogueSyncStatuses(ctx, owner.ID)
	if err != nil || len(statuses) != 1 || statuses[0].State != domain.CatalogueSyncFailed {
		t.Fatalf("stale status=%+v err=%v", statuses, err)
	}

	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	service.client = client
	first, err := service.Enqueue(ctx, owner.ID, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Enqueue(ctx, owner.ID, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("duplicate enqueue handles differ: first=%+v second=%+v", first, second)
	}
	var liveJobs int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'connection_id'=$3 AND state::text=ANY($4::text[])`, Kind, owner.ID, connection.ID, liveRiverStates()).Scan(&liveJobs); err != nil {
		t.Fatal(err)
	}
	if liveJobs != 1 {
		t.Fatalf("duplicate enqueue created %d live jobs", liveJobs)
	}
	if statuses, err = service.ListCatalogueSyncStatuses(ctx, owner.ID); err != nil || len(statuses) != 1 || statuses[0].State != domain.CatalogueSyncSyncing {
		t.Fatalf("live status=%+v err=%v", statuses, err)
	}
	if _, err = client.JobCancel(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.List(ctx, owner.ID); err != nil {
		t.Fatal(err)
	}
	status, err := store.GetCatalogueSyncStatus(ctx, owner.ID, connection.ID)
	if err != nil || status.State != domain.CatalogueSyncFailed {
		t.Fatalf("reconciled durable status=%+v err=%v", status, err)
	}
}
