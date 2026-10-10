//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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
	dave, err := store.CreateUser(ctx, "goal-dave", false)
	require.NoError(t, err)
	erin, err := store.CreateUser(ctx, "goal-erin", false)
	require.NoError(t, err)

	aliceBook, _, _ := createReadingFixture(t, ctx, store, alice.ID, "goal-active")
	bobBook, _, _ := createReadingFixture(t, ctx, store, bob.ID, "goal-queued")
	_, _, _ = createReadingFixture(t, ctx, store, dave.ID, "goal-complete")
	_, _, _ = createReadingFixture(t, ctx, store, erin.ID, "goal-abandoned")

	goal, err := store.GetCurrentReading(ctx, carol.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.CurrentReading{}, goal, "owner without a Goal")
	// Model a legacy goal directly against the baseline. The historical
	// migration that added language is retired, but current goal behavior still
	// needs coverage for persisted language-scoped rows.
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, alice.ID, aliceBook.ID)
	require.NoError(t, err)

	goal, err = store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, alice.ID, goal.OwnerID, "migrated active goal")
	assert.Equal(t, aliceBook.ID, goal.BookID, "migrated active goal")
	italianBook, italianSource, _ := createReadingFixtureInLanguage(t, ctx, store, alice.ID, "it", "goal-italian")
	makeAnalyzedToReadBook(t, ctx, store, italianBook, italianSource)
	italianReading, err := store.StartCurrentReading(ctx, alice.ID, "it", italianBook.ID)
	require.NoError(t, err)
	italianGoal, getErr := store.GetCurrentReading(ctx, alice.ID, "it")
	require.NoError(t, getErr)
	assert.Equal(t, italianBook.ID, italianGoal.BookID, "parallel Italian goal")
	err = store.EndCurrentReading(ctx, alice.ID, "it", italianBook.ID, italianReading.SnapshotID)
	require.NoError(t, err, "end parallel Italian reading")
	germanGoal, getErr := store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, getErr)
	assert.Equal(t, aliceBook.ID, germanGoal.BookID, "German goal after Italian clear")
	for _, owner := range []string{bob.ID, carol.ID, dave.ID, erin.ID} {
		goal, err := store.GetCurrentReading(ctx, owner, "de")
		require.NoError(t, err)
		assert.Equal(t, domain.CurrentReading{}, goal, "non-active owner=%s", owner)
	}

	otherAliceBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice second goal book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.StartCurrentReading(ctx, alice.ID, "de", otherAliceBook.ID)
	assert.ErrorIs(t, err, ErrCurrentReadingExists) //nolint:testifylint // Duplicate-goal rejection is independent of later eligibility cases.

	noDeckBook, err := store.CreateBook(ctx, domain.Book{OwnerID: carol.ID, Title: "Carol reading-only book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.StartCurrentReading(ctx, carol.ID, "de", noDeckBook.ID)
	assert.ErrorIs(t, err, ErrCurrentReadingIneligible) //nolint:testifylint // Eligibility rejection is an independent goal case.
	_, err = store.StartCurrentReading(ctx, carol.ID, "de", bobBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner reading rejection is independently asserted.

	replacementBook, replacementSource, _ := createReadingFixture(t, ctx, store, carol.ID, "goal-replacement")
	makeAnalyzedToReadBook(t, ctx, store, replacementBook, replacementSource)
	started, err := store.StartCurrentReading(ctx, carol.ID, "de", replacementBook.ID)
	require.NoError(t, err)
	switchTarget, switchSource, _ := createReadingFixture(t, ctx, store, carol.ID, "goal-switch-target")
	makeAnalyzedToReadBook(t, ctx, store, switchTarget, switchSource)
	_, err = store.SwitchCurrentReading(ctx, carol.ID, "de", otherAliceBook.ID, replacementBook.ID, started.SnapshotID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner switch target rejection is independent of the valid switch.
	_, err = store.SwitchCurrentReading(ctx, carol.ID, "de", switchTarget.ID, "stale-book", started.SnapshotID)
	assert.ErrorIs(t, err, ErrCurrentReadingStale) //nolint:testifylint // Stale-book switch rejection is independent of the valid switch.
	switched, err := store.SwitchCurrentReading(ctx, carol.ID, "de", switchTarget.ID, replacementBook.ID, started.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, switchTarget.ID, switched.BookID, "valid switch")
	err = store.EndCurrentReading(ctx, carol.ID, "de", replacementBook.ID, started.SnapshotID)
	assert.ErrorIs(t, err, ErrCurrentReadingStale) //nolint:testifylint // Stale end rejection is independent of the valid end.
	err = store.EndCurrentReading(ctx, carol.ID, "de", switchTarget.ID, switched.SnapshotID)
	require.NoError(t, err, "valid end")
	err = store.EndCurrentReading(ctx, carol.ID, "de", switchTarget.ID, switched.SnapshotID)
	require.NoError(t, err, "replaying end is idempotent")
	err = store.EndCurrentReading(ctx, bob.ID, "de", aliceBook.ID, started.SnapshotID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner end rejection is independent of the final owner lookup.
	goal, err = store.GetCurrentReading(ctx, bob.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.CurrentReading{}, goal, "cross-owner get goal")

}

func TestCurrentReadingPersistenceInterfacePreservesLifecycleGuards(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "current-reading-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "current-reading-bob", false)
	require.NoError(t, err)
	first, firstSource, _ := createReadingFixture(t, ctx, store, alice.ID, "current-reading-first")
	second, secondSource, _ := createReadingFixture(t, ctx, store, alice.ID, "current-reading-second")
	makeAnalyzedToReadBook(t, ctx, store, first, firstSource)
	makeAnalyzedToReadBook(t, ctx, store, second, secondSource)

	reading, err := store.StartCurrentReading(ctx, alice.ID, "de", first.ID)
	require.NoError(t, err)
	assert.Equal(t, first.ID, reading.BookID)
	assert.NotEmpty(t, reading.SnapshotID)
	firstSnapshotID := reading.SnapshotID
	_, err = store.FinishCurrentReading(ctx, alice.ID, "de", second.ID, reading.SnapshotID)
	require.ErrorIs(t, err, ErrCurrentReadingStale)

	other, err := store.GetCurrentReading(ctx, bob.ID, "de")
	require.NoError(t, err)
	assert.False(t, other.IsActive(), "current reading is owner-scoped")
	_, err = store.SwitchCurrentReading(ctx, alice.ID, "de", second.ID, "stale-book", reading.SnapshotID)
	require.ErrorIs(t, err, ErrCurrentReadingStale)
	_, err = store.SwitchCurrentReading(ctx, alice.ID, "de", second.ID, first.ID, "stale-snapshot")
	require.ErrorIs(t, err, ErrCurrentReadingStale)
	_, err = store.Pool().Exec(ctx, `
CREATE FUNCTION test_current_reading_switch_failure() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced current-reading switch failure'; END; $$;
CREATE TRIGGER test_current_reading_switch_failure
BEFORE UPDATE ON primary_goals FOR EACH ROW EXECUTE FUNCTION test_current_reading_switch_failure();`)
	require.NoError(t, err)
	_, err = store.SwitchCurrentReading(ctx, alice.ID, "de", second.ID, first.ID, firstSnapshotID)
	require.ErrorContains(t, err, "forced current-reading switch failure")
	currentAfterFailedSwitch, err := store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, first.ID, currentAfterFailedSwitch.BookID)
	assert.Equal(t, firstSnapshotID, currentAfterFailedSwitch.SnapshotID)
	var releasedOldSnapshot int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, alice.ID, firstSnapshotID).Scan(&releasedOldSnapshot))
	assert.Zero(t, releasedOldSnapshot, "failed switch keeps the old reservation active")
	var rolledBackReplacementSnapshot int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND book_id=$2`, alice.ID, second.ID).Scan(&rolledBackReplacementSnapshot))
	assert.Zero(t, rolledBackReplacementSnapshot, "failed switch rolls back the replacement snapshot")
	_, err = store.Pool().Exec(ctx, `DROP TRIGGER test_current_reading_switch_failure ON primary_goals; DROP FUNCTION test_current_reading_switch_failure()`)
	require.NoError(t, err)

	reading, err = store.SwitchCurrentReading(ctx, alice.ID, "de", second.ID, first.ID, firstSnapshotID)
	require.NoError(t, err)
	assert.Equal(t, second.ID, reading.BookID)
	replayedSwitch, err := store.SwitchCurrentReading(ctx, alice.ID, "de", second.ID, first.ID, firstSnapshotID)
	require.NoError(t, err, "replaying switch is idempotent")
	assert.Equal(t, reading.SnapshotID, replayedSwitch.SnapshotID, "retry retains the switched snapshot")
	releasedOldSnapshot = 0
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, alice.ID, firstSnapshotID).Scan(&releasedOldSnapshot))
	assert.Equal(t, 1, releasedOldSnapshot, "successful switch releases only the former active snapshot")
	require.ErrorIs(t, store.EndCurrentReading(ctx, alice.ID, "de", first.ID, reading.SnapshotID), ErrCurrentReadingStale)
	require.NoError(t, store.EndCurrentReading(ctx, alice.ID, "de", second.ID, reading.SnapshotID))

	reading, err = store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.False(t, reading.IsActive())
	reading, err = store.StartCurrentReading(ctx, alice.ID, "de", first.ID)
	require.NoError(t, err)
	finished, err := store.FinishCurrentReading(ctx, alice.ID, "de", first.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, reading.SnapshotID, finished.Completion.SnapshotID)
	replayed, err := store.FinishCurrentReading(ctx, alice.ID, "de", first.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, finished, replayed)
}

func TestCurrentReadingCanStartAnalyzedToReadBookWithoutOrdering(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-to-read", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "to-read-only")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	queueFailure := errors.New("prepared deck queue unavailable")
	_, err = store.StartCurrentReadingWith(ctx, owner.ID, "de", book.ID, func(context.Context, pgx.Tx, domain.CurrentReading) error {
		return queueFailure
	})
	require.ErrorIs(t, err, queueFailure)
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, current.BookID, "failed durable enqueue rolls back the current reading")
	var snapshots int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&snapshots))
	assert.Zero(t, snapshots, "failed durable enqueue rolls back the frozen snapshot")

	current, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.Equal(t, book.ID, current.BookID)
	assert.Equal(t, source.ID, current.SourceMaterialID)
	assert.NotEmpty(t, current.AnalysisRunID)
	assert.NotEmpty(t, current.SnapshotID)
	assert.Zero(t, current.SnapshotSize, "empty eligible snapshot is persisted without inventing vocabulary")

	loaded, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, current, loaded)
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.ErrorIs(t, err, ErrCurrentReadingExists)
}

func TestCurrentReadingEndReleasesOnlyActiveSnapshotIdempotently(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-transitions", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "current-reading-transition")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.NotEmpty(t, reading.SnapshotID)
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID))
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID), "replaying stop is idempotent")
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
	disposition, err := store.GetBookDisposition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)
	var releasedSnapshots int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, owner.ID, reading.SnapshotID).Scan(&releasedSnapshots))
	assert.Equal(t, 1, releasedSnapshots)
	var preservedVocabulary int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, owner.ID, reading.SnapshotID).Scan(&preservedVocabulary))
	assert.Equal(t, reading.SnapshotSize, preservedVocabulary, "released snapshots remain durable")

	reading, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	rollbackFailure := errors.New("forced current-reading transition rollback")
	_, err = store.Pool().Exec(ctx, `
CREATE FUNCTION test_current_reading_transition_failure() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced current-reading transition rollback'; END; $$;
CREATE TRIGGER test_current_reading_transition_failure
BEFORE DELETE ON primary_goals FOR EACH ROW EXECUTE FUNCTION test_current_reading_transition_failure();`)
	require.NoError(t, err)
	err = store.EndCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID)
	require.ErrorContains(t, err, rollbackFailure.Error())
	current, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, current.BookID, "failed transition preserves the current reading")
	assert.Equal(t, reading.SnapshotID, current.SnapshotID)
	disposition, err = store.GetBookDisposition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition, "failed transition preserves disposition")
	releasedSnapshots = 0
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, owner.ID, reading.SnapshotID).Scan(&releasedSnapshots))
	assert.Zero(t, releasedSnapshots, "failed transition preserves active reservation")
	_, err = store.Pool().Exec(ctx, `DROP TRIGGER test_current_reading_transition_failure ON primary_goals; DROP FUNCTION test_current_reading_transition_failure()`)
	require.NoError(t, err)
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID))
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID), "replaying end is idempotent")
	current, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
	disposition, err = store.GetBookDisposition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)
	releasedSnapshots = 0
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, owner.ID, reading.SnapshotID).Scan(&releasedSnapshots))
	assert.Equal(t, 1, releasedSnapshots)
}

func TestConcurrentCurrentReadingSwitchesKeepOneWinnerAndOneReservation(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-concurrent-switch", false)
	require.NoError(t, err)
	books := make([]domain.Book, 3)
	for i, suffix := range []string{"switch-first", "switch-second", "switch-third"} {
		book, source, _ := createReadingFixture(t, ctx, store, owner.ID, suffix)
		makeAnalyzedToReadBook(t, ctx, store, book, source)
		books[i] = book
		require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	}
	initial, err := store.StartCurrentReading(ctx, owner.ID, "de", books[0].ID)
	require.NoError(t, err)
	start := make(chan struct{})
	type switchResult struct {
		bookID string
		err    error
	}
	results := make(chan switchResult, 2)
	for _, candidate := range books[1:] {
		go func() {
			<-start
			_, switchErr := store.SwitchCurrentReading(ctx, owner.ID, "de", candidate.ID, initial.BookID, initial.SnapshotID)
			results <- switchResult{bookID: candidate.ID, err: switchErr}
		}()
	}
	close(start)
	winner := ""
	staleCount := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			winner = result.bookID
			continue
		}
		require.ErrorIs(t, result.err, ErrCurrentReadingStale)
		staleCount++
	}
	require.NotEmpty(t, winner)
	assert.Equal(t, 1, staleCount)
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, winner, current.BookID)
	var currentRows int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&currentRows))
	assert.Equal(t, 1, currentRows)
	var activeSnapshots int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND released_at IS NULL`, owner.ID).Scan(&activeSnapshots))
	assert.Equal(t, 1, activeSnapshots, "only the winning current reading retains a reservation")
	var oldSnapshotReleased int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, owner.ID, initial.SnapshotID).Scan(&oldSnapshotReleased))
	assert.Equal(t, 1, oldSnapshotReleased)
}

