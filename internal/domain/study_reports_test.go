package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func heldWord(word StudyProgress) StudyProgress {
	word.Held = true
	return word
}

func TestAHeldWordIsNotDueAndNotPartOfTheFinishLine(t *testing.T) {
	now := berlin(11, 25, 9, 0)
	cases := map[string]StudyProgress{
		"a review overdue":       reviewWord("a", StudyStateReview, berlin(11, 24, 20, 0), berlin(11, 21, 20, 0)),
		"a review due today":     reviewWord("b", StudyStateReview, berlin(11, 25, 20, 0), berlin(11, 22, 20, 0)),
		"a repeat that is due":   reviewWord("c", StudyStateRelearning, berlin(11, 25, 8, 50), berlin(11, 25, 8, 40)),
		"a learning repeat":      reviewWord("d", StudyStateLearning, berlin(11, 25, 8, 50), berlin(11, 25, 8, 40)),
		"a word just introduced": reviewWord("e", StudyStateNew, berlin(11, 25, 8, 0), time.Time{}),
	}
	for name, word := range cases {
		t.Run(name, func(t *testing.T) {
			held := heldWord(word)
			assert.False(t, StudyCardIsDue(held, now), "a held word gets no review")
			assert.False(t, StudyCountsToday(held, now), "and is not in the finish line")
			assert.False(t, StudyAwaitsMaterial(held, now))
			_, ok := NextStudyReview([]StudyProgress{held}, now)
			assert.False(t, ok)
		})
	}
}

func TestTodayFinishesWithAHeldWordOutstanding(t *testing.T) {
	now := berlin(11, 25, 9, 0)
	held := heldWord(reviewWord("held", StudyStateReview, berlin(11, 24, 20, 0), berlin(11, 21, 20, 0)))
	answered := reviewWord("answered", StudyStateReview, berlin(11, 28, 9, 0), berlin(11, 25, 8, 0))
	assert.Equal(t, StudyToday{Finished: true}, StudyFinishLine([]StudyProgress{held, answered}, now))

	due := reviewWord("due", StudyStateReview, berlin(11, 24, 20, 0), berlin(11, 21, 20, 0))
	assert.Equal(t, StudyToday{Remaining: 1}, StudyFinishLine([]StudyProgress{held, due}, now), "a held word does not count in remaining reviews")
	next, ok := NextStudyReview([]StudyProgress{held, due}, now)
	assert.True(t, ok)
	assert.Equal(t, "due", next.ID, "the next card skips the held word")

	// Once the report ends the word is due again.
	held.Held = false
	assert.Equal(t, 2, StudyFinishLine([]StudyProgress{held, due}, now).Remaining)
}

func TestAHeldWordIsNotForecast(t *testing.T) {
	now := berlin(11, 25, 9, 0)
	word := reviewWord("a", StudyStateReview, berlin(11, 26, 8, 0), berlin(11, 22, 8, 0))
	assert.Equal(t, 1, StudyForecast([]StudyProgress{word}, now))
	assert.Equal(t, 0, StudyForecast([]StudyProgress{heldWord(word)}, now))
}

func TestStudyHeldDueMovesLaterByTheTimeHeld(t *testing.T) {
	due := berlin(11, 24, 20, 0)
	reported := berlin(11, 25, 9, 0)
	now := reported.Add(50 * time.Hour)
	assert.True(t, due.Add(50*time.Hour).Equal(StudyHeldDue(due, reported, now)))
	assert.True(t, due.Equal(StudyHeldDue(due, reported, reported.Add(-time.Hour))), "a clock behind the report moves nothing")
}

func TestStudyReportKindsAndNotes(t *testing.T) {
	for _, name := range []string{"wrong_word", "unsuitable_context"} {
		kind, ok := ParseStudyReportKind(name)
		assert.True(t, ok, name)
		assert.Equal(t, StudyReportKind(name), kind)
	}
	_, ok := ParseStudyReportKind("flag")
	assert.False(t, ok)

	note, ok := CleanStudyReportNote("  kam … umhin \n")
	assert.True(t, ok)
	assert.Equal(t, "kam … umhin", note)
	_, ok = CleanStudyReportNote(strings.Repeat("ä", StudyReportNoteMax))
	assert.True(t, ok, "the bound counts characters, not bytes")
	_, ok = CleanStudyReportNote(strings.Repeat("ä", StudyReportNoteMax+1))
	assert.False(t, ok)
}

func TestOnlyAnOpenWrongWordReportHolds(t *testing.T) {
	assert.True(t, StudyReport{Kind: StudyReportWrongWord}.Held())
	assert.False(t, StudyReport{Kind: StudyReportUnsuitableContext}.Held())
	assert.False(t, StudyReport{Kind: StudyReportWrongWord, EndedAt: time.Now()}.Held())
	assert.True(t, StudyReportEndAnalysisUpdate.Repair())
	assert.False(t, StudyReportEndDismissed.Repair())
	assert.False(t, StudyReportEndUndone.Repair())
}
