//go:build integration

package persistence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFinishCurrentReadingPropagatesPersistenceFailures drops the table behind
// one step so a database failure reaches the caller instead of being read as an
// unknown reading.
func TestFinishCurrentReadingPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("the current reading lookup fails", func(t *testing.T) {
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "finish-lookup-failure", false)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `DROP TABLE primary_goals CASCADE`)
		require.NoError(t, err)

		_, err = store.FinishCurrentReading(ctx, owner.ID, "de")
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
	})

	t.Run("the completion write fails", func(t *testing.T) {
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "finish-completion-failure", false)
		require.NoError(t, err)
		book, source := createReadingFixture(t, ctx, store, owner.ID, "finish-completion")
		makeAnalyzedToReadBook(t, ctx, store, book, source)
		_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `DROP TABLE reading_history CASCADE`)
		require.NoError(t, err)

		_, err = store.FinishCurrentReading(ctx, owner.ID, "de")
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
		current, err := store.GetCurrentReading(ctx, owner.ID, "de")
		require.NoError(t, err)
		assert.Equal(t, book.ID, current.BookID, "a failed Finish leaves the reading current")
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

// TestUpdateCatalogueBookMetadataPropagatesPersistenceFailures breaks each
// read and write the metadata update performs, so a database failure reaches the
// caller instead of being read as a successful sync.
func TestUpdateCatalogueBookMetadataPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("the alias lookup fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "reconcile-alias-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		_, err := seedCatalogueBook(t, ctx, store, owner, connection.ID, "entry", "Title", "", "de")
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `DROP TABLE book_aliases CASCADE`)
		require.NoError(t, err)

		_, err = store.UpdateCatalogueBookMetadata(ctx, owner, connection.ID, "entry", "Retitled", "")
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrAliasConflict)
	})

	t.Run("the metadata update fails", func(t *testing.T) {
		store, owner := newFailureStore(t, ctx, "reconcile-update-failure-owner")
		connection := addFailureConnection(t, ctx, store, owner, "Failure")
		_, err := seedCatalogueBook(t, ctx, store, owner, connection.ID, "entry", "Title", "", "de")
		require.NoError(t, err)
		raiseOnWrite(t, ctx, store, "books", "UPDATE", "")

		_, err = store.UpdateCatalogueBookMetadata(ctx, owner, connection.ID, "entry", "Retitled", "")
		require.Error(t, err)
	})
}
