//go:build integration

package webapp

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reportedHaus prepares a Book of two words, answers both once, and reports the
// due word "haus" as a wrong word on the next Study day. It returns the Book,
// the word as reported and the sentence it was reported in.
func (s *studyHarness) reportedHaus(occurrences int) (bookID string, word domain.StudyProgress, sentence int64) {
	s.t.Helper()
	bookID = s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: occurrences}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	s.clock.Advance(24 * time.Hour)
	word = s.word("haus")
	require.Equal(s.t, http.StatusSeeOther, s.report(word, "wrong_word", "kam … umhin").Code)
	require.True(s.t, s.word("haus").Held)
	return bookID, word, s.material(word.MaterialID).SentenceOrdinal
}

func (s *studyHarness) reportOf(id string) domain.StudyReport {
	s.t.Helper()
	report, err := s.store.GetStudyReport(s.ctx, s.owner.ID, id)
	require.NoError(s.t, err)
	return report
}

// requireReleased checks a repaired word: its report ended for the reason, it is
// no longer held, and it is due at the start of the next Study day with the rest
// of its Study progress kept.
func (s *studyHarness) requireReleased(id string, before domain.StudyProgress, reason domain.StudyReportEnd) {
	s.t.Helper()
	report := s.reportOf(id)
	assert.Equal(s.t, reason, report.EndReason)
	assert.False(s.t, report.Open())
	released := s.word("haus")
	assert.False(s.t, released.Held)
	assert.True(s.t, domain.NextStudyDayStart(s.clock.Now()).Equal(released.Memory.Due), "due %v", released.Memory.Due)
	memory, was := released.Memory, before.Memory
	memory.Due, was.Due = time.Time{}, time.Time{}
	assert.Equal(s.t, was, memory, "the FSRS state is kept")
	assert.Len(s.t, s.reviewLog(before.ID), 1)
	assert.Empty(s.t, s.openReports())
}

func TestStudyLemmaCorrectionOfTheReportedOccurrenceEndsTheReportAndReintroducesTheWord(t *testing.T) {
	s := newStudyHarness(t)
	bookID, word, sentence := s.reportedHaus(3)
	id := s.openReports()[0].ID

	// The Book's Lemma review shows the report and marks its occurrence.
	review := getAs(t, s.h, "/reading/books/"+bookID+"/lemma-review?form=haus", s.cookies).Body.String()
	assert.Contains(t, review, "Reported in Study")
	assert.Contains(t, review, "kam … umhin")
	assert.Contains(t, review, "Reported as a wrong word in Study.")
	assert.Equal(t, 1, strings.Count(review, "Reported as a wrong word in Study."), "only the reported occurrence is marked")
	assert.Contains(t, getAs(t, s.h, "/de/study/reports", s.cookies).Body.String(), "/reading/books/"+bookID+"/lemma-review?form=")

	// A decision about another occurrence of the form leaves the report open.
	other := sentence + 1
	if other >= 3 {
		other = sentence - 1
	}
	s.decide(bookID, "haus", other, "keep", "")
	assert.True(t, s.reportOf(id).Open())

	s.decide(bookID, "haus", sentence, "correct", "hof")
	s.requireReleased(id, word, domain.StudyReportEndCorrected)
	assert.Equal(t, 1, s.today().Remaining, "only the other word is due today")

	// At the start of the next Study day it is re-introduced in new material.
	s.clock.Advance(24 * time.Hour)
	s.grade(s.word("buch"), "got-it")
	s.page()
	assert.True(t, s.word("haus").Reintroducing)
	testutil.Eventually(t, 60*time.Second, "the word to be re-introduced", func() (bool, string) {
		s.page()
		return s.word("haus").MaterialID != word.MaterialID, "the word is still waiting for new material"
	})
	reintroduced := s.word("haus")
	assert.True(t, reintroduced.Corrected())
	assert.Equal(t, s.material(reintroduced.MaterialID).Gloss, reintroduced.LearnedMeaning, "the learned meaning is re-anchored")
	assert.Equal(t, word.ID, reintroduced.ID, "the same Study progress")
	assert.Equal(t, word.ID, s.currentCard().ID)
}

func TestStudyOccurrenceExclusionEndsTheReportAndWithdrawsAWordWithNoOtherOccurrence(t *testing.T) {
	s := newStudyHarness(t)
	bookID, word, sentence := s.reportedHaus(1)
	id := s.openReports()[0].ID

	s.decide(bookID, "haus", sentence, "exclude", "")
	s.requireReleased(id, word, domain.StudyReportEndExcluded)
	assert.True(t, s.word("haus").Withdrawn, "no valid occurrence is left")
	assert.Equal(t, word.LearnedMeaning, s.word("haus").LearnedMeaning)
}

func TestStudyAnalysisUpdateEndsAReportOnlyWhenTheOccurrenceChangesIdentity(t *testing.T) {
	s := newStudyHarness(t)
	bookID, word, sentence := s.reportedHaus(3)
	id := s.openReports()[0].ID

	// An update that leaves the identity unchanged keeps the report open.
	newRun := s.republish(bookID, "")
	s.page()
	assert.True(t, s.reportOf(id).Open())
	assert.True(t, s.word("haus").Held)
	assert.Equal(t, newRun, s.material(s.word("haus").MaterialID).AnalysisRunID, "the held word's material follows the occurrence")

	// An update that gives the occurrence another lemma ends it.
	s.republish(bookID, "hof", sentence+10)
	s.page()
	s.requireReleased(id, word, domain.StudyReportEndAnalysisUpdate)
}

func TestStudyOccurrenceThatDisappearsEndsTheReport(t *testing.T) {
	s := newStudyHarness(t)
	_, word, _ := s.reportedHaus(3)
	id := s.openReports()[0].ID
	source := s.material(word.MaterialID)

	_, err := s.store.Pool().Exec(s.ctx, `DELETE FROM corpus_tokens WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=$3 AND token_ordinal=$4`,
		s.owner.ID, source.AnalysisRunID, source.SentenceOrdinal, source.TokenOrdinal)
	require.NoError(t, err)
	s.page()
	s.requireReleased(id, word, domain.StudyReportEndOccurrenceGone)
}