func TestCurrentReadingReadingFinishIsGuardedPersistentAndIdempotent(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	owner, err := store.CreateUser(ctx, "goal-finish", false)
	require.NoError(t, err)
	book, bookSource, _ := createReadingFixture(t, ctx, store, owner.ID, "finish")
	replacement, replacementSource, _ := createReadingFixture(t, ctx, store, owner.ID, "finish-replacement")
	makeAnalyzedToReadBook(t, ctx, store, book, bookSource)
	makeAnalyzedToReadBook(t, ctx, store, replacement, replacementSource)
	goal, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
CREATE FUNCTION test_reading_completion_failure() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced reading completion failure'; END; $$;
CREATE TRIGGER test_reading_completion_failure
BEFORE UPDATE ON book_dispositions
FOR EACH ROW EXECUTE FUNCTION test_reading_completion_failure();`)
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.Error(t, err)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner.ID, book.ID), "failed completion changed disposition")
	var historyCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Zero(t, historyCount, "failed completion left durable history")
	goalAfterRollback, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, goalAfterRollback.BookID, "failed completion cleared Goal")
	knownAfterRollback, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, knownAfterRollback, "failed completion changed Known vocabulary")
	_, err = store.Pool().Exec(ctx, `DROP TRIGGER test_reading_completion_failure ON book_dispositions; DROP FUNCTION test_reading_completion_failure();`)
	require.NoError(t, err)

	result, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, book.ID, result.Completion.BookID, "reading completion")
	assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, owner.ID, book.ID), "completed Book did not return to Inbox")
	persisted, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, persisted.BookID, "completed Goal cleared")
	var historyBook, historyLanguage string
	var completedAt time.Time
	err = store.Pool().QueryRow(ctx, `SELECT book_id::text, language, completed_at FROM reading_history WHERE owner_id=$1`, owner.ID).Scan(&historyBook, &historyLanguage, &completedAt)
	require.NoError(t, err)
	assert.Equal(t, book.ID, historyBook)
	assert.Equal(t, "de", historyLanguage)
	assert.False(t, completedAt.IsZero())

	repeated, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, result.Completion, repeated.Completion, "idempotent finish")
	replacementGoal, err := store.StartCurrentReading(ctx, owner.ID, "de", replacement.ID)
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.ErrorIs(t, err, ErrCurrentReadingStale)
	goalAfterStale, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, replacement.ID, goalAfterStale.BookID, "stale completion changed replacement reading")
	err = store.EndCurrentReading(ctx, owner.ID, "de", replacement.ID, replacementGoal.SnapshotID)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	secondGoal, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	second, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, secondGoal.SnapshotID)
	require.NoError(t, err)
	assert.NotEqual(t, result.Completion.CompletedAt, second.Completion.CompletedAt)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 2, historyCount)
	oldRetry, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, result.Completion, oldRetry.Completion)
}

func TestCurrentReadingReadingFinishConcurrentRequestsTransitionOnce(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-finish-concurrent", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "finish-concurrent")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	var corpusID string
	err = store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&corpusID)
	require.NoError(t, err)
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
		OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: "concurrent", UPOS: "NOUN",
		OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
	})
	require.NoError(t, err)
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
		OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: "legacy-concurrent", UPOS: "VERB",
		OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
	})
	require.NoError(t, err)
	goal, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "legacy-concurrent", "VERB")
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan domain.CurrentReadingFinishResult, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			result, finishErr := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
			results <- result
			errors <- finishErr
		}()
	}
	close(start)
	for range 2 {
		require.NoError(t, <-errors)
	}
	first, second := <-results, <-results
	assert.Equal(t, first.Completion, second.Completion)

	var historyCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 1, historyCount)
	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, known, 2)
	assert.Equal(t, 2, first.Completion.SnapshotVocabularyCount)
	assert.Equal(t, 1, first.Completion.EligibleVocabularyCount)
	assert.Equal(t, 1, first.Completion.GraduatedVocabularyCount)
	assert.Equal(t, 1, first.Completion.AlreadyKnownVocabularyCount)
	persisted, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, persisted.BookID)
}

func TestEndCurrentReadingRequiresExactCommitmentAndVerifiesReplays(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "end-exact-commitment", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "end-exact")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))

	first, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.ErrorIs(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, ""), ErrCurrentReadingStale, "missing snapshot is rejected")
	require.ErrorIs(t, store.EndCurrentReading(ctx, owner.ID, "de", "", first.SnapshotID), ErrCurrentReadingStale)
	_, err = store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, "")
	require.ErrorIs(t, err, ErrCurrentReadingStale, "a snapshot-bearing reading is not finished by an empty expectation")
	_, err = store.SwitchCurrentReading(ctx, owner.ID, "de", book.ID, book.ID, "")
	require.ErrorIs(t, err, ErrCurrentReadingStale)

	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, first.SnapshotID))
	var completions int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1`, owner.ID).Scan(&completions))
	assert.Zero(t, completions, "End records no completion")

	restarted, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.NotEqual(t, first.SnapshotID, restarted.SnapshotID)
	require.ErrorIs(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, first.SnapshotID), ErrCurrentReadingStale, "stale snapshot cannot end the restarted reading")
	_, err = store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, first.SnapshotID)
	require.ErrorIs(t, err, ErrCurrentReadingStale)
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, restarted.SnapshotID, current.SnapshotID, "restarted reading is untouched")

	other, otherSource, _ := createReadingFixture(t, ctx, store, owner.ID, "end-exact-other")
	makeAnalyzedToReadBook(t, ctx, store, other, otherSource)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, other.ID, domain.BookDispositionToRead))
	_, err = store.SwitchCurrentReading(ctx, owner.ID, "de", other.ID, book.ID, first.SnapshotID)
	require.ErrorIs(t, err, ErrCurrentReadingStale)
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, restarted.SnapshotID))
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, restarted.SnapshotID), "verified replay")
	switched, err := store.StartCurrentReading(ctx, owner.ID, "de", other.ID)
	require.NoError(t, err)
	require.ErrorIs(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, restarted.SnapshotID), ErrCurrentReadingStale, "replay cannot touch newer reading")
	current, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, switched.SnapshotID, current.SnapshotID)
}

