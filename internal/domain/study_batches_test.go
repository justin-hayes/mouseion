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

func TestStudyBatchOfferableCapsTheDayByBatchesOffered(t *testing.T) {
	cases := []struct {
		name     string
		ledger   StudyBatchLedger
		forecast int
		want     bool
	}{
		{"first batch of the day", StudyBatchLedger{}, 0, true},
		{"third batch", StudyBatchLedger{Taken: 2}, 0, true},
		{"no fourth batch", StudyBatchLedger{Taken: 3}, 0, false},
		{"a declined batch still counts towards the cap", StudyBatchLedger{Taken: 2, Declined: 1}, 0, true},
		{"offered is not taken", StudyBatchLedger{Offered: 3}, 0, true},
		{"one batch at 100", StudyBatchLedger{}, 100, true},
		{"no second batch at 100", StudyBatchLedger{Taken: 1}, 100, false},
		{"the one batch declined under the brake can still be taken", StudyBatchLedger{Declined: 1}, 100, true},
		{"none at 150", StudyBatchLedger{}, 150, false},
		{"one batch at 99 leaves two more", StudyBatchLedger{Taken: 1}, 99, true},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, StudyBatchOfferable(tc.ledger, tc.forecast), tc.name)
	}
}

func TestStudyBatchLedgerTakingTheDeclinedBatchConsumesNoOtherBatch(t *testing.T) {
	ledger := StudyBatchLedger{Offered: 1}
	ledger, err := ledger.Decline(3)
	require.NoError(t, err)
	assert.Equal(t, StudyBatchLedger{Offered: 1, Declined: 1}, ledger)
	assert.Equal(t, 1, ledger.OfferNumber(), "the declined batch is still the one on offer")

	for range 3 {
		ledger, err = ledger.Take(3)
		require.NoError(t, err)
	}
	assert.Equal(t, StudyBatchLedger{Offered: 3, Taken: 3}, ledger, "the declined batch was the first taken, so three batches were offered")
	_, err = ledger.Take(3)
	require.ErrorIs(t, err, ErrStudyBatchLimit)
	_, err = ledger.Decline(3)
	require.ErrorIs(t, err, ErrStudyBatchLimit)

	declined, err := StudyBatchLedger{Taken: 1, Offered: 2}.Decline(1)
	require.ErrorIs(t, err, ErrStudyBatchLimit, "the brake allows one batch")
	assert.Zero(t, declined.Declined)
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
		identity := StudyItem{Language: "de", CanonicalLemma: fmt.Sprintf("w%02d", i), UPOS: "NOUN"}
		ordered = append(ordered, StudyCandidate{StudyItem: identity})
		material = append(material, StudyMaterial{ID: "m" + identity.CanonicalLemma, StudyItem: identity, State: StudyMaterialReady})
	}
	batch := NextStudyBatch(ordered, material, carried, nil, now)
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
	assert.Len(t, NextStudyBatch(ordered, material, many, nil, now), StudyBatchSize, "a large carry-over still makes one batch")
}

func TestStudyBatchIsShortOnlyWhileWordsArePreparing(t *testing.T) {
	assert.True(t, StudyBatchIsShort(make([]StudyBatchItem, 4), 2))
	assert.False(t, StudyBatchIsShort(make([]StudyBatchItem, 4), 0), "a short batch with nothing more coming is not explained")
	assert.False(t, StudyBatchIsShort(make([]StudyBatchItem, StudyBatchSize), 5))
}

func batchStream(n int) (ordered []StudyCandidate, material []StudyMaterial) {
	for i := range n {
		identity := StudyItem{Language: "de", CanonicalLemma: fmt.Sprintf("w%02d", i), UPOS: "NOUN"}
		ordered = append(ordered, StudyCandidate{StudyItem: identity})
		material = append(material, StudyMaterial{ID: "m" + identity.CanonicalLemma, StudyItem: identity, State: StudyMaterialReady})
	}
	return ordered, material
}

func TestNextStudyBatchSkipsHeldWordsAndFillsFromTheNextCandidate(t *testing.T) {
	now := berlin(11, 26, 9, 0)
	ordered, material := batchStream(12)
	held := []StudyItem{ordered[0].StudyItem, ordered[3].StudyItem}

	batch := NextStudyBatch(ordered, material, nil, held, now)

	require.Len(t, batch, StudyBatchSize, "the batch fills from the next candidates")
	var lemmas []string
	for _, item := range batch {
		lemmas = append(lemmas, item.CanonicalLemma)
	}
	assert.Equal(t, []string{"w01", "w02", "w04", "w05", "w06", "w07", "w08", "w09", "w10", "w11"}, lemmas)
	for _, identity := range held {
		assert.NotContains(t, StudyBatchLookup(material, nil, held), identity, "a held word is not read from the stream")
	}
	assert.Len(t, StudyBatchLookup(material, nil, held), 10)
}

func TestAHeldWordIsNotCarriedOverOrWaitingToBeAnswered(t *testing.T) {
	now := berlin(11, 26, 9, 0)
	carried := untouched("alt", berlin(11, 24, 9, 0))
	carried.Held = true
	today := untouched("heute", berlin(11, 26, 5, 0))
	today.Held = true
	other := untouched("neu", berlin(11, 25, 9, 0))
	all := []StudyProgress{carried, today, other}

	require.Len(t, StudyCarryOver(all, now), 1, "a held word does not come back to the front of the stream")
	assert.Equal(t, "neu", StudyCarryOver(all, now)[0].CanonicalLemma)
	assert.Empty(t, StudyUnintroduced(all, now), "a held word does not block the next batch offer")
	ordered, material := batchStream(2)
	for _, word := range NextStudyBatch(ordered, material, all, nil, now) {
		assert.NotEqual(t, "alt", word.CanonicalLemma)
	}
	assert.Equal(t, "p-neu", NextStudyBatch(ordered, material, []StudyProgress{func() StudyProgress { o := other; o.ID = "p-neu"; return o }(), carried}, nil, now)[0].ProgressID)
}

func TestStudyHeldIdentitiesAreTheOpenWrongWordReports(t *testing.T) {
	a := StudyItem{Language: "de", CanonicalLemma: "a", UPOS: "NOUN"}
	b := StudyItem{Language: "de", CanonicalLemma: "b", UPOS: "NOUN"}
	reports := []StudyReport{
		{StudyItem: a, Kind: StudyReportWrongWord},
		{StudyItem: b, Kind: StudyReportUnsuitableContext},
		{StudyItem: b, Kind: StudyReportWrongWord, EndedAt: berlin(11, 26, 9, 0)},
	}
	assert.Equal(t, []StudyItem{a}, StudyHeldIdentities(reports))
}
