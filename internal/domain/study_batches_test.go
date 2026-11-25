package domain

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func untouched(lemma string, introduced time.Time) StudyProgress {
	p := reviewWord(lemma, StudyStateNew, introduced, time.Time{})
	p.Memory.Reps, p.IntroducedAt, p.MaterialID = 0, introduced, "material-"+lemma
	return p
}

func TestStudyCarryOverIsTheWordsTakenOnEarlierDaysAndNeverAnswered(t *testing.T) {
	now := berlin(11, 26, 9, 0)
	older := untouched("older", berlin(11, 24, 9, 0))
	newer := untouched("newer", berlin(11, 25, 9, 0))
	today := untouched("today", berlin(11, 26, 5, 0))
	graded := reviewWord("graded", StudyStateReview, berlin(11, 27, 5, 0), berlin(11, 25, 9, 0))
	all := []StudyProgress{newer, today, graded, older}

	carried := StudyCarryOver(all, now)
	require.Len(t, carried, 2)
	assert.Equal(t, []string{"older", "newer"}, []string{carried[0].CanonicalLemma, carried[1].CanonicalLemma})
	pending := StudyUnintroduced(all, now)
	require.Len(t, pending, 1, "only the batch taken today is waiting to be answered")
	assert.Equal(t, "today", pending[0].CanonicalLemma)

	assert.False(t, StudyCardIsDue(older, now), "a carried-over word is held until its batch is taken")
	assert.True(t, StudyCardIsDue(today, now))
	assert.False(t, StudyCardIsDue(newer, berlin(11, 25, 20, 0).Add(-24*time.Hour)), "a word taken later than now is not due")
}

func TestNextStudyBatchLeadsWithTheCarryOverAndFillsFromTheStream(t *testing.T) {
	now := berlin(11, 26, 9, 0)
	carried := []StudyProgress{untouched("alt", berlin(11, 24, 9, 0)), untouched("neu", berlin(11, 25, 9, 0))}
	carried[0].ID, carried[1].ID = "p-alt", "p-neu"
	var ordered []StudyCandidate
	var material []StudyMaterial
	for i := range 12 {
		identity := StudyIdentity{Language: "de", CanonicalLemma: fmt.Sprintf("w%02d", i), UPOS: "NOUN"}
		ordered = append(ordered, StudyCandidate{StudyIdentity: identity})
		material = append(material, StudyMaterial{ID: "m" + identity.CanonicalLemma, StudyIdentity: identity, State: StudyMaterialReady})
	}
	batch := NextStudyBatch(ordered, material, carried, now)
	require.Len(t, batch, StudyBatchSize, "the carry-over counts towards the batch size")
	assert.Equal(t, "p-alt", batch[0].ProgressID)
	assert.Equal(t, "p-neu", batch[1].ProgressID)
	assert.Equal(t, "material-alt", batch[0].MaterialID)
	assert.Empty(t, batch[2].ProgressID)
	assert.Equal(t, "w00", batch[2].CanonicalLemma, "the stream fills the rest")

	var many []StudyProgress
	for i := range 12 {
		many = append(many, untouched(fmt.Sprintf("c%02d", i), berlin(11, 24, 9, i)))
	}
	assert.Len(t, NextStudyBatch(ordered, material, many, now), StudyBatchSize, "a large carry-over still makes one batch")
}

func TestStudyBatchIsShortOnlyWhileWordsArePreparing(t *testing.T) {
	assert.True(t, StudyBatchIsShort(make([]StudyBatchWord, 4), 2))
	assert.False(t, StudyBatchIsShort(make([]StudyBatchWord, 4), 0), "a short batch with nothing more coming is not explained")
	assert.False(t, StudyBatchIsShort(make([]StudyBatchWord, StudyBatchSize), 5))
}
