//go:build integration

package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	currentReadingRaceLemma   = "race-lemma"
	currentReadingRaceTimeout = 30 * time.Second
)

// raceCurrentReadingTransitions releases every transition at once and returns
// their errors in argument order. The race, not the fixture setup, is bounded
// so a transition that blocks forever fails the test instead of hanging the
// suite.
func raceCurrentReadingTransitions(t *testing.T, ctx context.Context, transitions ...func(context.Context) error) []error {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, currentReadingRaceTimeout)
	defer cancel()
	start := make(chan struct{})
	errs := make([]error, len(transitions))
	var wait sync.WaitGroup
	wait.Add(len(transitions))
	for i, transition := range transitions {
		go func() {
			defer wait.Done()
			<-start
			errs[i] = transition(ctx)
		}()
	}
	close(start)
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		require.FailNow(t, "concurrent current-reading transitions did not finish", ctx.Err().Error())
	}
	return errs
}

// requireOneRaceWinner asserts exactly one transition succeeded and returns its
// index. Every loser must fail with one of the allowed errors.
func requireOneRaceWinner(t *testing.T, errs []error, allowedLoserErrs ...error) int {
	t.Helper()
	winner := -1
	for i, err := range errs {
		if err == nil {
			require.Equal(t, -1, winner, "more than one transition won: %v", errs)
			winner = i
		}
	}
	require.NotEqual(t, -1, winner, "no transition won: %v", errs)
	for i, err := range errs {
		if i == winner {
			continue
		}
		matched := false
		for _, allowed := range allowedLoserErrs {
			matched = matched || errors.Is(err, allowed)
		}
		require.True(t, matched, "loser %d failed with an unexpected error: %v", i, err)
	}
	return winner
}

// requireCurrentReadingStateConsistent asserts the language holds at most one
// Current reading and that its active reservation set is exactly the current
// reading's snapshot.
func requireCurrentReadingStateConsistent(t *testing.T, ctx context.Context, store *PostgresStore, owner, language string) domain.CurrentReading {
	t.Helper()
	var currentRows int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1 AND language=$2`, owner, language).Scan(&currentRows))
	require.LessOrEqual(t, currentRows, 1, "more than one Current reading for the language")
	var activeSnapshots int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND language=$2 AND released_at IS NULL`, owner, language).Scan(&activeSnapshots))
	require.Equal(t, currentRows, activeSnapshots, "active reservation sets must match the Current reading")

	current, err := store.GetCurrentReading(ctx, owner, language)
	require.NoError(t, err)
	if currentRows == 1 {
		var currentSnapshotActive int
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NULL`, owner, current.SnapshotID).Scan(&currentSnapshotActive))
		require.Equal(t, 1, currentSnapshotActive, "Current reading names a released snapshot")
	}
	return current
}

// requireFinishAtomicity asserts a completion fact and Known acceptance exist
// together or not at all.
func requireFinishAtomicity(t *testing.T, ctx context.Context, store *PostgresStore, owner, bookID string, finished bool) {
	t.Helper()
	var completions int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner, bookID).Scan(&completions))
	known, err := store.ListKnownVocabulary(ctx, owner, "de")
	require.NoError(t, err)
	knownLemmas := make([]string, 0, len(known))
	for _, entry := range known {
		knownLemmas = append(knownLemmas, entry.CanonicalLemma)
	}
	if finished {
		assert.Equal(t, 1, completions, "Finish recorded exactly one completion")
		assert.Equal(t, []string{currentReadingRaceLemma}, knownLemmas, "completion without Known acceptance")
		assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, owner, bookID), "completed Book returns to Inbox")
		return
	}
	assert.Zero(t, completions, "transition other than Finish recorded a completion")
	assert.Empty(t, knownLemmas, "Known acceptance without a completion")
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner, bookID), "unfinished Book keeps its disposition")
}

func requireSnapshotReleased(t *testing.T, ctx context.Context, store *PostgresStore, owner, snapshotID string, released bool) {
	t.Helper()
	var count int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, owner, snapshotID).Scan(&count))
	want := 0
	if released {
		want = 1
	}
	assert.Equal(t, want, count, "snapshot %s released=%t", snapshotID, released)
}

type currentReadingRaceFixture struct {
	owner string
	books []domain.Book
}

// newCurrentReadingRaceFixture creates bookCount analyzed To Read German Books.
// The first Book's corpus carries one candidate so its snapshot has vocabulary
// that Finish must accept as Known.
func newCurrentReadingRaceFixture(t *testing.T, ctx context.Context, store *PostgresStore, name string, bookCount int) currentReadingRaceFixture {
	t.Helper()
	owner, err := store.CreateUser(ctx, name, false)
	require.NoError(t, err)
	fixture := currentReadingRaceFixture{owner: owner.ID}
	for i := range bookCount {
		suffix := name + "-" + string(rune('a'+i))
		book, source, _ := createReadingFixture(t, ctx, store, owner.ID, suffix)
		makeAnalyzedToReadBook(t, ctx, store, book, source)
		fixture.books = append(fixture.books, book)
	}
	var corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, fixture.books[0].ID).Scan(&corpusID))
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
		OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: currentReadingRaceLemma, UPOS: "NOUN",
		OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
	})
	require.NoError(t, err)
	return fixture
}

func TestConcurrentCurrentReadingStartsOnDifferentBooksKeepOneWinner(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	fixture := newCurrentReadingRaceFixture(t, ctx, store, "race-start-different", 2)

	errs := raceCurrentReadingTransitions(t, ctx,
		func(ctx context.Context) error {
			_, err := store.StartCurrentReading(ctx, fixture.owner, "de", fixture.books[0].ID)
			return err
		},
		func(ctx context.Context) error {
			_, err := store.StartCurrentReading(ctx, fixture.owner, "de", fixture.books[1].ID)
			return err
		},
	)
	winner := requireOneRaceWinner(t, errs, ErrCurrentReadingExists)

	current := requireCurrentReadingStateConsistent(t, ctx, store, fixture.owner, "de")
	assert.Equal(t, fixture.books[winner].ID, current.BookID, "the winning Start owns the Current reading")
	var snapshots int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1`, fixture.owner).Scan(&snapshots))
	assert.Equal(t, 1, snapshots, "the losing Start leaves no snapshot behind")
}

