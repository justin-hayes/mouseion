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

func TestCurrentReadingPersistence(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)

	alice, err := store.CreateUser(ctx, "goal-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "goal-bob", false)
	require.NoError(t, err)
	carol, err := store.CreateUser(ctx, "goal-carol", false)
	require.NoError(t, err)

	aliceBook, _ := createReadingFixture(t, ctx, store, alice.ID, "goal-active")
	bobBook, bobSource := createReadingFixture(t, ctx, store, bob.ID, "goal-queued")
	makeAnalyzedToReadBook(t, ctx, store, bobBook, bobSource)

	goal, err := store.GetCurrentReading(ctx, carol.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.CurrentReading{}, goal, "owner without a reading")
	// Model a persisted language-scoped row directly against the baseline.
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, alice.ID, aliceBook.ID)
	require.NoError(t, err)
	goal, err = store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, alice.ID, goal.OwnerID)
	assert.Equal(t, aliceBook.ID, goal.BookID)

	italianBook, italianSource := createReadingFixtureInLanguage(t, ctx, store, alice.ID, "it", "goal-italian")
	makeAnalyzedToReadBook(t, ctx, store, italianBook, italianSource)
	_, err = store.StartCurrentReading(ctx, alice.ID, "it", italianBook.ID)
	require.NoError(t, err)
	require.NoError(t, store.EndCurrentReading(ctx, alice.ID, "it"), "end the parallel Italian reading")
	germanGoal, err := store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, aliceBook.ID, germanGoal.BookID, "the German reading survives the Italian End")

	t.Run("cross-owner Book is not found", func(t *testing.T) {
		_, err := store.StartCurrentReading(ctx, carol.ID, "de", bobBook.ID)
		require.ErrorIs(t, err, ErrNotFound)
	})
	t.Run("a Book without analysis is ineligible", func(t *testing.T) {
		noDeckBook, err := store.CreateBook(ctx, domain.Book{OwnerID: carol.ID, Title: "Carol reading-only book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
		require.NoError(t, err)
		_, err = store.StartCurrentReading(ctx, carol.ID, "de", noDeckBook.ID)
		require.ErrorIs(t, err, ErrCurrentReadingIneligible)
	})
	t.Run("another owner's reading is not found", func(t *testing.T) {
		require.ErrorIs(t, store.EndCurrentReading(ctx, bob.ID, "de"), ErrNotFound)
		_, err := store.FinishCurrentReading(ctx, bob.ID, "de")
		require.ErrorIs(t, err, ErrNotFound)
		current, err := store.GetCurrentReading(ctx, alice.ID, "de")
		require.NoError(t, err)
		assert.Equal(t, aliceBook.ID, current.BookID, "another owner's End never touches the reading")
	})
}

func TestReadNextReplacesTheCurrentReadingWithoutASnapshot(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "current-reading-alice", false)
	require.NoError(t, err)
	first, firstSource := createReadingFixture(t, ctx, store, alice.ID, "current-reading-first")
	second, secondSource := createReadingFixture(t, ctx, store, alice.ID, "current-reading-second")
	makeAnalyzedToReadBook(t, ctx, store, first, firstSource)
	makeAnalyzedToReadBook(t, ctx, store, second, secondSource)

	reading, err := store.StartCurrentReading(ctx, alice.ID, "de", first.ID)
	require.NoError(t, err)
	assert.Equal(t, first.ID, reading.BookID)

	replayed, err := store.StartCurrentReadingResult(ctx, alice.ID, "de", first.ID)
	require.NoError(t, err)
	assert.True(t, replayed.Replayed, "reading the current Book again changes nothing")

	_, err = store.Pool().Exec(ctx, `
CREATE FUNCTION test_current_reading_switch_failure() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced current-reading switch failure'; END; $$;
CREATE TRIGGER test_current_reading_switch_failure
BEFORE UPDATE ON primary_goals FOR EACH ROW EXECUTE FUNCTION test_current_reading_switch_failure();`)
	require.NoError(t, err)
	_, err = store.StartCurrentReading(ctx, alice.ID, "de", second.ID)
	require.ErrorContains(t, err, "forced current-reading switch failure")
	current, err := store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, first.ID, current.BookID, "a failed Read next keeps the current reading")
	_, err = store.Pool().Exec(ctx, `DROP TRIGGER test_current_reading_switch_failure ON primary_goals; DROP FUNCTION test_current_reading_switch_failure()`)
	require.NoError(t, err)

	reading, err = store.StartCurrentReading(ctx, alice.ID, "de", second.ID)
	require.NoError(t, err)
	assert.Equal(t, second.ID, reading.BookID)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, first.ID), "the former Book stays To Read")

	var snapshots, snapshotRefs int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1`, alice.ID).Scan(&snapshots))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1 AND snapshot_id IS NOT NULL`, alice.ID).Scan(&snapshotRefs))
	assert.Zero(t, snapshots, "Read next writes no vocabulary snapshot")
	assert.Zero(t, snapshotRefs, "the reading names no snapshot")
}

