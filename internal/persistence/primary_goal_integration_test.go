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

func TestPrimaryGoalPersistence(t *testing.T) {
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

	aliceBook, _, _ := createJourneyFixture(t, ctx, store, alice.ID, "goal-active")
	bobBook, _, _ := createJourneyFixture(t, ctx, store, bob.ID, "goal-queued")
	_, _, _ = createJourneyFixture(t, ctx, store, dave.ID, "goal-complete")
	_, _, _ = createJourneyFixture(t, ctx, store, erin.ID, "goal-abandoned")

	goal, err := store.GetPrimaryGoal(ctx, carol.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.PrimaryGoal{}, goal, "owner without a Goal")
	// Model a legacy goal directly against the baseline. The historical
	// migration that added language is retired, but current goal behavior still
	// needs coverage for persisted language-scoped rows.
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, alice.ID, aliceBook.ID)
	require.NoError(t, err)

	goal, err = store.GetPrimaryGoal(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, alice.ID, goal.OwnerID, "migrated active goal")
	assert.Equal(t, aliceBook.ID, goal.BookID, "migrated active goal")
	italianBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice Italian goal book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, alice.ID, "it", italianBook.ID)
	require.NoError(t, err)
	italianGoal, getErr := store.GetPrimaryGoal(ctx, alice.ID, "it")
	require.NoError(t, getErr)
	assert.Equal(t, italianBook.ID, italianGoal.BookID, "parallel Italian goal")
	err = store.ClearPrimaryGoal(ctx, alice.ID, "it", italianBook.ID)
	require.NoError(t, err, "clear parallel Italian goal")
	germanGoal, getErr := store.GetPrimaryGoal(ctx, alice.ID, "de")
	require.NoError(t, getErr)
	assert.Equal(t, aliceBook.ID, germanGoal.BookID, "German goal after Italian clear")
	for _, owner := range []string{bob.ID, carol.ID, dave.ID, erin.ID} {
		goal, err := store.GetPrimaryGoal(ctx, owner, "de")
		require.NoError(t, err)
		assert.Equal(t, domain.PrimaryGoal{}, goal, "non-active owner=%s", owner)
	}

	otherAliceBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice second goal book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, alice.ID, "de", otherAliceBook.ID)
	assert.ErrorIs(t, err, ErrGoalExists) //nolint:testifylint // Duplicate-goal rejection is independent of later eligibility cases.

	noDeckBook, err := store.CreateBook(ctx, domain.Book{OwnerID: carol.ID, Title: "Carol reading-only book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, carol.ID, "de", noDeckBook.ID)
	assert.ErrorIs(t, err, ErrGoalIneligible) //nolint:testifylint // Eligibility rejection is an independent goal case.
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, carol.ID, "de", noDeckBook.ID)
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, carol.ID, "de", bobBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner goal rejection is independently asserted.

	changed, err := store.ChangePrimaryGoal(ctx, carol.ID, "de", otherAliceBook.ID, noDeckBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Stale-book change rejection is independent of the valid replacement.
	assert.Equal(t, domain.PrimaryGoal{}, changed, "cross-owner change goal")
	replacementBook, replacementSource, _ := createJourneyFixture(t, ctx, store, carol.ID, "goal-replacement")
	makeJourneyMemberAnalyzed(t, ctx, store, replacementBook, replacementSource)
	_, err = store.ChangePrimaryGoal(ctx, carol.ID, "de", replacementBook.ID, "stale-book")
	assert.ErrorIs(t, err, ErrGoalStale) //nolint:testifylint // Stale-book change rejection is independent of the valid replacement.
	changed, err = store.ChangePrimaryGoal(ctx, carol.ID, "de", replacementBook.ID, noDeckBook.ID)
	require.NoError(t, err)
	assert.Equal(t, replacementBook.ID, changed.BookID, "valid change goal")
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", noDeckBook.ID)
	assert.ErrorIs(t, err, ErrGoalStale) //nolint:testifylint // Stale clear rejection is independent of the valid clear.
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", replacementBook.ID)
	require.NoError(t, err, "valid clear")
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", replacementBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Repeated clear rejection is an independent idempotency case.
	err = store.ClearPrimaryGoal(ctx, bob.ID, "de", aliceBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner clear rejection is independent of the final owner lookup.
	goal, err = store.GetPrimaryGoal(ctx, bob.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.PrimaryGoal{}, goal, "cross-owner get goal")

}

func TestCurrentReadingPersistenceInterfacePreservesLifecycleGuards(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "current-reading-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "current-reading-bob", false)
	require.NoError(t, err)
	first, firstSource, _ := createJourneyFixture(t, ctx, store, alice.ID, "current-reading-first")
	second, secondSource, _ := createJourneyFixture(t, ctx, store, alice.ID, "current-reading-second")
	makeJourneyMemberAnalyzed(t, ctx, store, first, firstSource)
	makeJourneyMemberAnalyzed(t, ctx, store, second, secondSource)
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", first.ID, journey.Revision)
	require.NoError(t, err)
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", second.ID, journey.Revision)
	require.NoError(t, err)

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
	require.ErrorIs(t, store.StopCurrentReading(ctx, alice.ID, "de", first.ID, reading.SnapshotID), ErrCurrentReadingStale)
	require.NoError(t, store.StopCurrentReading(ctx, alice.ID, "de", second.ID, reading.SnapshotID))

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

func TestCurrentReadingCanStartAnalyzedToReadBookOutsideJourney(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-to-read", false)
	require.NoError(t, err)
	book, source, _ := createJourneyFixture(t, ctx, store, owner.ID, "to-read-only")
	makeJourneyMemberAnalyzed(t, ctx, store, book, source)
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	queueFailure := errors.New("prepared deck queue unavailable")
	_, err = store.CreatePrimaryGoalWith(ctx, owner.ID, "de", book.ID, func(context.Context, pgx.Tx, domain.PrimaryGoal) error {
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
	require.ErrorIs(t, err, ErrGoalExists)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, journey.Entries, "starting a To Read candidate does not require or create Journey membership")
}

func TestCurrentReadingStopAndSetAsideReleaseOnlyActiveSnapshotIdempotently(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "current-reading-transitions", false)
	require.NoError(t, err)
	book, source, _ := createJourneyFixture(t, ctx, store, owner.ID, "current-reading-transition")
	makeJourneyMemberAnalyzed(t, ctx, store, book, source)
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.NotEmpty(t, reading.SnapshotID)
	require.NoError(t, store.StopCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID))
	require.NoError(t, store.StopCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID), "replaying stop is idempotent")
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
	err = store.SetAsideCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID)
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
	require.NoError(t, store.SetAsideCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID))
	require.NoError(t, store.SetAsideCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID), "replaying set aside is idempotent")
	current, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
	disposition, err = store.GetBookDisposition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, disposition)
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
		book, source, _ := createJourneyFixture(t, ctx, store, owner.ID, suffix)
		makeJourneyMemberAnalyzed(t, ctx, store, book, source)
		books[i] = book
		journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
		require.NoError(t, err)
		_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
		require.NoError(t, err)
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