func TestConcurrentCurrentReadingStartsOnSameBookKeepOneWinner(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	fixture := newCurrentReadingRaceFixture(t, ctx, store, "race-start-same", 1)

	var winnerSnapshots sync.Map
	start := func(slot int) func(context.Context) error {
		return func(ctx context.Context) error {
			reading, err := store.StartCurrentReading(ctx, fixture.owner, "de", fixture.books[0].ID)
			if err == nil {
				winnerSnapshots.Store(slot, reading.SnapshotID)
			}
			return err
		}
	}
	errs := raceCurrentReadingTransitions(t, ctx, start(0), start(1))
	winner := requireOneRaceWinner(t, errs, ErrCurrentReadingExists)

	current := requireCurrentReadingStateConsistent(t, ctx, store, fixture.owner, "de")
	snapshotID, ok := winnerSnapshots.Load(winner)
	require.True(t, ok)
	assert.Equal(t, snapshotID, current.SnapshotID)
	assert.Equal(t, fixture.books[0].ID, current.BookID)
	var snapshots int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1`, fixture.owner).Scan(&snapshots))
	assert.Equal(t, 1, snapshots, "the losing Start leaves no snapshot behind")
}

func TestConcurrentCurrentReadingSwitchAndFinishKeepOneWinner(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	fixture := newCurrentReadingRaceFixture(t, ctx, store, "race-switch-finish", 2)
	initial, err := store.StartCurrentReading(ctx, fixture.owner, "de", fixture.books[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, initial.SnapshotID)

	const switchIndex, finishIndex = 0, 1
	errs := raceCurrentReadingTransitions(t, ctx,
		func(ctx context.Context) error {
			_, err := store.SwitchCurrentReading(ctx, fixture.owner, "de", fixture.books[1].ID, initial.BookID, initial.SnapshotID)
			return err
		},
		func(ctx context.Context) error {
			_, err := store.FinishCurrentReading(ctx, fixture.owner, "de", initial.BookID, initial.SnapshotID)
			return err
		},
	)
	// A Switch that loses finds no Current reading; a Finish that loses finds a
	// different one.
	winner := requireOneRaceWinner(t, errs, ErrCurrentReadingStale, ErrNotFound)

	current := requireCurrentReadingStateConsistent(t, ctx, store, fixture.owner, "de")
	requireSnapshotReleased(t, ctx, store, fixture.owner, initial.SnapshotID, true)
	switch winner {
	case switchIndex:
		require.ErrorIs(t, errs[finishIndex], ErrCurrentReadingStale)
		assert.Equal(t, fixture.books[1].ID, current.BookID, "Switch made the target Current")
		requireFinishAtomicity(t, ctx, store, fixture.owner, initial.BookID, false)
	case finishIndex:
		require.ErrorIs(t, errs[switchIndex], ErrNotFound)
		assert.Empty(t, current.BookID, "Finish left no Current reading")
		requireFinishAtomicity(t, ctx, store, fixture.owner, initial.BookID, true)
		assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, fixture.owner, fixture.books[1].ID), "losing Switch target keeps its disposition")
	}
}

func TestConcurrentCurrentReadingEndAndFinishKeepOneWinner(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	fixture := newCurrentReadingRaceFixture(t, ctx, store, "race-end-finish", 1)
	initial, err := store.StartCurrentReading(ctx, fixture.owner, "de", fixture.books[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, initial.SnapshotID)

	const endIndex, finishIndex = 0, 1
	errs := raceCurrentReadingTransitions(t, ctx,
		func(ctx context.Context) error {
			return store.EndCurrentReading(ctx, fixture.owner, "de", initial.BookID, initial.SnapshotID)
		},
		func(ctx context.Context) error {
			_, err := store.FinishCurrentReading(ctx, fixture.owner, "de", initial.BookID, initial.SnapshotID)
			return err
		},
	)
	// Neither transition replays the other: a completed snapshot is stale to End,
	// and an ended one has no completion for Finish to replay.
	winner := requireOneRaceWinner(t, errs, ErrCurrentReadingStale, ErrNotFound)

	current := requireCurrentReadingStateConsistent(t, ctx, store, fixture.owner, "de")
	assert.Empty(t, current.BookID, "the winning transition ended the Current reading")
	requireSnapshotReleased(t, ctx, store, fixture.owner, initial.SnapshotID, true)
	switch winner {
	case endIndex:
		require.ErrorIs(t, errs[finishIndex], ErrNotFound)
		requireFinishAtomicity(t, ctx, store, fixture.owner, initial.BookID, false)
	case finishIndex:
		require.ErrorIs(t, errs[endIndex], ErrCurrentReadingStale)
		requireFinishAtomicity(t, ctx, store, fixture.owner, initial.BookID, true)
	}
}