// TestReadNextReplacesALegacyReadingThatNamesASnapshot holds that a reading
// started before the snapshot was retired is replaced like any other; the store
// no longer reads or releases the snapshot it names (ADR 0136 §4).
func TestReadNextReplacesALegacyReadingThatNamesASnapshot(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-legacy", false)
	require.NoError(t, err)
	legacy, legacySource := createReadingFixture(t, ctx, store, owner.ID, "legacy-first")
	next, nextSource := createReadingFixture(t, ctx, store, owner.ID, "legacy-next")
	makeAnalyzedToReadBook(t, ctx, store, legacy, legacySource)
	makeAnalyzedToReadBook(t, ctx, store, next, nextSource)

	// A reading started before the snapshot was retired still names one.
	var snapshotID string
	require.NoError(t, store.Pool().QueryRow(ctx, `
INSERT INTO primary_goal_snapshots(owner_id, language, book_id, source_material_id, analysis_run_id, content_revision_id, content_snapshot_id, corpus_id)
SELECT ca.owner_id, 'de', ca.book_id, ca.source_material_id, ca.analysis_run_id, ca.content_revision_id, ca.snapshot_id, ca.corpus_id
FROM current_analysis_identity ca WHERE ca.owner_id=$1 AND ca.book_id=$2 RETURNING id::text`, owner.ID, legacy.ID).Scan(&snapshotID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id,snapshot_id) VALUES($1,'de',$2,$3)`, owner.ID, legacy.ID, snapshotID)
	require.NoError(t, err)

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", next.ID)
	require.NoError(t, err)
	assert.Equal(t, next.ID, reading.BookID)
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, next.ID, current.BookID)
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de"), "End clears a reading that still names a legacy snapshot")
}

func TestEndCurrentReadingNeedsNoCommitmentAndRecordsNothing(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "end-no-commitment", false)
	require.NoError(t, err)
	book, source := createReadingFixture(t, ctx, store, owner.ID, "end-book")
	makeAnalyzedToReadBook(t, ctx, store, book, source)

	require.ErrorIs(t, store.EndCurrentReading(ctx, owner.ID, "de"), ErrNotFound, "nothing to end")
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de"))
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner.ID, book.ID), "End keeps the Book To Read")
	var completions int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1`, owner.ID).Scan(&completions))
	assert.Zero(t, completions, "End records no completion")
	require.ErrorIs(t, store.EndCurrentReading(ctx, owner.ID, "de"), ErrNotFound, "a second End has nothing to end")
	require.ErrorIs(t, store.EndCurrentReading(ctx, "not-a-uuid", "de"), ErrNotFound, "malformed owner")
}

