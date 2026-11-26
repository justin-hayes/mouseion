//go:build integration

package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	assert.Equal(t, "/de/study?answer=1", response.Header().Get("Location"))

	assert.Empty(t, s.unsuitableReports(), "the report has ended")
	ended, err := s.store.GetStudyReport(s.ctx, s.owner.ID, report.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyReportEndUndone, ended.EndReason)
	restored := s.progressByID(card.ID)
	assert.Equal(t, original.ID, restored.MaterialID, "the word is back on the reported material")
	assert.False(t, restored.Reintroducing)
	assert.Equal(t, card.CorrectedAt, restored.CorrectedAt, "the correction time is as it was")
	assert.False(t, restored.Withdrawn)
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
	assert.Equal(t, "/de/study", response.Header().Get("Location"), "a refused Undo does not ask for the answer side")
	require.Len(t, s.unsuitableReports(), 1, "the report stands")
	assert.NotContains(t, getAs(t, s.h, "/de/study?reported="+report.ID, s.cookies).Body.String(), `id="study-reported-undo"`, "the offer ends with the next Study action")
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

// lastReviewWithABatchWaiting takes the first batch of ten words, answers them
// all, and on the next Study day answers every review but one: Today is down to
// that review, and the two other words wait as the next batch. It returns the
// review that is left.
func (s *studyHarness) lastReviewWithABatchWaiting() domain.StudyProgress {
	s.t.Helper()
	s.preparedTwelve()
	s.clock.Set(berlinTime(s.t, 25, 9, 0))
	s.take()
	s.gradeAll()
	s.clock.Set(berlinTime(s.t, 26, 9, 0))
	s.gradeAllButOne()
	return s.currentCard()
}

// gradeAllButOne answers every due card Got it except the last one.
func (s *studyHarness) gradeAllButOne() {
	s.t.Helper()
	for s.today().Remaining > 1 {
		s.grade(s.currentCard(), "got-it")
	}
}

func (s *studyHarness) next(id string) *httptest.ResponseRecorder {
	s.t.Helper()
	return s.post("/study/reports/"+id+"/next", url.Values{"language": {"de"}})
}

func TestStudyNextEndsTheUndoOfAnUnsuitableReportAndRetiresIt(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	location := s.report(card, "unsuitable_context", "").Header().Get("Location")
	report := s.unsuitableReports()[0]
	require.Contains(t, getAs(t, s.h, location, s.cookies).Body.String(), `id="study-reported-undo"`)
	assert.Contains(t, getAs(t, s.h, location, s.cookies).Body.String(), "/study/reports/"+report.ID+"/next", "Next is a form")

	forged := perform(t, s.h, http.MethodPost, "/study/reports/"+report.ID+"/next", url.Values{"csrf_token": {"forged"}, "language": {"de"}}, s.cookies)
	assert.Equal(t, http.StatusForbidden, forged.Code)
	assert.Len(t, s.unsuitableReports(), 1)

	response := s.next(report.ID)
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/de/study", response.Header().Get("Location"))
	ended, err := s.store.GetStudyReport(s.ctx, s.owner.ID, report.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyReportEndRetired, ended.EndReason)
	retired, err := s.store.ListRetiredStudyOccurrences(s.ctx, s.owner.ID, card.ID)
	require.NoError(t, err)
	assert.Len(t, retired, 1, "the sentence stays retired")

	// Back on the reported card, Undo is no longer shown or accepted.
	assert.NotContains(t, getAs(t, s.h, location, s.cookies).Body.String(), `id="study-reported-undo"`)
	undone := s.undo(report.ID)
	require.Equal(t, http.StatusSeeOther, undone.Code)
	assert.Equal(t, "/de/study", undone.Header().Get("Location"))
	assert.Equal(t, domain.StudyReportEndRetired, s.reportOf(report.ID).EndReason)
	assert.Equal(t, http.StatusNotFound, s.next("00000000-0000-4000-8000-000000000000").Code)
}

func TestStudyAnotherStudyActionRetiresTheReportWhenTodayIsShown(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	s.report(card, "unsuitable_context", "")
	report := s.unsuitableReports()[0]
	s.grade(s.currentCard(), "got-it")
	s.page()
	assert.Equal(t, domain.StudyReportEndRetired, s.reportOf(report.ID).EndReason)
}

func TestStudyAnUndoScreenOffersNoNewBatchBehindIt(t *testing.T) {
	s := newStudyHarness(t)
	last := s.lastReviewWithABatchWaiting()
	location := s.report(last, "unsuitable_context", "").Header().Get("Location")
	before := s.ledger()

	shown := getAs(t, s.h, location, s.cookies).Body.String()
	require.Contains(t, shown, `id="study-reported-undo"`)
	assert.NotContains(t, shown, "study-offer", "no batch is offered behind the Undo screen")
	assert.Equal(t, before, s.ledger(), "showing the Undo screen counts no batch as offered")
	assert.Contains(t, s.page(), "study-offer", "the batch is on offer once the Undo screen is left")
}

func TestStudyUndoIsRefusedAfterABatchIsTakenOrDeclined(t *testing.T) {
	for _, action := range []string{"take", "decline"} {
		t.Run(action, func(t *testing.T) {
			s := newStudyHarness(t)
			s.report(s.lastReviewWithABatchWaiting(), "unsuitable_context", "")
			report := s.unsuitableReports()[0]
			s.page() // Today shows the batch, which counts it as offered.
			offer := strconv.Itoa(s.ledger().OfferNumber())
			require.Equal(t, http.StatusSeeOther, s.post("/study/batches/"+action, url.Values{"language": {"de"}, "offer": {offer}}).Code)
			require.NotZero(t, s.ledger().Taken+s.ledger().Declined, "the batch was answered")

			response := s.undo(report.ID)
			require.Equal(t, http.StatusSeeOther, response.Code)
			assert.Equal(t, "/de/study", response.Header().Get("Location"))
			assert.Equal(t, domain.StudyReportEndRetired, s.settledReportEnd(report.ID), "the report stands, closed for Undo")
		})
	}
}

// settledReportEnd opens Today, which ends a report the learner has moved on
// from, and says how the report ended.
func (s *studyHarness) settledReportEnd(id string) domain.StudyReportEnd {
	s.t.Helper()
	s.page()
	return s.reportOf(id).EndReason
}

func TestStudyUndoIsRefusedAfterAFluencyResponse(t *testing.T) {
	s := newStudyHarness(t)
	word := s.matureHaus()
	s.openFluency()
	current := s.progressByID(word.ID)
	record := domain.StudyReportRecord{ProgressID: word.ID, MaterialID: current.MaterialID, ExpectedReps: current.Memory.Reps, Kind: domain.StudyReportUnsuitableContext, CreatedAt: s.clock.Now()}
	report, err := s.store.CreateStudyReport(s.ctx, s.owner.ID, record)
	require.NoError(t, err)
	context := s.shownFluencyContext()
	require.NotEmpty(t, context)
	s.answerFluency(context, "recognised")

	response := s.undo(report.ID)
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/de/study", response.Header().Get("Location"))
	assert.Equal(t, domain.StudyReportEndRetired, s.settledReportEnd(report.ID))
}
