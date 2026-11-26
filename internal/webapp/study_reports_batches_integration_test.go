//go:build integration

package webapp

import (
	"net/http"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *studyHarness) studiedLemmas() []string {
	s.t.Helper()
	var lemmas []string
	for _, item := range s.progress() {
		lemmas = append(lemmas, item.CanonicalLemma)
	}
	return lemmas
}

func TestStudyReportingANewWordLeavesItUnintroducedAndOutOfLaterBatches(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.take()
	baum := s.word("baum")
	require.Zero(t, baum.Memory.Reps, "the introduction card has no graded review")

	require.Equal(t, http.StatusSeeOther, s.report(baum, "wrong_word", "not a tree").Code)

	reports := s.openReports()
	require.Len(t, reports, 1)
	assert.Equal(t, "baum", reports[0].CanonicalLemma)
	assert.Equal(t, baum.MaterialID, reports[0].MaterialID, "the report and its material survive")
	assert.NotContains(t, s.studiedLemmas(), "baum", "the held new word has no Study progress")
	assert.Len(t, s.progress(), 9)
	assert.Empty(t, s.reviewLog(baum.ID))
	assert.Contains(t, s.page(), "1 word is held because you reported it.", "the held new word is counted without Study progress")

	s.gradeAll()
	assert.Equal(t, domain.StudyBatchLedger{StudyDay: "2026-11-25", Offered: 1, Taken: 1}, s.ledger())
	assert.Contains(t, s.page(), "2 new words are ready to learn.", "the held word is not offered again")
	s.take()
	s.gradeAll()
	assert.NotContains(t, s.studiedLemmas(), "baum")
	assert.Len(t, s.progress(), 11, "the batch filled from the next candidates")
	assert.Equal(t, domain.StudyBatchLedger{StudyDay: "2026-11-25", Offered: 2, Taken: 2}, s.ledger(), "the daily cap counts the batches as before")
	assert.NotContains(t, s.page(), "study-offer", "only the held word remains")
}

func TestStudyAHeldNewWordFromATakenBatchDoesNotReturnToTheFrontOfTheStream(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.take()
	require.Equal(t, http.StatusSeeOther, s.report(s.word("baum"), "wrong_word", "").Code)

	// The nine other words are never answered, so the next Study day carries
	// them over: they lead the batch and the held word does not.
	s.clock.Set(berlinTime(t, 26, 9, 0))
	s.take()

	lemmas := s.studiedLemmas()
	assert.NotContains(t, lemmas, "baum")
	assert.Contains(t, lemmas, "wald", "the batch filled from the next candidate")
	assert.Len(t, lemmas, 10)

	// Dismissing the report releases the word back to the stream.
	require.NoError(t, s.store.DismissStudyReport(s.ctx, s.owner.ID, s.openReports()[0].ID, s.clock.Now()))
	s.gradeAll()
	assert.Contains(t, s.page(), "2 new words are ready to learn.", "wasser and the released word")
}

func TestStudyRepeatedReportOfAnUnintroducedWordIsAHarmlessNoOp(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.take()
	baum := s.word("baum")
	require.Equal(t, http.StatusSeeOther, s.report(baum, "wrong_word", "not a tree").Code)
	require.NotContains(t, s.studiedLemmas(), "baum", "the first report left the word with no Study progress")

	// A double tap posts the same card again: not a missing word, only Today.
	again := s.report(baum, "wrong_word", "not a tree")
	require.Equal(t, http.StatusSeeOther, again.Code)
	assert.Equal(t, "/de/study", again.Header().Get("Location"))
	assert.Len(t, s.openReports(), 1, "one report")
	assert.Equal(t, http.StatusNotFound, s.report(domain.StudyProgress{ID: "00000000-0000-4000-8000-000000000000", MaterialID: "00000000-0000-4000-8000-0000000000bb"}, "wrong_word", "").Code, "a word that was never the learner's is still not found")
}
