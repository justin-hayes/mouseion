package domain

import (
	"sort"
	"time"
)

// StudyReviewDue is the time a word's review is due for Today's purposes. A
// word still repeating (learning or relearning) never waits past the start of
// the Study day after its last answer: an Again graded in the last minutes
// before the boundary would otherwise come due a few minutes after it and
// miss the day's due set, so it is due at the boundary instead. Every other
// word is due when its scheduling state says.
func StudyReviewDue(p StudyProgress) time.Time {
	due := p.Memory.Due
	repeating := p.Memory.State == StudyStateLearning || p.Memory.State == StudyStateRelearning
	if repeating && !p.Memory.LastReview.IsZero() {
		if boundary := NextStudyDayStart(p.Memory.LastReview); boundary.Before(due) {
			return boundary
		}
	}
	return due
}

// studyReviewInToday reports whether a word on a day-scale schedule belongs to
// Today's due set: its due time falls in the current Study day or an earlier
// one, so a review due at 20:00 is due from the day's start and every overdue
// review is due after missed days. A word already answered in the current
// Study day is not due again, whatever the length of the day.
func studyReviewInToday(p StudyProgress, now time.Time) bool {
	if !StudyReviewDue(p).Before(NextStudyDayStart(now)) {
		return false
	}
	return p.Memory.LastReview.IsZero() || p.Memory.LastReview.Before(StudyDayStart(now))
}

// StudyCardIsDue reports whether Today shows the word at now. A word on a
// day-scale schedule belongs to the Study day that contains its due time and
// is due once that Study day is today or earlier; the day's set is fixed when
// the day begins. A word still repeating, and a word just introduced, is due
// when its due time arrives.
func StudyCardIsDue(p StudyProgress, now time.Time) bool {
	if p.Withdrawn || p.Reintroducing || studyCarriedOver(p, now) {
		return false
	}
	if p.Memory.State == StudyStateReview {
		return studyReviewInToday(p, now)
	}
	return !StudyReviewDue(p).After(now)
}

// StudyCountsToday reports whether the word is part of Today's finish line: a
// review whose due Study day is today or earlier, or a review the learner
// forgot and is repeating. New-word introductions and their repeats are never
// part of it (spec #2691), so a word still new or learning is not counted. A
// review waiting for new Study material after a correction is not counted
// either, since it cannot be shown; it joins Today once its material is ready.
func StudyCountsToday(p StudyProgress, now time.Time) bool {
	return !p.Reintroducing && studyInTodaysDueSet(p, now)
}

// StudyAwaitsMaterial reports whether the word is part of Today's due set but
// waits for new Study material after a correction.
func StudyAwaitsMaterial(p StudyProgress, now time.Time) bool {
	return p.Reintroducing && studyInTodaysDueSet(p, now)
}

func studyInTodaysDueSet(p StudyProgress, now time.Time) bool {
	if p.Withdrawn {
		return false
	}
	if p.Memory.State == StudyStateReview {
		return studyReviewInToday(p, now)
	}
	return p.Memory.State == StudyStateRelearning
}

// StudyToday is where the learner stands on the finish line.
type StudyToday struct {
	// Remaining is how many reviews of Today's due set are still unanswered,
	// the same-day repeats they caused included.
	Remaining int
	// Finished is true once none remains. It is derived from the words'
	// scheduling state alone: a Study day's due set is fixed when the day
	// begins and an answered review is never due again that day, so new
	// material never reopens it.
	Finished bool
}

// StudyFinishLine counts what is left of Today's due set.
func StudyFinishLine(progress []StudyProgress, now time.Time) StudyToday {
	remaining := 0
	for _, item := range progress {
		if StudyCountsToday(item, now) {
			remaining++
		}
	}
	return StudyToday{Remaining: remaining, Finished: remaining == 0}
}

// NextStudyReview picks the word to show at now: the earliest due, ties
// broken by introduction time and then identity so the order is deterministic.
// Reviews of the finish line come before introductions, and an introduction
// is held back while a repeat of those reviews is still to come. ok is false
// when nothing is due.
func NextStudyReview(progress []StudyProgress, now time.Time) (StudyProgress, bool) {
	due, waiting := dueStudyWords(progress, now)
	if waiting {
		counted := due[:0:0]
		for _, item := range due {
			if StudyCountsToday(item, now) {
				counted = append(counted, item)
			}
		}
		due = counted
	}
	if len(due) == 0 {
		return StudyProgress{}, false
	}
	sort.SliceStable(due, func(i, j int) bool { return studyReviewBefore(due[i], due[j], now) })
	return due[0], true
}

// dueStudyWords splits out the words due at now, and reports whether a word
// of the finish line is still waiting for its repeat.
func dueStudyWords(progress []StudyProgress, now time.Time) (due []StudyProgress, waiting bool) {
	for _, item := range progress {
		switch {
		case StudyCardIsDue(item, now):
			due = append(due, item)
		case StudyCountsToday(item, now):
			waiting = true
		}
	}
	return due, waiting
}

func studyReviewBefore(a, b StudyProgress, now time.Time) bool {
	if ca, cb := StudyCountsToday(a, now), StudyCountsToday(b, now); ca != cb {
		return ca
	}
	if da, db := StudyReviewDue(a), StudyReviewDue(b); !da.Equal(db) {
		return da.Before(db)
	}
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
