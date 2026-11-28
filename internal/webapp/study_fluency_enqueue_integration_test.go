//go:build integration

package webapp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fluencyJobRows counts every Fluency preparation job of the word, in any state.
func (s *studyHarness) fluencyJobRows() int {
	s.t.Helper()
	var n int
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM river_job WHERE kind='study_prepare_fluency'`).Scan(&n))
	return n
}

// parkedFluencyJob leaves one preparation job for the word in the given
// state, created two hours ago so that it falls in an earlier hour than now.
func (s *studyHarness) parkedFluencyJob(state string) {
	s.t.Helper()
	// With the workers stopped, the job stays where the enqueue put it.
	require.NoError(s.t, s.client.Stop(s.ctx))
	_, err := s.store.Pool().Exec(s.ctx, `DELETE FROM river_job WHERE kind='study_prepare_fluency'`)
	require.NoError(s.t, err)
	require.NoError(s.t, s.planner.EnqueueFluencyPreparation(s.ctx, s.owner.ID, "de"))
	_, err = s.store.Pool().Exec(s.ctx, `UPDATE river_job SET created_at = now() - interval '2 hours', state = $1::river_job_state, finalized_at = CASE WHEN $1 = 'completed' THEN now() - interval '2 hours' END WHERE kind='study_prepare_fluency'`, state)
	require.NoError(s.t, err)
	require.Equal(s.t, 1, s.fluencyJobRows())
}

func TestStudyFluencyEnqueueAddsNoDuplicateAcrossAnHourWhileAJobIsLive(t *testing.T) {
	for _, state := range []string{"available", "pending", "scheduled", "retryable", "running"} {
		t.Run(state, func(t *testing.T) {
			s := newStudyHarness(t)
			s.matureHaus()
			s.parkedFluencyJob(state)

			require.NoError(t, s.planner.EnqueueFluencyPreparation(s.ctx, s.owner.ID, "de"))
			require.NoError(t, s.planner.EnqueueFluencyPreparation(s.ctx, s.owner.ID, "de"))

			assert.Equal(t, 1, s.fluencyJobRows(), "a word with a live job from an earlier hour is not enqueued again")
		})
	}
}

func TestStudyFluencyEnqueueWaitsForRetentionAfterAJobCompletes(t *testing.T) {
	s := newStudyHarness(t)
	s.matureHaus()
	s.parkedFluencyJob("completed")

	require.NoError(t, s.planner.EnqueueFluencyPreparation(s.ctx, s.owner.ID, "de"))
	assert.Equal(t, 1, s.fluencyJobRows(), "a word whose job just finished is not scanned again")

	// River deletes completed jobs after its retention period; the word may then be asked for again.
	_, err := s.store.Pool().Exec(s.ctx, `DELETE FROM river_job WHERE kind='study_prepare_fluency'`)
	require.NoError(t, err)
	require.NoError(t, s.planner.EnqueueFluencyPreparation(s.ctx, s.owner.ID, "de"))
	assert.Equal(t, 1, s.fluencyJobRows(), "the word is enqueued once its finished job is gone")
}
