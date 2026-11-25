package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func berlin(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, month, day, hour, minute, 0, 0, studyLocation)
}

func reviewWord(lemma string, state StudyCardState, due, last time.Time) StudyProgress {
	p := StudyProgress{
		ID:           lemma,
		IntroducedAt: berlin(1, 1, 9, 0),
		Memory:       StudyMemory{State: state, Due: due.UTC(), LastReview: last.UTC(), Reps: 1},
	}
	p.CanonicalLemma, p.UPOS = lemma, "NOUN"
	return p
}

func TestStudyReviewsDueAtTheStartOfTheStudyDay(t *testing.T) {
	now := berlin(11, 25, 9, 0) // the Study day began at 05:00 today
	cases := []struct {
		name     string
		progress StudyProgress
		shown    bool
		counts   bool
	}{
		{"due before the day began", reviewWord("a", StudyStateReview, berlin(11, 24, 20, 0), berlin(11, 21, 20, 0)), true, true},
		{"due exactly at the start", reviewWord("b", StudyStateReview, berlin(11, 25, 5, 0), berlin(11, 24, 8, 0)), true, true},
		{"due later today waits for the next Study day", reviewWord("c", StudyStateReview, berlin(11, 25, 14, 0), berlin(11, 22, 14, 0)), false, false},
		{"due tomorrow", reviewWord("d", StudyStateReview, berlin(11, 26, 5, 0), berlin(11, 25, 8, 0)), false, false},
		{"three missed days are all due", reviewWord("e", StudyStateReview, berlin(11, 22, 7, 0), berlin(11, 20, 7, 0)), true, true},
		{"a repeat that is due", reviewWord("f", StudyStateRelearning, berlin(11, 25, 8, 50), berlin(11, 25, 8, 40)), true, true},
		{"a repeat that is not due yet still counts", reviewWord("g", StudyStateRelearning, berlin(11, 25, 9, 10), berlin(11, 25, 9, 0)), false, true},
		{"a new word never counts", reviewWord("h", StudyStateNew, berlin(11, 25, 8, 0), time.Time{}), true, false},
		{"an introduction repeat never counts", reviewWord("i", StudyStateLearning, berlin(11, 25, 8, 50), berlin(11, 25, 8, 40)), true, false},
		{"a withdrawn word is not due", func() StudyProgress {
			p := reviewWord("j", StudyStateReview, berlin(11, 20, 5, 0), berlin(11, 19, 5, 0))
			p.Withdrawn = true
			return p
		}(), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.shown, StudyCardIsDue(tc.progress, now), "shown")
			assert.Equal(t, tc.counts, StudyCountsToday(tc.progress, now), "counts towards the finish line")
		})
	}
}

// An Again graded in the last minutes before 05:00 is due after the boundary.
// It is a same-day repeat, so it must come due at the boundary rather than
// wait for the next Study day.
func TestStudyRepeatAcrossTheStudyDayBoundaryIsDueAtTheBoundary(t *testing.T) {
	last := berlin(11, 25, 4, 55)
	repeat := reviewWord("haus", StudyStateRelearning, berlin(11, 25, 5, 5), last)
	assert.True(t, berlin(11, 25, 5, 0).Equal(StudyReviewDue(repeat)))
	assert.False(t, StudyCardIsDue(repeat, berlin(11, 25, 4, 58)), "it comes back after its short wait, not at once")
	assert.True(t, StudyCardIsDue(repeat, berlin(11, 25, 5, 0)), "the new Study day starts with it due")
	assert.True(t, StudyCountsToday(repeat, berlin(11, 25, 5, 0)))
	// An introduction graded Again just before the boundary is due then too.
	intro := reviewWord("buch", StudyStateLearning, berlin(11, 25, 5, 1), last)
	assert.True(t, StudyCardIsDue(intro, berlin(11, 25, 5, 0)))
	assert.False(t, StudyCountsToday(intro, berlin(11, 25, 5, 0)))
	// A repeat inside the day is untouched.
	inside := reviewWord("tisch", StudyStateRelearning, berlin(11, 25, 9, 10), berlin(11, 25, 9, 0))
	assert.True(t, berlin(11, 25, 9, 10).Equal(StudyReviewDue(inside)))
}

