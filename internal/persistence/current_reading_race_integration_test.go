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
// Current reading and that no vocabulary snapshot exists.
func requireCurrentReadingStateConsistent(t *testing.T, ctx context.Context, store *PostgresStore, owner string) domain.CurrentReading {
	t.Helper()
	var currentRows, snapshots int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1 AND language='de'`, owner).Scan(&currentRows))
	require.LessOrEqual(t, currentRows, 1, "more than one Current reading for the language")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1`, owner).Scan(&snapshots))
	require.Zero(t, snapshots, "a transition wrote a vocabulary snapshot")
	current, err := store.GetCurrentReading(ctx, owner, "de")
	require.NoError(t, err)
	return current
}

// requireCompletions asserts how many completions Reading history holds for the
// Book, and that the Book's disposition is the one Finish or End leaves.
func requireCompletions(t *testing.T, ctx context.Context, store *PostgresStore, owner, bookID string, want int) {
	t.Helper()
	var completions int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner, bookID).Scan(&completions))
	assert.Equal(t, want, completions, "completions recorded")
	disposition := domain.BookDispositionToRead
	if want > 0 {
		disposition = domain.BookDispositionInbox
	}
	assert.Equal(t, disposition, mustBookDisposition(t, store, owner, bookID))
}

type currentReadingRaceFixture struct {
	owner string
	books []domain.Book
}

// newCurrentReadingRaceFixture creates bookCount analyzed To Read German Books.
func newCurrentReadingRaceFixture(t *testing.T, ctx context.Context, store *PostgresStore, name string, bookCount int) currentReadingRaceFixture {
	t.Helper()
	owner, err := store.CreateUser(ctx, name, false)
	require.NoError(t, err)
	fixture := currentReadingRaceFixture{owner: owner.ID}
	for i := range bookCount {
		suffix := name + "-" + string(rune('a'+i))
		book, source := createReadingFixture(t, ctx, store, owner.ID, suffix)
		makeAnalyzedToReadBook(t, ctx, store, book, source)
		fixture.books = append(fixture.books, book)
	}
	return fixture
}

func TestConcurrentCurrentReadingStartsOnDifferentBooksLeaveOneReading(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	fixture := newCurrentReadingRaceFixture(t, ctx, store, "race-start-different", 2)

	// Read next replaces, so both succeed one after the other; the learner lock
	// leaves exactly one reading and the loser's Book stays To Read.
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
	for slot, err := range errs {
		require.NoError(t, err, "Read next %d", slot)
	}
	current := requireCurrentReadingStateConsistent(t, ctx, store, fixture.owner)
	assert.Contains(t, []string{fixture.books[0].ID, fixture.books[1].ID}, current.BookID)
	for _, book := range fixture.books {
		assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, fixture.owner, book.ID))
	}
}

func TestConcurrentCurrentReadingStartsOnSameBookKeepOneWinner(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	fixture := newCurrentReadingRaceFixture(t, ctx, store, "race-start-same", 1)

	// The loser of a same-Book race finds the winner's reading under the
	// learner lock and replays it: both succeed, one writes.
	var results sync.Map
	start := func(slot int) func(context.Context) error {
		return func(ctx context.Context) error {
			result, err := store.StartCurrentReadingResult(ctx, fixture.owner, "de", fixture.books[0].ID)
			if err == nil {
				results.Store(slot, result)
			}
			return err
		}
	}
	errs := raceCurrentReadingTransitions(t, ctx, start(0), start(1))
	for slot, err := range errs {
		require.NoError(t, err, "same-Book Read next %d", slot)
	}
	current := requireCurrentReadingStateConsistent(t, ctx, store, fixture.owner)
	assert.Equal(t, fixture.books[0].ID, current.BookID)
	fresh := 0
	for slot := range errs {
		loaded, ok := results.Load(slot)
		require.True(t, ok)
		result, ok := loaded.(StartResult)
		require.True(t, ok)
		if !result.Replayed {
			fresh++
		}
	}
	assert.Equal(t, 1, fresh, "exactly one Read next writes the reading; the other replays it")
}

func TestConcurrentCurrentReadingEndAndFinishKeepOneWinner(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	fixture := newCurrentReadingRaceFixture(t, ctx, store, "race-end-finish", 1)
	initial, err := store.StartCurrentReading(ctx, fixture.owner, "de", fixture.books[0].ID)
	require.NoError(t, err)

	const endIndex, finishIndex = 0, 1
	errs := raceCurrentReadingTransitions(t, ctx,
		func(ctx context.Context) error {
			return store.EndCurrentReading(ctx, fixture.owner, "de")
		},
		func(ctx context.Context) error {
			_, err := store.FinishCurrentReading(ctx, fixture.owner, "de")
			return err
		},
	)
	// The loser finds no current reading.
	winner := requireOneRaceWinner(t, errs, ErrNotFound)

	current := requireCurrentReadingStateConsistent(t, ctx, store, fixture.owner)
	assert.Empty(t, current.BookID, "the winning transition ended the Current reading")
	switch winner {
	case endIndex:
		requireCompletions(t, ctx, store, fixture.owner, initial.BookID, 0)
	case finishIndex:
		requireCompletions(t, ctx, store, fixture.owner, initial.BookID, 1)
	}
}

func TestConcurrentCurrentReadingFinishesRecordOneCompletion(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	fixture := newCurrentReadingRaceFixture(t, ctx, store, "race-finish-finish", 1)
	_, err := store.StartCurrentReading(ctx, fixture.owner, "de", fixture.books[0].ID)
	require.NoError(t, err)

	finish := func(ctx context.Context) error {
		_, err := store.FinishCurrentReading(ctx, fixture.owner, "de")
		return err
	}
	errs := raceCurrentReadingTransitions(t, ctx, finish, func(ctx context.Context) error { return finish(ctx) })
	requireOneRaceWinner(t, errs, ErrNotFound)
	requireCompletions(t, ctx, store, fixture.owner, fixture.books[0].ID, 1)
}
