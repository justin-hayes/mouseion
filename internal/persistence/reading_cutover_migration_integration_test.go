//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadingCutoverMigrationPreservesIndependentReadingState(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	// Seed the latest pre-cutover schema so migration 18 itself proves the
	// membership-to-disposition contract and preserves unrelated durable state.
	moveApplicationMigrationsTo(t, databaseURL, 17)
	// Current application code consults the successor correction table while
	// starting a reading. Keep an empty stand-in during setup against this old
	// schema, then remove it so migration 23 is still exercised below.
	_, err := pool.Exec(ctx, `CREATE TABLE occurrence_lemma_corrections (
		owner_id uuid NOT NULL, book_id uuid NOT NULL, corpus_id uuid NOT NULL,
		analysis_run_id uuid NOT NULL
	)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `CREATE TABLE occurrence_lemma_review_flags (
		owner_id uuid NOT NULL, book_id uuid NOT NULL, analysis_run_id uuid NOT NULL,
		resolution text
	)`)
	require.NoError(t, err)
	// The current disposition writes include the successor revision field. Add it
	// temporarily while seeding with application code, then remove it so the
	// migration under test starts from the exact version-17 schema.
	_, err = pool.Exec(ctx, `ALTER TABLE book_dispositions ADD COLUMN revision bigint NOT NULL DEFAULT 1`)
	require.NoError(t, err)
	owner, err := store.CreateUser(ctx, "reading-cutover-owner", false)
	require.NoError(t, err)
	active, activeSource, deck := createReadingFixture(t, ctx, store, owner.ID, "cutover-active")
	makeAnalyzedToReadBook(t, ctx, store, active, activeSource)
	current, err := store.StartCurrentReading(ctx, owner.ID, "de", active.ID)
	require.NoError(t, err)

	finished, finishedSource, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "it", "cutover-finished")
	makeAnalyzedToReadBook(t, ctx, store, finished, finishedSource)
	finishedReading, err := store.StartCurrentReading(ctx, owner.ID, "it", finished.ID)
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(ctx, owner.ID, "it", finished.ID, finishedReading.SnapshotID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO reading_journeys(owner_id, language) VALUES ($1, 'de'), ($1, 'it') ON CONFLICT DO NOTHING`, owner.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id, language, book_id, position) VALUES ($1, 'de', $2, 1), ($1, 'it', $3, 1)`, owner.ID, active.ID, finished.ID)
	require.NoError(t, err)
	// Delete the active book's disposition to model a legacy membership row
	// without its successor representation.
	_, err = pool.Exec(ctx, `DELETE FROM book_dispositions WHERE owner_id=$1 AND book_id=$2`, owner.ID, active.ID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `ALTER TABLE book_dispositions DROP COLUMN revision`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DROP TABLE occurrence_lemma_corrections`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DROP TABLE occurrence_lemma_review_flags`)
	require.NoError(t, err)
	moveApplicationMigrationsTo(t, databaseURL, 18)
	migrateApplicationMigrationsToLatest(t, databaseURL)

	var tablePresent bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.reading_journey_membership') IS NOT NULL`).Scan(&tablePresent))
	assert.False(t, tablePresent, "ordered membership table survived cutover")
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.reading_journeys') IS NOT NULL`).Scan(&tablePresent))
	assert.False(t, tablePresent, "Journey revision table survived cutover")

	disposition, err := store.GetBookDisposition(ctx, owner.ID, active.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition, "legacy membership was not contracted to To Read")
	disposition, err = store.GetBookDisposition(ctx, owner.ID, finished.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, disposition, "cutover overwrote the finished Book's existing disposition")

	loaded, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, current, loaded, "current reading changed during cutover")
	var activeSnapshotCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NULL`, owner.ID, current.SnapshotID).Scan(&activeSnapshotCount))
	assert.Equal(t, 1, activeSnapshotCount, "active reservation snapshot was not preserved")
	var activeSnapshotVocabularyCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, owner.ID, current.SnapshotID).Scan(&activeSnapshotVocabularyCount))
	assert.Equal(t, current.SnapshotSize, activeSnapshotVocabularyCount, "active snapshot vocabulary was not preserved")

	var historyRows int
	var historyBookID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*), max(book_id::text) FROM reading_history WHERE owner_id=$1 AND language='it'`, owner.ID).Scan(&historyRows, &historyBookID))
	assert.Equal(t, 1, historyRows)
	assert.Equal(t, finished.ID, historyBookID)
	var historySnapshotID string
	var hasGoalSnapshot bool
	var historySnapshotVocabularyCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT goal_snapshot_id::text, goal_snapshot_id IS NOT NULL, snapshot_vocabulary_count FROM reading_history WHERE owner_id=$1 AND language='it' AND book_id=$2`, owner.ID, finished.ID).Scan(&historySnapshotID, &hasGoalSnapshot, &historySnapshotVocabularyCount))
	assert.Equal(t, finishedReading.SnapshotID, historySnapshotID, "completion lost its frozen snapshot provenance")
	assert.True(t, hasGoalSnapshot, "Mouseion completion lost its snapshot provenance")
	assert.Equal(t, finishedReading.SnapshotSize, historySnapshotVocabularyCount)

	persistedDeck, err := store.GetDeckPreparation(ctx, owner.ID, deck.ID)
	require.NoError(t, err)
	assert.Equal(t, deck, persistedDeck, "prepared-deck provenance changed during cutover")
	artifact, err := store.DownloadDeckPreparation(ctx, owner.ID, deck.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, artifact.Artifact)
}
