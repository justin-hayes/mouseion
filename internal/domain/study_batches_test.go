package domain

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStudyBatchesAllowedFollowsTheForecastThresholds(t *testing.T) {
	cases := []struct{ forecast, allowed int }{
		{0, 3}, {99, 3}, {100, 1}, {149, 1}, {150, 0}, {400, 0},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.allowed, StudyBatchesAllowed(tc.forecast), "forecast %d", tc.forecast)
	}
}

func TestStudyBatchOfferableCapsTheDayAndCountsTakenBatchesOnly(t *testing.T) {
	cases := []struct {
		name     string
		ledger   StudyBatchLedger
		forecast int
		want     bool
	}{
		{"first batch of the day", StudyBatchLedger{}, 0, true},
		{"third batch", StudyBatchLedger{Taken: 2}, 0, true},
		{"no fourth batch", StudyBatchLedger{Taken: 3}, 0, false},
		{"a declined batch ends the day's offers", StudyBatchLedger{Declined: 1}, 0, false},
		{"offered is not taken", StudyBatchLedger{Offered: 3}, 0, true},
		{"one batch at 100", StudyBatchLedger{}, 100, true},
		{"no second batch at 100", StudyBatchLedger{Taken: 1}, 100, false},
		{"none at 150", StudyBatchLedger{}, 150, false},
		{"one batch at 99 leaves two more", StudyBatchLedger{Taken: 1}, 99, true},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, StudyBatchOfferable(tc.ledger, tc.forecast), tc.name)
	}
}

func TestStudyForecastCountsReviewsDueOnTheNextStudyDayOrEarlier(t *testing.T) {
	now := berlin(11, 25, 21, 0)
	withdrawn := reviewWord("withdrawn", StudyStateReview, berlin(11, 26, 9, 0), berlin(11, 25, 8, 0))
	withdrawn.Withdrawn = true
	progress := []StudyProgress{
		reviewWord("tomorrow-early", StudyStateReview, berlin(11, 26, 5, 0), berlin(11, 25, 8, 0)),
		reviewWord("tomorrow-evening", StudyStateReview, berlin(11, 26, 20, 0), berlin(11, 20, 8, 0)),
		reviewWord("tomorrow-last-minute", StudyStateReview, berlin(11, 27, 4, 59), berlin(11, 20, 8, 0)),
		reviewWord("overdue", StudyStateReview, berlin(11, 23, 8, 0), berlin(11, 20, 8, 0)),
		reviewWord("repeating-tonight", StudyStateLearning, berlin(11, 25, 21, 10), berlin(11, 25, 21, 0)),
		reviewWord("day-after", StudyStateReview, berlin(11, 27, 5, 0), berlin(11, 25, 8, 0)),
		reviewWord("untouched", StudyStateNew, berlin(11, 25, 20, 0), time.Time{}),
		withdrawn,
	}
	assert.Equal(t, 5, StudyForecast(progress, now))
}

func TestStudyForecastUsesTheSameDayBoundaryAsToday(t *testing.T) {
	// 03:00 on 26 November still belongs to the Study day that began on 25 November.
	now := berlin(11, 26, 3, 0)
	progress := []StudyProgress{
		reviewWord("a", StudyStateReview, berlin(11, 26, 4, 0), berlin(11, 20, 8, 0)),
		reviewWord("b", StudyStateReview, berlin(11, 26, 5, 0), berlin(11, 20, 8, 0)),
		reviewWord("c", StudyStateReview, berlin(11, 27, 4, 59), berlin(11, 20, 8, 0)),
		reviewWord("d", StudyStateReview, berlin(11, 27, 5, 0), berlin(11, 20, 8, 0)),
	}
	assert.Equal(t, 3, StudyForecast(progress, now), "the next Study day began at 05:00 on 26 November and ends at 05:00 on 27 November")
}

func TestStudyForecastReachesTheBrakeThresholds(t *testing.T) {
	now := berlin(11, 25, 12, 0)
	for _, tc := range []struct{ count, allowed int }{{99, 3}, {100, 1}, {149, 1}, {150, 0}} {
		progress := make([]StudyProgress, 0, tc.count)
		for i := range tc.count {
			progress = append(progress, reviewWord(fmt.Sprintf("w%03d", i), StudyStateReview, berlin(11, 26, 9, 0), berlin(11, 25, 8, 0)))
		}
		assert.Equal(t, tc.allowed, StudyBatchesAllowed(StudyForecast(progress, now)), "%d reviews", tc.count)
	}
}

func TestStudyUnintroducedLeadsOldestFirst(t *testing.T) {
	older := reviewWord("older", StudyStateNew, berlin(11, 24, 9, 0), time.Time{})
	older.Memory.Reps, older.IntroducedAt = 0, berlin(11, 24, 9, 0)
	newer := reviewWord("newer", StudyStateNew, berlin(11, 25, 9, 0), time.Time{})
	newer.Memory.Reps, newer.IntroducedAt = 0, berlin(11, 25, 9, 0)
	graded := reviewWord("graded", StudyStateReview, berlin(11, 26, 5, 0), berlin(11, 25, 9, 0))
	got := StudyUnintroduced([]StudyProgress{newer, graded, older})
	assert.Equal(t, []string{"older", "newer"}, []string{got[0].CanonicalLemma, got[1].CanonicalLemma})
}

func TestStudyBatchIsShortOnlyWhileWordsArePreparing(t *testing.T) {
	assert.True(t, StudyBatchIsShort(make([]StudyBatchWord, 4), 2))
	assert.False(t, StudyBatchIsShort(make([]StudyBatchWord, 4), 0), "a short batch with nothing more coming is not explained")
	assert.False(t, StudyBatchIsShort(make([]StudyBatchWord, StudyBatchSize), 5))
}