func TestCurrentReadingStartAndSwitchRejectEachIneligibilityReason(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-ineligible", false)
	require.NoError(t, err)
	anchor, anchorSource, _ := createReadingFixture(t, ctx, store, owner.ID, "ineligible-anchor")
	makeAnalyzedToReadBook(t, ctx, store, anchor, anchorSource)
	cases := ineligibleCurrentReadingCases(t, ctx, store, owner.ID)

	for _, c := range cases {
		_, err := store.StartCurrentReading(ctx, owner.ID, "de", c.book.ID)
		requireCurrentReadingRejected(t, err, c.reason, "start "+c.name)
	}
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive(), "rejected starts leave no current reading")

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", anchor.ID)
	require.NoError(t, err)
	for _, c := range cases {
		_, err := store.SwitchCurrentReading(ctx, owner.ID, "de", c.book.ID, anchor.ID, reading.SnapshotID)
		requireCurrentReadingRejected(t, err, c.reason, "switch "+c.name)
	}
	current, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, anchor.ID, current.BookID, "rejected switches keep the current reading")
	assert.Equal(t, reading.SnapshotID, current.SnapshotID, "rejected switches keep the reservation")
}

func TestCurrentReadingStartTreatsMalformedIdentitiesAsNotFound(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-malformed", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "malformed-anchor")
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
		book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "re-analysis-"+state)
		makeAnalyzedToReadBook(t, ctx, store, book, source)
		insertAnalysisRun(t, ctx, store, source, state)

		reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
		require.NoError(t, err, "start during %s re-analysis", state)
		var publishedRunID string
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&publishedRunID))
		assert.Equal(t, publishedRunID, reading.AnalysisRunID, "%s re-analysis keeps the published analysis", state)
		require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID))
	}
}

