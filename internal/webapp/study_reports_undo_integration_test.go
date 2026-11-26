//go:build integration

package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *studyHarness) undo(id string) *httptest.ResponseRecorder {
	s.t.Helper()
	response := s.post("/study/reports/"+id+"/undo", url.Values{"language": {"de"}})
	return response
}

func TestStudyUndoRestoresTheReportedWordAndItsCard(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	original := s.material(card.MaterialID)
	reviews := len(s.reviewLog(card.ID))

	location := s.report(card, "unsuitable_context", "").Header().Get("Location")
	report := s.unsuitableReports()[0]
	shown := getAs(t, s.h, location, s.cookies).Body.String()
	assert.Contains(t, shown, "/study/reports/"+report.ID+"/undo", "the reported card offers Undo")
	assert.Contains(t, shown, `id="study-reported-undo"`)
	s.waitForNewMaterial(card.CanonicalLemma, card.MaterialID)

	response := s.undo(report.ID)
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/de/study", response.Header().Get("Location"))

	assert.Empty(t, s.unsuitableReports(), "the report has ended")
	ended, err := s.store.GetStudyReport(s.ctx, s.owner.ID, report.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyReportEndUndone, ended.EndReason)
	restored := s.progressByID(card.ID)
	assert.Equal(t, original.ID, restored.MaterialID, "the word is back on the reported material")
	assert.False(t, restored.Reintroducing)
	assert.Equal(t, card.Memory, restored.Memory, "Undo never touches the schedule")
	assert.Equal(t, card.ID, s.currentCard().ID, "the reported card is due again")
	assert.Contains(t, s.page(), original.TargetWord, "the reported sentence is shown again")
	assert.Len(t, s.reviewLog(card.ID), reviews, "Undo writes no review-log row")
	assert.NotContains(t, s.page(), `id="study-reported"`)

	// A second tap changes nothing and is not an error.
	assert.Equal(t, http.StatusSeeOther, s.undo(report.ID).Code)
}

func TestStudyUndoIsRefusedOnceAnotherStudyActionFollowed(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	s.report(card, "unsuitable_context", "")
	report := s.unsuitableReports()[0]
	replaced := s.progressByID(card.ID)

	other := s.currentCard()
	require.NotEqual(t, card.ID, other.ID)
	s.grade(other, "got-it")

	response := s.undo(report.ID)
	require.Equal(t, http.StatusSeeOther, response.Code, "a refusal redirects back like a stale grade")
	require.Len(t, s.unsuitableReports(), 1, "the report stands")
	assert.Equal(t, replaced.MaterialID, s.progressByID(card.ID).MaterialID, "nothing changed")
}

func TestStudyUndoOfAnEarlierReportIsRefusedAfterALaterOne(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	first := s.currentCard()
	s.report(first, "unsuitable_context", "")
	second := s.currentCard()
	require.NotEqual(t, first.ID, second.ID)
	s.report(second, "unsuitable_context", "")
	reports := s.unsuitableReports()
	require.Len(t, reports, 2)

	for _, report := range reports {
		if report.ProgressID == first.ID {
			s.undo(report.ID)
		}
	}
	assert.Len(t, s.unsuitableReports(), 2, "a later report refuses the earlier Undo")
}

func TestStudyUndoOfAnotherLearnersReportIsNotFound(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	assert.Equal(t, http.StatusNotFound, s.undo("00000000-0000-4000-8000-000000000000").Code)
}
