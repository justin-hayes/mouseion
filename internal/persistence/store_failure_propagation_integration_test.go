//go:build integration

package persistence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// missingBookID and missingSnapshotID are well-formed UUIDs that name nothing,
// so the commitment checks pass and the lookup underneath is exercised.
const (
	missingBookID     = "00000000-0000-0000-0000-000000000001"
	missingSnapshotID = "00000000-0000-0000-0000-000000000002"
)

// TestFinishCurrentReadingPropagatesPersistenceFailures drops the table behind
// one read so a database failure reaches the caller instead of being read as a
// stale or unknown reading.
func TestFinishCurrentReadingPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("the current reading lookup fails", func(t *testing.T) {
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "finish-lookup-failure", false)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `DROP TABLE primary_goals CASCADE`)
		require.NoError(t, err)

		_, err = store.FinishCurrentReading(ctx, owner.ID, "de", missingBookID, missingSnapshotID)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
		require.NotErrorIs(t, err, ErrCurrentReadingStale)
	})

	t.Run("the completion lookup fails", func(t *testing.T) {
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "finish-completion-failure", false)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `DROP TABLE reading_history CASCADE`)
		require.NoError(t, err)

		_, err = store.FinishCurrentReading(ctx, owner.ID, "de", missingBookID, missingSnapshotID)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
		require.NotErrorIs(t, err, ErrCurrentReadingStale)
	})

	t.Run("the snapshot identities lookup fails", func(t *testing.T) {
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "finish-snapshot-failure", false)
		require.NoError(t, err)
		book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "finish-snapshot")
		makeAnalyzedToReadBook(t, ctx, store, book, source)
		require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
		reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `DROP TABLE primary_goal_snapshot_vocabulary CASCADE`)
		require.NoError(t, err)

		_, err = store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
		require.NotErrorIs(t, err, ErrCurrentReadingStale)
	})
}

// TestResolveOrCreateBookForAcquisitionPropagatesPersistenceFailures drops the
// identity tables one at a time so a lookup failure reaches the caller instead
// of being read as an absent match.
func TestResolveOrCreateBookForAcquisitionPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T, statement string) error {
		t.Helper()
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "acquisition-lookup-failure", false)
		require.NoError(t, err)
		book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Selected", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, statement)
		require.NoError(t, err)

		_, err = store.ResolveOrCreateBookForAcquisitionForBook(ctx, owner.ID, book.ID, "lookup-failure-source", "de", "Selected")
		return err
	}

	t.Run("the linked source lookup fails", func(t *testing.T) {
		err := run(t, `DROP TABLE source_materials CASCADE`)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
		require.NotErrorIs(t, err, ErrSourceBookConflict)
		require.NotErrorIs(t, err, ErrAliasConflict)
	})

	t.Run("the unscoped alias lookup fails", func(t *testing.T) {
		err := run(t, `DROP TABLE book_aliases CASCADE`)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
		require.NotErrorIs(t, err, ErrSourceBookConflict)
		require.NotErrorIs(t, err, ErrAliasConflict)
	})
}

// TestRecordBookCoverAdvertisementPropagatesPersistenceFailures drops the
// connection table so the lock failure reaches the caller.
func TestRecordBookCoverAdvertisementPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "cover-lookup-failure", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Cover", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Cover catalog", URL: "https://cover.example/opds"})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DROP TABLE opds_connections CASCADE`)
	require.NoError(t, err)

	_, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, book.ID, connection.ID, "cover-source", true)
	require.Error(t, err)
}

// raiseOnWrite installs a trigger that raises for one write statement, so a
// failure reaches the caller instead of being read as a successful write.
func raiseOnWrite(t *testing.T, ctx context.Context, store *PostgresStore, table, event, when string) {
	t.Helper()
	name := "test_raise_" + table + "_" + strings.ReplaceAll(event, " ", "")
	statement := fmt.Sprintf(`
