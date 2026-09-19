//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadingHistoryBackfillPreservesOwnerLanguageAndKnownState(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	// Return to the pre-feature schema so this test exercises the shipped
	// backfill and cleanup migrations rather than reproducing their SQL.
	moveApplicationMigrations(t, databaseURL, -3)
	owner, err := store.CreateUser(ctx, "history-migration-owner", false)
	require.NoError(t, err)
	otherOwner, err := store.CreateUser(ctx, "history-migration-other", false)
	require.NoError(t, err)
	finishedBook, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Migrated finished book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	activeBook, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Unfinished Italian book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	require.NoError(t, err)
	otherBook, err := store.CreateBook(ctx, domain.Book{OwnerID: otherOwner.ID, Title: "Other owner's book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	completedAt := time.Date(2026, time.January, 20, 12, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `INSERT INTO reading_journeys(owner_id, language) VALUES ($1, 'de'), ($1, 'it'), ($2, 'de')`, owner.ID, otherOwner.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id, language, book_id, position) VALUES ($1, 'de', $3, 1), ($1, 'it', $4, 1), ($2, 'de', $5, 1)`, owner.ID, otherOwner.ID, finishedBook.ID, activeBook.ID, otherBook.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id, language, book_id, reading_finished_at) VALUES ($1, 'de', $3, $6), ($1, 'it', $4, NULL), ($2, 'de', $5, NULL)`, owner.ID, otherOwner.ID, finishedBook.ID, activeBook.ID, otherBook.ID, completedAt)
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "Haus", "NOUN")
	require.NoError(t, err)

	moveApplicationMigrations(t, databaseURL, 3)
	var historyCount int
	var migratedAt time.Time
	err = pool.QueryRow(ctx, `SELECT count(*), max(completed_at) FROM reading_history WHERE owner_id=$1`, owner.ID).Scan(&historyCount, &migratedAt)
	require.NoError(t, err)
	assert.Equal(t, 1, historyCount)
	assert.True(t, completedAt.Equal(migratedAt), "completion timestamp changed: got %s want %s", migratedAt, completedAt)
	goal, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, goal.BookID, "finished Goal survived backfill")
	italianGoal, err := store.GetPrimaryGoal(ctx, owner.ID, "it")
	require.NoError(t, err)
	assert.Equal(t, activeBook.ID, italianGoal.BookID, "language-isolated active Goal was changed")
	otherGoal, err := store.GetPrimaryGoal(ctx, otherOwner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, otherBook.ID, otherGoal.BookID, "owner-isolated active Goal was changed")
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, journey.Entries, "finished Book survived Journey backfill")
	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, known, 1, "backfill changed Known vocabulary")
}

func moveApplicationMigrations(t *testing.T, databaseURL string, steps int) {
	t.Helper()
	source, err := iofs.New(migrations.FS, ".")
	require.NoError(t, err)
	migrator, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	require.NoError(t, err)
	err = migrator.Steps(steps)
	sourceErr, databaseErr := migrator.Close()
	require.NoError(t, err)
	require.NoError(t, sourceErr)
	require.NoError(t, databaseErr)
}
