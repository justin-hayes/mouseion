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
// whose ledger is given. Declining a batch ends the day's offers: declining
// marks no word, so the same words would otherwise be offered again at once.
// The cap counts batches taken, never those declined.
func StudyBatchOfferable(ledger StudyBatchLedger, forecast int) bool {
	return ledger.Declined == 0 && ledger.Taken < StudyBatchesAllowed(forecast)
}

// StudyUnintroduced returns the words taken in a batch and not yet
// introduced, that is, not yet answered once, oldest first. When a Study day
// begins they lead the stream: they are shown before any new batch, ahead of
// the words that follow them in stream order, and a batch is offered only
// once they are answered.
func StudyUnintroduced(progress []StudyProgress) []StudyProgress {
	var pending []StudyProgress
	for _, item := range progress {
		if !item.Withdrawn && !item.Graded() {
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
