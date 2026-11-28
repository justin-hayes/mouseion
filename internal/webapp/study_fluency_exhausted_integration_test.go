//go:build integration

package webapp

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enqueuedFluencyJobs enqueues Fluency preparation and counts the jobs it made,
// then clears them so the next enqueue is not held by uniqueness.
func (s *studyHarness) enqueuedFluencyJobs() int {
	s.t.Helper()
	require.NoError(s.t, s.planner.EnqueueFluencyPreparation(s.ctx, s.owner.ID, "de"))
	var n int
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM river_job WHERE kind='study_prepare_fluency'`).Scan(&n))
	_, err := s.store.Pool().Exec(s.ctx, `DELETE FROM river_job WHERE kind='study_prepare_fluency'`)
	require.NoError(s.t, err)
	return n
}

func TestStudyFluencyEnqueueSkipsAnExhaustedWordUntilItsCorpusChangesOrItLapses(t *testing.T) {
	s := newStudyHarness(t)
	word := s.matureHaus()
	// With the workers stopped, nothing prepares the word behind the test.
	require.NoError(t, s.client.Stop(s.ctx))
	_, err := s.store.Pool().Exec(s.ctx, `DELETE FROM study_fluency_context WHERE owner_id=$1`, s.owner.ID)
	require.NoError(t, err)
	s.enqueuedFluencyJobs()
	require.Equal(t, 1, s.enqueuedFluencyJobs(), "a word with fewer than five contexts and no scan is queued")

	scanned := time.Now().Add(time.Minute)
	require.NoError(t, s.store.RecordStudyFluencyExhausted(s.ctx, s.owner.ID, word.ID, scanned))
	assert.Zero(t, s.enqueuedFluencyJobs(), "an exhausted word is not queued again")
	assert.Zero(t, s.enqueuedFluencyJobs(), "nor on any later visit")

	t.Run("a Book added to My Books", func(t *testing.T) {
		_, err := s.store.Pool().Exec(s.ctx, `UPDATE book_membership SET activated_at=$2 WHERE owner_id=$1`, s.owner.ID, scanned.Add(time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 1, s.enqueuedFluencyJobs())
	})

	scanned = scanned.Add(2 * time.Minute)
	require.NoError(t, s.store.RecordStudyFluencyExhausted(s.ctx, s.owner.ID, word.ID, scanned))
	require.Zero(t, s.enqueuedFluencyJobs(), "a newer scan stands again")

	t.Run("a completed analysis run", func(t *testing.T) {
		_, err := s.store.Pool().Exec(s.ctx, `UPDATE analysis_runs SET completed_at=$2 WHERE owner_id=$1 AND state='completed'`, s.owner.ID, scanned.Add(time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 1, s.enqueuedFluencyJobs())
	})

	scanned = scanned.Add(2 * time.Minute)
	require.NoError(t, s.store.RecordStudyFluencyExhausted(s.ctx, s.owner.ID, word.ID, scanned))
	require.Zero(t, s.enqueuedFluencyJobs())

	t.Run("a Book removed from My Books", func(t *testing.T) {
		_, err := s.store.Pool().Exec(s.ctx, `UPDATE book_membership SET state='removed', removed_at=$2, activated_at=NULL WHERE owner_id=$1`, s.owner.ID, scanned.Add(time.Minute))
		require.NoError(t, err)
		assert.Equal(t, 1, s.enqueuedFluencyJobs())
	})

	scanned = scanned.Add(2 * time.Minute)
	require.NoError(t, s.store.RecordStudyFluencyExhausted(s.ctx, s.owner.ID, word.ID, scanned))
	require.Zero(t, s.enqueuedFluencyJobs())

	t.Run("a lapse", func(t *testing.T) {
		_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO study_review_log (owner_id, progress_id, material_id, grade, reviewed_at, study_day, state_before, state_after)
			SELECT owner_id, id, material_id, 'again', $2::timestamptz, ($2::timestamptz)::date, '{}', '{}' FROM study_progress WHERE owner_id=$1 AND id=$3`,
			s.owner.ID, scanned.Add(time.Minute), word.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, s.enqueuedFluencyJobs())
	})
}

func TestStudyFluencyExhaustedRecordIsKeptPerWordAndRefused(t *testing.T) {
	s := newStudyHarness(t)
	word := s.matureHaus()
	require.NoError(t, s.client.Stop(s.ctx))
	first := time.Now().Add(-time.Hour)
	require.NoError(t, s.store.RecordStudyFluencyExhausted(s.ctx, s.owner.ID, word.ID, first))
	require.NoError(t, s.store.RecordStudyFluencyExhausted(s.ctx, s.owner.ID, word.ID, first.Add(time.Minute)))

	scans, err := s.store.ListStudyFluencyScans(s.ctx, s.owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, scans, 1, "a word has one scan record, replaced by a newer scan")
	assert.WithinDuration(t, first.Add(time.Minute), scans[0].ExhaustedAt, time.Millisecond)

	assert.ErrorIs(t, s.store.RecordStudyFluencyExhausted(s.ctx, s.owner.ID, "00000000-0000-0000-0000-000000000000", first), persistence.ErrNotFound)
}
