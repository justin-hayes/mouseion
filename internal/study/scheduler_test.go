package study

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var berlin = func() *time.Location {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(err)
	}
	return location
}()

func TestSchedulerUsesTheDocumentedParameters(t *testing.T) {
	// fsrs.NewFSRS silently replaces rejected parameters with its defaults, so
	// the configuration is asserted after construction.
	params := NewScheduler().fsrs.Parameters
	assert.InDelta(t, 0.9, params.RequestRetention, 0)
	assert.True(t, params.EnableFuzz, "interval fuzz is on")
	assert.True(t, params.EnableShortTerm)
	assert.Equal(t, []float64{1}, params.LearningSteps)
	assert.Equal(t, []float64{10}, params.RelearningSteps)
}

func TestGotItOnFirstSightIsDueAtTheStartOfTheNextStudyDay(t *testing.T) {
	scheduler := NewScheduler()
	for _, now := range []time.Time{
		time.Date(2026, 11, 25, 9, 0, 0, 0, berlin),
		time.Date(2026, 11, 26, 1, 0, 0, 0, berlin), // after midnight, still the same Study day
		time.Date(2026, 11, 25, 22, 30, 0, 0, berlin),
	} {
		after, err := scheduler.Schedule(domain.NewStudyMemory(now), domain.StudyGradeGotIt, now)
		require.NoError(t, err)
		assert.Equal(t, domain.StudyStateReview, after.State)
		assert.True(t, domain.NextStudyDayStart(now).Equal(after.Due), "due %v at %v", after.Due, now)
		assert.Equal(t, int64(1), after.Reps)
		assert.Positive(t, after.Stability, "FSRS memory state is kept")
		assert.Positive(t, after.Difficulty)
		assert.True(t, now.Equal(after.LastReview))
	}
}

func TestAgainOnFirstSightRepeatsTheSameDay(t *testing.T) {
	scheduler := NewScheduler()
	now := time.Date(2026, 11, 25, 9, 0, 0, 0, berlin)
	after, err := scheduler.Schedule(domain.NewStudyMemory(now), domain.StudyGradeAgain, now)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyStateLearning, after.State, "Again keeps the word in learning")
	assert.True(t, after.Due.After(now))
	assert.True(t, after.Due.Before(domain.NextStudyDayStart(now)), "and repeats it on the same Study day")

	// Got it on the repeat graduates the word, due the next Study day: the
	// introduction is not over until it has been answered correctly.
	later := after.Due
	graduated, err := scheduler.Schedule(after, domain.StudyGradeGotIt, later)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyStateReview, graduated.State)
	assert.True(t, domain.NextStudyDayStart(later).Equal(graduated.Due))
	assert.Equal(t, int64(2), graduated.Reps)
}

func TestLaterReviewsFollowFSRSIntervals(t *testing.T) {
	scheduler := NewScheduler()
	now := time.Date(2026, 11, 25, 9, 0, 0, 0, berlin)
	first, err := scheduler.Schedule(domain.NewStudyMemory(now), domain.StudyGradeGotIt, now)
	require.NoError(t, err)

	// A review answered on the day it is due keeps the word on a day scale.
	reviewedAt := first.Due.Add(2 * time.Hour)
	second, err := scheduler.Schedule(first, domain.StudyGradeGotIt, reviewedAt)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyStateReview, second.State)
	assert.Greater(t, second.Stability, first.Stability, "recalling a word makes its memory more stable")
	assert.Greater(t, second.ScheduledDays, int64(1), "and spaces the next review further out")
	assert.True(t, second.Due.After(reviewedAt.Add(24*time.Hour)))

	// Forgetting a reviewed word relearns it the same day and counts a lapse.
	lapsed, err := scheduler.Schedule(second, domain.StudyGradeAgain, second.Due)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyStateRelearning, lapsed.State)
	assert.Equal(t, int64(1), lapsed.Lapses)
	assert.True(t, lapsed.Due.Before(second.Due.Add(time.Hour)))
}

func TestSchedulerRejectsAnUnknownGradeAndAReviewBeforeTheLastOne(t *testing.T) {
	scheduler := NewScheduler()
	now := time.Date(2026, 11, 25, 9, 0, 0, 0, berlin)
	_, err := scheduler.Schedule(domain.NewStudyMemory(now), domain.StudyGrade("hard"), now)
	require.Error(t, err)

	first, err := scheduler.Schedule(domain.NewStudyMemory(now), domain.StudyGradeGotIt, now)
	require.NoError(t, err)
	_, err = scheduler.Schedule(first, domain.StudyGradeGotIt, now.Add(-time.Hour))
	require.Error(t, err, "a clock that moved backwards cannot grade a card")
}