CREATE FUNCTION %[1]s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced %[1]s'; END; $$;
CREATE TRIGGER %[1]s BEFORE %[2]s ON %[3]s FOR EACH ROW %[4]s EXECUTE FUNCTION %[1]s()`, name, event, table, when)
	_, err := store.Pool().Exec(ctx, statement)
	require.NoError(t, err)
}

// newFailureStore opens a fresh store with one owner for a failure-injection
// subtest, so each subtest can break a different statement in isolation.
func newFailureStore(t *testing.T, ctx context.Context, ownerName string) (*PostgresStore, string) {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, ownerName, false)
	require.NoError(t, err)
	return store, owner.ID
}

// addFailureConnection adds one named OPDS connection to an owner.
func addFailureConnection(t *testing.T, ctx context.Context, store *PostgresStore, owner, name string) domain.OpdsConnection {
	t.Helper()
	connection, err := store.CreateOpdsConnection(ctx, owner, domain.OpdsConnection{Name: name, URL: "https://" + strings.ToLower(name) + ".example/opds"})
	require.NoError(t, err)
	return connection
}

// TestReconcileCatalogueEntryPropagatesPersistenceFailures breaks each write the
// metadata upsert performs, so a failure reaches the caller instead of being
// read as a successful sync.
func TestReconcileCatalogueEntryPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("the alias lookup fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "reconcile-alias-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		_, err := store.Pool().Exec(ctx, `DROP TABLE book_aliases CASCADE`)
		require.NoError(t, err)

		_, err = store.ReconcileCatalogueEntry(ctx, owner, connection.ID, "entry", "Title", "", "de")
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrAliasConflict)
	})

	t.Run("the book insert fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "reconcile-insert-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		raiseOnWrite(t, ctx, store, "books", "INSERT", "")

		_, err := store.ReconcileCatalogueEntry(ctx, owner, connection.ID, "entry", "Title", "", "de")
		require.Error(t, err)
	})

	t.Run("the metadata update fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "reconcile-update-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		_, err := store.ReconcileCatalogueEntry(ctx, owner, connection.ID, "entry", "Title", "", "de")
		require.NoError(t, err)
		raiseOnWrite(t, ctx, store, "books", "UPDATE", "")

		_, err = store.ReconcileCatalogueEntry(ctx, owner, connection.ID, "entry", "Retitled", "", "de")
		require.Error(t, err)
	})

	t.Run("the membership activation fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "reconcile-membership-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		raiseOnWrite(t, ctx, store, "book_membership", "INSERT OR UPDATE", "")

		_, err := store.ReconcileCatalogueEntry(ctx, owner, connection.ID, "entry", "Title", "", "de")
		require.Error(t, err)
	})

	t.Run("the inbox placement fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "reconcile-inbox-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		raiseOnWrite(t, ctx, store, "book_dispositions", "INSERT", "")

		_, err := store.ReconcileCatalogueEntry(ctx, owner, connection.ID, "entry", "Title", "", "de")
		require.Error(t, err)
	})

	t.Run("the catalogue alias insert fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "reconcile-alias-insert-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		raiseOnWrite(t, ctx, store, "book_aliases", "INSERT", "")

		_, err := store.ReconcileCatalogueEntry(ctx, owner, connection.ID, "entry", "Title", "", "de")
		require.Error(t, err)
	})
}

// TestBookCoverAdvertisementPropagatesWriteFailures breaks the cover writes so a
// failure reaches the caller instead of being read as a settled advertisement.
func TestBookCoverAdvertisementPropagatesWriteFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("the cover row read fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "cover-read-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Cover", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `DROP TABLE book_covers CASCADE`)
		require.NoError(t, err)

		_, _, err = store.RecordBookCoverAdvertisement(ctx, owner, book.ID, connection.ID, "cover-source", true)
		require.Error(t, err)
	})

	t.Run("the refresh write fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "cover-refresh-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		reconciled, err := store.ReconcileCatalogueEntry(ctx, owner, connection.ID, "entry", "Title", "", "de")
		require.NoError(t, err)
		_, advertisedAt, err := store.RecordBookCoverAdvertisement(ctx, owner, reconciled.Book.ID, connection.ID, "entry", true)
		require.NoError(t, err)
		require.NoError(t, store.SaveBookCover(ctx, owner, reconciled.Book.ID, connection.ID, "entry", advertisedAt, "image/png", 4, 6, "image", []byte("image")))
		raiseOnWrite(t, ctx, store, "book_covers", "UPDATE", "WHEN (OLD.state = 'available')")

		_, _, err = store.RecordBookCoverAdvertisement(ctx, owner, reconciled.Book.ID, connection.ID, "entry", true)
		require.Error(t, err)
	})

	t.Run("the demotion write fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "cover-demote-failure-owner")
		first := addFailureConnection(t, ctx, store, owner, "First")
		second := addFailureConnection(t, ctx, store, owner, "Second")
		bookID, advertisedAt := advertiseBookCoverCandidates(t, ctx, store, owner, first, second, "Demotion", "demote-1", "demote-2")
		require.NoError(t, store.SaveBookCover(ctx, owner, bookID, first.ID, "demote-1", advertisedAt, "image/png", 4, 6, "image", []byte("image")))
		raiseOnWrite(t, ctx, store, "book_covers", "UPDATE", "WHEN (NEW.state = 'pending')")

		_, _, err := store.RecordBookCoverAdvertisement(ctx, owner, bookID, first.ID, "demote-1", false)
		require.Error(t, err)
	})

	t.Run("the candidate failure write fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "cover-candidate-failure-owner")
		first := addFailureConnection(t, ctx, store, owner, "First")
		second := addFailureConnection(t, ctx, store, owner, "Second")
		bookID, advertisedAt := advertiseBookCoverCandidates(t, ctx, store, owner, first, second, "Candidate failure", "fail-1", "fail-2")
		raiseOnWrite(t, ctx, store, "book_cover_candidates", "UPDATE", "")

		err := store.MarkBookCoverUnavailable(ctx, owner, bookID, second.ID, "fail-2", advertisedAt, "forced failure")
		require.Error(t, err)
	})
}