func TestFinishCurrentReadingAppendsACompletionEachTime(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "finish-appends", false)
	require.NoError(t, err)
	book, source := createReadingFixture(t, ctx, store, owner.ID, "finish-book")
	makeAnalyzedToReadBook(t, ctx, store, book, source)

	_, err = store.FinishCurrentReading(ctx, owner.ID, "de")
	require.ErrorIs(t, err, ErrNotFound, "nothing to finish")
	for round := 1; round <= 2; round++ {
		require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
		_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
		require.NoError(t, err)
		result, err := store.FinishCurrentReading(ctx, owner.ID, "de")
		require.NoError(t, err)
		assert.Equal(t, book.ID, result.Completion.BookID)
		assert.Equal(t, "de", result.Completion.Language)
		current, err := store.GetCurrentReading(ctx, owner.ID, "de")
		require.NoError(t, err)
		assert.False(t, current.IsActive(), "Finish clears the role")
		assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, owner.ID, book.ID), "Finish clears To Read")
		var completions, withSnapshot, known int
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*), count(goal_snapshot_id) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&completions, &withSnapshot))
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&known))
		assert.Equal(t, round, completions, "every Finish appends one completion")
		assert.Zero(t, withSnapshot, "a completion names no snapshot")
		assert.Zero(t, known, "Finish has no vocabulary effect")
	}
	_, err = store.FinishCurrentReading(ctx, "not-a-uuid", "de")
	require.ErrorIs(t, err, ErrNotFound, "malformed owner")
}

func TestCurrentReadingStartRejectsEachIneligibilityReason(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-ineligible", false)
	require.NoError(t, err)
	anchor, anchorSource := createReadingFixture(t, ctx, store, owner.ID, "ineligible-anchor")
	makeAnalyzedToReadBook(t, ctx, store, anchor, anchorSource)
	cases := ineligibleCurrentReadingCases(t, ctx, store, owner.ID)

	for _, c := range cases {
		_, err := store.StartCurrentReading(ctx, owner.ID, "de", c.book.ID)
		requireCurrentReadingRejected(t, err, c.reason, "start "+c.name)
	}
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive(), "rejected Read next leaves no current reading")

	_, err = store.StartCurrentReading(ctx, owner.ID, "de", anchor.ID)
	require.NoError(t, err)
	for _, c := range cases {
		_, err := store.StartCurrentReading(ctx, owner.ID, "de", c.book.ID)
		requireCurrentReadingRejected(t, err, c.reason, "replace "+c.name)
	}
	current, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, anchor.ID, current.BookID, "rejected Read next keeps the current reading")
}

func TestCurrentReadingStartTreatsMalformedIdentitiesAsNotFound(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-malformed", false)
	require.NoError(t, err)
	book, source := createReadingFixture(t, ctx, store, owner.ID, "malformed-anchor")
	makeAnalyzedToReadBook(t, ctx, store, book, source)

	for _, bookID := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000", "' OR 1=1 --"} {
		_, err := store.StartCurrentReading(ctx, owner.ID, "de", bookID)
		require.ErrorIs(t, err, ErrNotFound, bookID)
	}
	_, err = store.StartCurrentReading(ctx, "not-a-uuid", "de", book.ID)
	require.ErrorIs(t, err, ErrNotFound, "malformed owner")
}

func TestCurrentReadingStartAcceptsPublishedAnalysisDuringReAnalysis(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-re-analysis", false)
	require.NoError(t, err)

	for _, state := range []string{"queued", "running", "failed", "cancelled"} {
		book, source := createReadingFixture(t, ctx, store, owner.ID, "re-analysis-"+state)
		makeAnalyzedToReadBook(t, ctx, store, book, source)
		insertAnalysisRun(t, ctx, store, source, state)

		reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
		require.NoError(t, err, "start during %s re-analysis", state)
		assert.Equal(t, book.ID, reading.BookID, "%s re-analysis keeps the published analysis", state)
		require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de"))
	}
}

// currentReadingIneligibleCase is a Book the classifier rejects for one reason
// when it is read next as as the German current reading.
type currentReadingIneligibleCase struct {
	name   string
	reason domain.CurrentReadingEligibilityReason
	book   domain.Book
}

