//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKnownVocabularyCleanupDeletesOnlyImportsAndDiscardsPendingJobs exercises
// migration 34: a data-only cleanup that removes imported Known vocabulary while
// leaving Reading-completion rows, active snapshots, queue history, and
// processing history untouched.
func TestKnownVocabularyCleanupDeletesOnlyImportsAndDiscardsPendingJobs(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	// Seed at version 33 so migration 34 runs as a real successor migration.
	moveApplicationMigrationsTo(t, databaseURL, 33)
	owner, err := store.CreateUser(ctx, "known-cleanup-owner", false)
	require.NoError(t, err)

	// A finished Reading contributes one completion-provenance row, which must
	// survive. Its active snapshot is the frozen vocabulary the migration must
	// not rewrite.
	book, snapshot := insertReadingCompletionFixture(t, ctx, pool, store, owner.ID, "ablauf")

	// Imported rows have no completion provenance. Seed both a lemma-wide
	// (POS-less) row and a per-POS row.
	_, err = pool.Exec(ctx, `INSERT INTO known_vocabulary(owner_id, language, canonical_lemma, upos) VALUES ($1, 'de', 'haus', ''), ($1, 'de', 'gehen', 'VERB')`, owner.ID)
	require.NoError(t, err)

	// River holds live and finalized import jobs; only the live states may be
	// discarded.
	insertKnownVocabularyJobs(t, ctx, pool)

	// Facts the migration must preserve.
	_, err = pool.Exec(ctx, `INSERT INTO processing_history(owner_id, operation, status, details) VALUES ($1, 'known_vocabulary.import', 'completed', '{"river_job_id": 1}')`, owner.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO reading_history(owner_id, language, book_id, completed_at, goal_snapshot_id,
    snapshot_vocabulary_count, eligible_vocabulary_count, graduated_vocabulary_count, already_known_vocabulary_count)
VALUES ($1, 'de', $2, now(), $3, 3, 2, 1, 1)`, owner.ID, book, snapshot)
	require.NoError(t, err)

	moveApplicationMigrationsTo(t, databaseURL, 34)

	t.Run("completion rows survive", func(t *testing.T) {
		var count int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND completion_book_id IS NOT NULL`, owner.ID).Scan(&count))
		require.Equal(t, 1, count)
		var lemma, upos string
		require.NoError(t, pool.QueryRow(ctx, `SELECT canonical_lemma, upos FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&lemma, &upos))
		assert.Equal(t, "ablauf", lemma)
		assert.Equal(t, "NOUN", upos)
	})

	t.Run("provenance-less rows are deleted", func(t *testing.T) {
		var count int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND completion_book_id IS NULL`, owner.ID).Scan(&count))
		assert.Zero(t, count)
	})

	// This live-state list mirrors migration 34's contract; both must change
	// together.
	t.Run("pending jobs are discarded and finalized history kept", func(t *testing.T) {
		var live, finalized int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='import_known_vocabulary' AND state IN ('available','pending','running','retryable','scheduled')`).Scan(&live))
		assert.Zero(t, live)
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='import_known_vocabulary' AND state='completed'`).Scan(&finalized))
		assert.Equal(t, 1, finalized)
	})

	t.Run("active snapshot and history are unchanged", func(t *testing.T) {
		var snapshotVocabulary int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, owner.ID, snapshot).Scan(&snapshotVocabulary))
		assert.Equal(t, 1, snapshotVocabulary)
		var historyRows int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='known_vocabulary.import'`, owner.ID).Scan(&historyRows))
		assert.Equal(t, 1, historyRows)
		var eligible, alreadyKnown, snapshotCount int
		require.NoError(t, pool.QueryRow(ctx, `SELECT eligible_vocabulary_count, already_known_vocabulary_count, snapshot_vocabulary_count FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book).Scan(&eligible, &alreadyKnown, &snapshotCount))
		assert.Equal(t, 2, eligible)
		assert.Equal(t, 1, alreadyKnown)
		assert.Equal(t, 3, snapshotCount)
	})

	t.Run("a second run changes nothing", func(t *testing.T) {
		// Replay the down migration (a no-op) and the up migration against a
		// freshly imported row: the rerun still deletes it and leaves every
		// retained fact alone.
		moveApplicationMigrationsTo(t, databaseURL, 33)
		_, err := pool.Exec(ctx, `INSERT INTO known_vocabulary(owner_id, language, canonical_lemma, upos) VALUES ($1, 'de', 'neu', '')`, owner.ID)
		require.NoError(t, err)
		moveApplicationMigrationsTo(t, databaseURL, 34)

		var imported int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND completion_book_id IS NULL`, owner.ID).Scan(&imported))
		assert.Zero(t, imported, "rerun left a provenance-less row")
		var total int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&total))
		assert.Equal(t, 1, total, "rerun changed the surviving completion row")
		var jobs int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='import_known_vocabulary'`).Scan(&jobs))
		assert.Equal(t, 1, jobs, "rerun changed queue history")
		var snapshotVocabulary int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, owner.ID, snapshot).Scan(&snapshotVocabulary))
		assert.Equal(t, 1, snapshotVocabulary, "rerun changed the active snapshot")
	})
}