// currentReadingIneligibleCase is a Book the classifier rejects for one reason
// when it is started or switched to as the German current reading.
type currentReadingIneligibleCase struct {
	name   string
	reason domain.CurrentReadingEligibilityReason
	book   domain.Book
}

func ineligibleCurrentReadingCases(t *testing.T, ctx context.Context, store *PostgresStore, owner string) []currentReadingIneligibleCase {
	t.Helper()
	notToRead, notToReadSource, _ := createReadingFixture(t, ctx, store, owner, "case-inbox")
	makeAnalyzedToReadBook(t, ctx, store, notToRead, notToReadSource)
	require.NoError(t, store.SetBookDisposition(ctx, owner, notToRead.ID, domain.BookDispositionInbox))

	unknownLanguage, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Reading case unknown language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner, unknownLanguage.ID, domain.BookDispositionToRead))

	unacquired, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Reading case unacquired", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner, unacquired.ID, domain.BookDispositionToRead))

	italian, italianSource, _ := createReadingFixtureInLanguage(t, ctx, store, owner, "it", "case-italian")
	makeAnalyzedToReadBook(t, ctx, store, italian, italianSource)

	stale, staleSource, _ := createReadingFixture(t, ctx, store, owner, "case-stale")
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
	book, source, _ := createReadingFixture(t, ctx, store, owner, suffix)
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
