package domain

import (
	"sort"
	"time"
)

// Pilot starting values for how many New-word batches Today offers (spec
// #2691). They are constants, not learner settings.
const (
	// StudyBatchesPerDay is the most New-word batches offered on one Study day
	// (three batches of ten words).
	StudyBatchesPerDay = 3
	// StudyBrakeOneBatchForecast is the forecast review count from which at
	// most one batch is offered on the Study day.
	StudyBrakeOneBatchForecast = 100
	// StudyBrakeNoBatchForecast is the forecast review count from which no
	// batch is offered.
	StudyBrakeNoBatchForecast = 150
)

// StudyForecast is the number of reviews forecast for the next Study day: the
// words in study, other than those introduced and not yet answered, whose
// review is due on the next Study day or earlier. It applies the rule Today
// uses (a review is due on the Study day containing its due time), so what is
// counted here is what Today then shows. A word repeating today counts, since
// it becomes a review the next day.
func StudyForecast(progress []StudyProgress, now time.Time) int {
	end := NextStudyDayStart(NextStudyDayStart(now))
	count := 0
	for _, item := range progress {
		if item.Withdrawn || item.Memory.State == StudyStateNew {
			continue
		}
		if StudyReviewDue(item).Before(end) {
			count++
		}
	}
	return count
}

// StudyBatchesAllowed is how many New-word batches may be offered on a Study
// day given the forecast review load: the daily cap, lowered to one at
// StudyBrakeOneBatchForecast reviews and to none at StudyBrakeNoBatchForecast.
func StudyBatchesAllowed(forecast int) int {
	switch {
	case forecast >= StudyBrakeNoBatchForecast:
		return 0
	case forecast >= StudyBrakeOneBatchForecast:
		return 1
	}
	return StudyBatchesPerDay
}

// StudyBatchOfferable reports whether another batch may be offered on a day
// whose ledger is given. The cap counts batches taken, never those declined.
// Declining ends only Today's own offers (see FluencyUnlocked): the learner may
// still take a remaining batch from inside Fluency while the cap and the
// backlog brake allow it.
func StudyBatchOfferable(ledger StudyBatchLedger, forecast int) bool {
	return ledger.Taken < StudyBatchesAllowed(forecast)
}

// studyCarriedOver reports whether the word was taken in an earlier Study
// day's batch and never answered. It is held out of Today until the batch it
// now belongs to is taken.
func studyCarriedOver(p StudyProgress, now time.Time) bool {
	return !p.Withdrawn && p.Memory.State == StudyStateNew && !p.Graded() && p.IntroducedAt.Before(StudyDayStart(now))
}

// StudyCarryOver returns the words taken on an earlier Study day and not yet
// introduced, oldest first. They lead the next batch and count towards its
// size, the daily cap and the backlog brake. While no batch is taken they are
// not shown, so under the brake they wait at the front of the stream.
func StudyCarryOver(progress []StudyProgress, now time.Time) []StudyProgress {
	var carried []StudyProgress
	for _, item := range progress {
		if studyCarriedOver(item, now) {
			carried = append(carried, item)
		}
	}
	sort.SliceStable(carried, func(i, j int) bool { return studyIntroducedBefore(carried[i], carried[j]) })
	return carried
}

// StudyUnintroduced returns the words of a batch taken on the current Study
// day that the learner has not yet answered. Another batch is offered only
// once they are answered.
func StudyUnintroduced(progress []StudyProgress, now time.Time) []StudyProgress {
	var pending []StudyProgress
	for _, item := range progress {
		if !item.Withdrawn && !item.Graded() && !studyCarriedOver(item, now) {
			pending = append(pending, item)
		}
	}
	sort.SliceStable(pending, func(i, j int) bool { return studyIntroducedBefore(pending[i], pending[j]) })
	return pending
}

func studyIntroducedBefore(a, b StudyProgress) bool {
	if !a.IntroducedAt.Equal(b.IntroducedAt) {
		return a.IntroducedAt.Before(b.IntroducedAt)
	}
	if a.CanonicalLemma != b.CanonicalLemma {
		return a.CanonicalLemma < b.CanonicalLemma
	}
	if a.UPOS != b.UPOS {
		return a.UPOS < b.UPOS
	}
	return a.LexicalDiscriminator < b.LexicalDiscriminator
}

// StudyBatchIsShort reports whether a batch holds fewer words than a full
// batch while more words are still being prepared, so Today says why.
func StudyBatchIsShort(batch []StudyBatchWord, preparing int) bool {
	return len(batch) < StudyBatchSize && preparing > 0
}