// insertReadingCompletionFixture seeds one finished Reading with an active
// snapshot and a completion-provenance Known row for lemma. It returns the Book
// and the active snapshot so callers can assert neither changed.
func insertReadingCompletionFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *PostgresStore, owner, lemma string) (book, snapshot string) {
	t.Helper()
	book, source, run, corpus := insertMigrationAnalysisFixture(t, ctx, pool, store, owner, "de", "known-cleanup")
	var revisionID, contentSnapshotID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT current_content_revision_id::text, current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, source).Scan(&revisionID, &contentSnapshotID))
	snapshot = uuid.NewString()
	_, err := pool.Exec(ctx, `
INSERT INTO primary_goal_snapshots(id, owner_id, language, book_id, source_material_id, analysis_run_id,
    content_revision_id, content_snapshot_id, corpus_id)
VALUES ($1, $2, 'de', $3, $4, $5, $6, $7, $8)`, snapshot, owner, book, source, run, revisionID, contentSnapshotID, corpus)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id, language, book_id, snapshot_id) VALUES ($1, 'de', $2, $3)`, owner, book, snapshot)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO primary_goal_snapshot_vocabulary(owner_id, snapshot_id, corpus_id, language, canonical_lemma, upos,
    occurrence_count, observed_forms, eligible_sentence_refs, provenance)
VALUES ($1, $2, $3, 'de', $4, 'NOUN', 3, '[]', '[]', '{}')`, owner, snapshot, corpus, lemma)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO known_vocabulary(owner_id, language, canonical_lemma, upos,
    completion_book_id, completion_at, completion_goal_snapshot_id, completion_source_material_id,
    completion_analysis_run_id, completion_content_revision_id, completion_content_snapshot_id, completion_corpus_id)
VALUES ($1, 'de', $2, 'NOUN', $3, now(), $4, $5, $6, $7, $8, $9)`, owner, lemma, book, snapshot, source, run, revisionID, contentSnapshotID, corpus)
	require.NoError(t, err)
	return book, snapshot
}

// insertKnownVocabularyJobs seeds one import job in each River state: the five
// live states the cleanup discards and one finalized state it retains.
func insertKnownVocabularyJobs(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO river_job(kind, state, args, queue, scheduled_at, finalized_at) VALUES
    ('import_known_vocabulary', 'available', '{}', 'known_vocabulary', now(), NULL),
    ('import_known_vocabulary', 'pending',   '{}', 'known_vocabulary', now(), NULL),
    ('import_known_vocabulary', 'running',   '{}', 'known_vocabulary', now(), NULL),
    ('import_known_vocabulary', 'retryable', '{}', 'known_vocabulary', now(), NULL),
    ('import_known_vocabulary', 'scheduled', '{}', 'known_vocabulary', now(), NULL),
    ('import_known_vocabulary', 'completed', '{}', 'known_vocabulary', now(), now())`)
	require.NoError(t, err)
}