func TestStudyFinishLineStaysFinishedUntilTheNextStudyDay(t *testing.T) {
	due := reviewWord("a", StudyStateReview, berlin(11, 24, 8, 0), berlin(11, 22, 8, 0))
	assert.Equal(t, StudyToday{Remaining: 1}, StudyFinishLine([]StudyProgress{due}, berlin(11, 25, 9, 0)))

	// Answered Got it: due days later.
	answered := reviewWord("a", StudyStateReview, berlin(11, 28, 9, 0), berlin(11, 25, 9, 0))
	// New material and a freshly introduced word do not reopen the day.
	fresh := reviewWord("b", StudyStateNew, berlin(11, 25, 9, 1), time.Time{})
	progress := []StudyProgress{answered, fresh}
	for _, now := range []time.Time{berlin(11, 25, 9, 5), berlin(11, 25, 23, 59), berlin(11, 26, 4, 59)} {
		assert.True(t, StudyFinishLine(progress, now).Finished, now.String())
	}
	// A word introduced Got it is due at the next Study day start.
	introduced := reviewWord("c", StudyStateReview, NextStudyDayStart(berlin(11, 25, 9, 0)), berlin(11, 25, 9, 0))
	assert.True(t, StudyFinishLine([]StudyProgress{introduced}, berlin(11, 26, 4, 59)).Finished)
	assert.Equal(t, 1, StudyFinishLine([]StudyProgress{introduced}, berlin(11, 26, 5, 0)).Remaining)
}

// Both daylight-saving changes move the clock by an hour, so the Study day is
// 23 or 25 hours long, and the boundary stays at 05:00 local.
func TestStudyFinishLineAcrossDaylightSavingChanges(t *testing.T) {
	cases := []struct {
		name      string
		answered  time.Time // a review answered late in the Study day that is ending
		length    time.Duration
		startsDay string
	}{
		{"spring forward", berlin(3, 28, 22, 0), 23 * time.Hour, "2026-03-28"},
		{"autumn back", berlin(10, 24, 22, 0), 25 * time.Hour, "2026-10-24"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := StudyDayStart(tc.answered)
			boundary := NextStudyDayStart(tc.answered)
			assert.Equal(t, tc.length, boundary.Sub(start), "the Study day is as long as the clock change makes it")
			assert.Equal(t, 5, start.In(studyLocation).Hour())
			assert.Equal(t, 5, boundary.In(studyLocation).Hour())
			assert.Equal(t, tc.startsDay, StudyDay(tc.answered))
			// Every instant just before the boundary, including the repeated or
			// skipped hour after midnight, belongs to the day that is ending.
			assert.Equal(t, tc.startsDay, StudyDay(boundary.Add(-time.Second)))
			assert.Equal(t, tc.startsDay, StudyDay(boundary.Add(-3*time.Hour)))
			assert.NotEqual(t, tc.startsDay, StudyDay(boundary))

			// A word answered Got it on first sight is due at the boundary: not
			// yet due a second before, due at it, and counted once.
			word := reviewWord("haus", StudyStateReview, boundary, tc.answered)
			assert.False(t, StudyCardIsDue(word, boundary.Add(-time.Second)))
			assert.Equal(t, StudyToday{Finished: true}, StudyFinishLine([]StudyProgress{word}, boundary.Add(-time.Second)))
			assert.True(t, StudyCardIsDue(word, boundary))
			assert.Equal(t, StudyToday{Remaining: 1}, StudyFinishLine([]StudyProgress{word}, boundary))
		})
	}
}

func TestNextStudyReviewOrdersTheFinishLineFirst(t *testing.T) {
	now := berlin(11, 25, 9, 0)
	review := reviewWord("zebra", StudyStateReview, berlin(11, 24, 20, 0), berlin(11, 20, 20, 0))
	older := reviewWord("apfel", StudyStateReview, berlin(11, 23, 20, 0), berlin(11, 20, 20, 0))
	fresh := reviewWord("baum", StudyStateNew, berlin(11, 25, 8, 0), time.Time{})
	got, ok := NextStudyReview([]StudyProgress{fresh, review, older}, now)
	assert.True(t, ok)
	assert.Equal(t, "apfel", got.ID, "the most overdue review comes first")
	got, _ = NextStudyReview([]StudyProgress{fresh, review}, now)
	assert.Equal(t, "zebra", got.ID, "reviews come before a word just introduced")
	got, _ = NextStudyReview([]StudyProgress{fresh}, now)
	assert.Equal(t, "baum", got.ID)
	_, ok = NextStudyReview([]StudyProgress{reviewWord("x", StudyStateReview, berlin(11, 28, 5, 0), now)}, now)
	assert.False(t, ok)
}

func TestNextStudyReviewHoldsIntroductionsBackWhileARepeatIsStillToCome(t *testing.T) {
	now := berlin(11, 25, 9, 0)
	waiting := reviewWord("haus", StudyStateRelearning, berlin(11, 25, 9, 10), berlin(11, 25, 9, 0))
	intro := reviewWord("baum", StudyStateNew, berlin(11, 25, 8, 0), time.Time{})
	_, ok := NextStudyReview([]StudyProgress{waiting, intro}, now)
	assert.False(t, ok, "new words wait for the last review's repeat")
	got, ok := NextStudyReview([]StudyProgress{waiting, intro}, berlin(11, 25, 9, 10))
	assert.True(t, ok)
	assert.Equal(t, "haus", got.ID)
}