func TestPrimaryGoalReadingFinishIsGuardedPersistentAndIdempotent(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	owner, err := store.CreateUser(ctx, "goal-finish", false)
	require.NoError(t, err)
	book, bookSource, _ := createJourneyFixture(t, ctx, store, owner.ID, "finish")
	replacement, replacementSource, _ := createJourneyFixture(t, ctx, store, owner.ID, "finish-replacement")
	makeJourneyMemberAnalyzed(t, ctx, store, book, bookSource)
	makeJourneyMemberAnalyzed(t, ctx, store, replacement, replacementSource)
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", replacement.ID, journey.Revision)
	require.NoError(t, err)
	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
CREATE FUNCTION test_reading_completion_failure() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced reading completion failure'; END; $$;
CREATE TRIGGER test_reading_completion_failure
BEFORE DELETE ON reading_journey_membership
FOR EACH ROW EXECUTE FUNCTION test_reading_completion_failure();`)
	require.NoError(t, err)
	_, err = store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.Error(t, err)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner.ID, book.ID), "failed completion changed disposition")
	var historyCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Zero(t, historyCount, "failed completion left durable history")
	goalAfterRollback, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, goalAfterRollback.BookID, "failed completion cleared Goal")
	journeyAfterRollback, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Contains(t, journeyBookIDs(journeyAfterRollback), book.ID, "failed completion removed Book from Journey")
	knownAfterRollback, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, knownAfterRollback, "failed completion changed Known vocabulary")
	_, err = store.Pool().Exec(ctx, `DROP TRIGGER test_reading_completion_failure ON reading_journey_membership; DROP FUNCTION test_reading_completion_failure();`)
	require.NoError(t, err)

	result, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, book.ID, result.Completion.BookID, "reading completion")
	assert.Equal(t, domain.BookDispositionSetAside, mustBookDisposition(t, store, owner.ID, book.ID), "completed Book was not set aside")
	persisted, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, persisted.BookID, "completed Goal cleared")
	var historyBook, historyLanguage string
	var completedAt time.Time
	err = store.Pool().QueryRow(ctx, `SELECT book_id::text, language, completed_at FROM reading_history WHERE owner_id=$1`, owner.ID).Scan(&historyBook, &historyLanguage, &completedAt)
	require.NoError(t, err)
	assert.Equal(t, book.ID, historyBook)
	assert.Equal(t, "de", historyLanguage)
	assert.False(t, completedAt.IsZero())
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.NotContains(t, journeyBookIDs(journey), book.ID, "completed Book removed from Journey")

	repeated, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, result.Completion, repeated.Completion, "idempotent finish")
	_, err = store.CreatePrimaryGoal(ctx, owner.ID, "de", replacement.ID)
	require.NoError(t, err)
	_, err = store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.ErrorIs(t, err, ErrGoalStale)
	goalAfterStale, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, replacement.ID, goalAfterStale.BookID, "stale completion changed replacement Goal")
	err = store.ClearPrimaryGoal(ctx, owner.ID, "de", replacement.ID)
	require.NoError(t, err)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	secondGoal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	second, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, secondGoal.SnapshotID)
	require.NoError(t, err)
	assert.NotEqual(t, result.Completion.CompletedAt, second.Completion.CompletedAt)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 2, historyCount)
	oldRetry, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, result.Completion, oldRetry.Completion)
}

func TestPrimaryGoalReadingFinishConcurrentRequestsTransitionOnce(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-finish-concurrent", false)
	require.NoError(t, err)
	book, source, _ := createJourneyFixture(t, ctx, store, owner.ID, "finish-concurrent")
	makeJourneyMemberAnalyzed(t, ctx, store, book, source)
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
	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "legacy-concurrent", "VERB")
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan ReadingFinishResult, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			result, finishErr := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
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
	persisted, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, persisted.BookID)
}

func journeyBookIDs(journey domain.ReadingJourney) []string {
	ids := make([]string, 0, len(journey.Entries))
	for _, entry := range journey.Entries {
		ids = append(ids, entry.BookID)
	}
	return ids
}
