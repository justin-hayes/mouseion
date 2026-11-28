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
		analysis_run_id uuid NOT NULL, source_document_id text NOT NULL,
		start_offset bigint NOT NULL, end_offset bigint NOT NULL,
		canonical_lemma text NOT NULL, excluded boolean NOT NULL
	)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `CREATE TABLE occurrence_lemma_review_flags (
		owner_id uuid NOT NULL, book_id uuid NOT NULL, analysis_run_id uuid NOT NULL,
		evidence_provenance jsonb NOT NULL DEFAULT '{}'::jsonb,
		resolution text
	)`)
	require.NoError(t, err)
	// The current disposition writes include the successor revision field. Add it
	// temporarily while seeding with application code, then remove it so the
	// migration under test starts from the exact version-17 schema.
	_, err = pool.Exec(ctx, `ALTER TABLE book_dispositions ADD COLUMN revision bigint NOT NULL DEFAULT 1`)
	require.NoError(t, err)
	// Current application code also reads and writes the successor identity
	// columns (analysis syntax state, the token, frozen snapshot and Known
	// discriminator and headword). Add empty stand-ins while seeding against
	// this old schema, then drop them so the successor migrations that
	// introduce them still run below.
	_, err = pool.Exec(ctx, `ALTER TABLE analysis_runs ADD COLUMN syntax_state text NOT NULL DEFAULT 'ud'`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE corpus_tokens ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '', ADD COLUMN headword text NOT NULL DEFAULT ''`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE primary_goal_snapshot_vocabulary ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '', ADD COLUMN headword text NOT NULL DEFAULT ''`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE known_vocabulary ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '', ADD COLUMN headword text NOT NULL DEFAULT ''`)
	require.NoError(t, err)
	// Current writers name the successor snapshot convention column. Add an
	// empty stand-in while seeding against this old schema, then drop it so
	// migration 45 still runs below.
	_, err = pool.Exec(ctx, `ALTER TABLE source_material_unit_snapshots ADD COLUMN structure_convention_set text`)
	require.NoError(t, err)
	// Likewise the content revision's Document digest (migration 84).
	_, err = pool.Exec(ctx, `ALTER TABLE source_content_revisions ADD COLUMN document_digest text`)
	require.NoError(t, err)
	owner, err := store.CreateUser(ctx, "reading-cutover-owner", false)
	require.NoError(t, err)
	active, activeSource := createReadingFixture(t, ctx, store, owner.ID, "cutover-active")
	makeAnalyzedToReadBook(t, ctx, store, active, activeSource)
	current, err := store.StartCurrentReading(ctx, owner.ID, "de", active.ID)
	require.NoError(t, err)

	finished, finishedSource := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "it", "cutover-finished")
	makeAnalyzedToReadBook(t, ctx, store, finished, finishedSource)
	_, err = store.StartCurrentReading(ctx, owner.ID, "it", finished.ID)
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(ctx, owner.ID, "it")
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
	_, err = pool.Exec(ctx, `ALTER TABLE analysis_runs DROP COLUMN syntax_state`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE corpus_tokens DROP COLUMN lexical_discriminator, DROP COLUMN headword`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE primary_goal_snapshot_vocabulary DROP COLUMN lexical_discriminator, DROP COLUMN headword`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE known_vocabulary DROP COLUMN lexical_discriminator, DROP COLUMN headword`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE source_material_unit_snapshots DROP COLUMN structure_convention_set`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `ALTER TABLE source_content_revisions DROP COLUMN document_digest`)
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
	var historyRows int
	var historyBookID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*), max(book_id::text) FROM reading_history WHERE owner_id=$1 AND language='it'`, owner.ID).Scan(&historyRows, &historyBookID))
	assert.Equal(t, 1, historyRows)
	assert.Equal(t, finished.ID, historyBookID)
}
