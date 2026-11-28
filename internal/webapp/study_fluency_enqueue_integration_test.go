//go:build integration

package webapp

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/study"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// riverClock is a settable River time source, so that two enqueues can fall in
// different clock-aligned hours.
type riverClock struct{ now time.Time }

func (c *riverClock) Now() time.Time       { return c.now }
func (c *riverClock) NowOrNil() *time.Time { return &c.now }

// fluencyJobRows counts every Fluency preparation job, in any state.
func (s *studyHarness) fluencyJobRows() int {
	s.t.Helper()
	var n int
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM river_job WHERE kind='study_prepare_fluency'`).Scan(&n))
	return n
}

// hourlyEnqueuer stops the workers, clears the Fluency jobs, and returns a
// function that opens Today's enqueue at the given time on an insert-only
// client, so jobs stay exactly where the enqueue put them.
func (s *studyHarness) hourlyEnqueuer() func(at time.Time) {
	s.t.Helper()
	require.NoError(s.t, s.client.Stop(s.ctx))
	_, err := s.store.Pool().Exec(s.ctx, `DELETE FROM river_job WHERE kind='study_prepare_fluency'`)
	require.NoError(s.t, err)
	clock := &riverClock{}
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(s.store.Pool()), &river.Config{Test: river.TestConfig{Time: clock}})
	require.NoError(s.t, err)
	service := study.NewService(s.store, client)
	return func(at time.Time) {
		s.t.Helper()
		clock.now = at
		require.NoError(s.t, service.EnqueueFluencyPreparation(s.ctx, s.owner.ID, "de"))
	}
}

func TestStudyFluencyEnqueueAddsNoDuplicateAcrossAnHourWhileAJobIsLive(t *testing.T) {
	for _, state := range []string{"available", "pending", "scheduled", "retryable", "running"} {
		t.Run(state, func(t *testing.T) {
			s := newStudyHarness(t)
			s.matureHaus()
			enqueue := s.hourlyEnqueuer()

			enqueue(time.Date(2026, 11, 25, 10, 57, 0, 0, time.UTC))
			require.Equal(t, 1, s.fluencyJobRows())
			_, err := s.store.Pool().Exec(s.ctx, `UPDATE river_job SET state = $1::river_job_state WHERE kind='study_prepare_fluency'`, state)
			require.NoError(t, err)
			enqueue(time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC))

			assert.Equal(t, 1, s.fluencyJobRows(), "a word with a live job from the previous hour is not enqueued again")
		})
	}
}

func TestStudyFluencyEnqueueWaitsForRetentionAfterAJobCompletes(t *testing.T) {
	s := newStudyHarness(t)
	s.matureHaus()
	enqueue := s.hourlyEnqueuer()

	enqueue(time.Date(2026, 11, 25, 10, 57, 0, 0, time.UTC))
	_, err := s.store.Pool().Exec(s.ctx, `UPDATE river_job SET state='completed', finalized_at = now() WHERE kind='study_prepare_fluency'`)
	require.NoError(t, err)
	enqueue(time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC))
	assert.Equal(t, 1, s.fluencyJobRows(), "a word whose job just finished is not scanned again")

	// River deletes completed jobs after its retention period; the word may then be asked for again.
	_, err = s.store.Pool().Exec(s.ctx, `DELETE FROM river_job WHERE kind='study_prepare_fluency'`)
	require.NoError(t, err)
	enqueue(time.Date(2026, 11, 26, 11, 0, 0, 0, time.UTC))
	assert.Equal(t, 1, s.fluencyJobRows(), "the word is enqueued once its finished job is gone")
}