func ineligibleCurrentReadingCases(t *testing.T, ctx context.Context, store *PostgresStore, owner string) []currentReadingIneligibleCase {
	t.Helper()
	notToRead, notToReadSource := createReadingFixture(t, ctx, store, owner, "case-inbox")
	makeAnalyzedToReadBook(t, ctx, store, notToRead, notToReadSource)
	require.NoError(t, store.MarkBookPreviouslyRead(ctx, owner, notToRead.ID))

	unknownLanguage, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Reading case unknown language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner, unknownLanguage.ID, domain.BookDispositionToRead))

	unacquired, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Reading case unacquired", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner, unacquired.ID, domain.BookDispositionToRead))

	italian, italianSource := createReadingFixtureInLanguage(t, ctx, store, owner, "it", "case-italian")
	makeAnalyzedToReadBook(t, ctx, store, italian, italianSource)

	stale, staleSource := createReadingFixture(t, ctx, store, owner, "case-stale")
	makeAnalyzedToReadBook(t, ctx, store, stale, staleSource)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET status='pending' WHERE owner_id=$1 AND source_material_id=$2`, owner, staleSource.ID)
	require.NoError(t, err)

	return []currentReadingIneligibleCase{
		{name: "not-to-read", reason: domain.CurrentReadingNotToRead, book: notToRead},
		{name: "no-chosen-language", reason: domain.CurrentReadingNoChosenLanguage, book: unknownLanguage},
		{name: "needs-current-content", reason: domain.CurrentReadingNeedsCurrentContent, book: unacquired},
		{name: "analysis-in-progress", reason: domain.CurrentReadingAnalysisInProgress, book: unanalyzedToReadBook(t, ctx, store, owner, "case-queued", "queued")},
		{name: "failed", reason: domain.CurrentReadingFailed, book: unanalyzedToReadBook(t, ctx, store, owner, "case-failed", "failed")},
		{name: "cancelled", reason: domain.CurrentReadingCancelled, book: unanalyzedToReadBook(t, ctx, store, owner, "case-cancelled", "cancelled")},
		{name: "stale", reason: domain.CurrentReadingStale, book: stale},
		{name: "no-completed-analysis", reason: domain.CurrentReadingNoCompletedAnalysis, book: unanalyzedToReadBook(t, ctx, store, owner, "case-unanalyzed", "")},
		{name: "other-language", reason: domain.CurrentReadingOtherLanguage, book: italian},
	}
}

// unanalyzedToReadBook makes a To Read Book whose latest analysis run is in
// runState, or has none when runState is empty.
func unanalyzedToReadBook(t *testing.T, ctx context.Context, store *PostgresStore, owner, suffix, runState string) domain.Book {
	t.Helper()
	book, source := createReadingFixture(t, ctx, store, owner, suffix)
	require.NoError(t, store.SetBookDisposition(ctx, owner, book.ID, domain.BookDispositionToRead))
	if runState != "" {
		insertAnalysisRun(t, ctx, store, source, runState)
	}
	return book
}

// insertAnalysisRun records a newer analysis run for source in state, with
// the job that the evidence view reads as its latest run. It leaves any
// published analysis in place, as a re-analysis does.
func insertAnalysisRun(t *testing.T, ctx context.Context, store *PostgresStore, source domain.SourceMaterial, state string) {
	t.Helper()
	var snapshotID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, source.OwnerID, source.ID).Scan(&snapshotID))
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state) VALUES($1,$2,$3,$4,'test','1',$5,$6) RETURNING id::text`, source.OwnerID, source.ID, source.ContentRevisionID, snapshotID, "run-"+state, state).Scan(&runID))
	_, err := store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,display_number,owner_id,source_material_id,content_hash,analysis_run_id) SELECT COALESCE(MAX(river_job_id),0)+1, COALESCE(MAX(display_number),0)+1, $1, $2, $3, $4 FROM analysis_jobs`, source.OwnerID, source.ID, source.ContentHash, runID)
	require.NoError(t, err)
}

func requireCurrentReadingRejected(t *testing.T, err error, reason domain.CurrentReadingEligibilityReason, label string) {
	t.Helper()
	require.ErrorIs(t, err, ErrCurrentReadingIneligible, label)
	var rejected CurrentReadingIneligibleError
	require.ErrorAs(t, err, &rejected, label)
	assert.Equal(t, reason, rejected.Reason, label)
}
