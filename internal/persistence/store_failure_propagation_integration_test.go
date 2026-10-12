//go:build integration

package persistence

import (
	"context"
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
