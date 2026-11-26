//go:build integration

package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *studyHarness) report(card domain.StudyProgress, kind, note string) *httptest.ResponseRecorder {
	s.t.Helper()
	return s.post("/study/reports", s.reportForm(card, kind, note))
}

// reportForm is the form a card's Report control submits.
func (s *studyHarness) reportForm(card domain.StudyProgress, kind, note string) url.Values {
	return url.Values{"language": {"de"}, "progress": {card.ID}, "reps": {strconv.FormatInt(card.Memory.Reps, 10)}, "material": {card.MaterialID}, "kind": {kind}, "note": {note}}
}

func (s *studyHarness) openReports() []domain.StudyReport {
	s.t.Helper()
	reports, err := s.store.ListOpenStudyReports(s.ctx, s.owner.ID, "de", domain.StudyReportWrongWord)
	require.NoError(s.t, err)
	return reports
}

// twoReviewsDue introduces both words, answers them Got it and moves to the
// next Study day, when both reviews are due.
func (s *studyHarness) twoReviewsDue() {
	s.t.Helper()
	s.clock.Set(berlinTime(s.t, 25, 22, 0))
	s.introduceAllGotIt()
	s.clock.Set(berlinTime(s.t, 26, 9, 0))
	require.Equal(s.t, 2, s.today().Remaining)
}

func TestStudyReportingADueWordHoldsItWithoutGradingAndAdvances(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	before := s.reviewLog(card.ID)

	response := s.report(card, "wrong_word", "  kam … umhin  ")
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/de/study", response.Header().Get("Location"), "the learner goes on to the next card")

	reports := s.openReports()
	require.Len(t, reports, 1)
	assert.Equal(t, card.ID, reports[0].ProgressID)
	assert.Equal(t, "kam … umhin", reports[0].Note)
	held := s.progressByID(card.ID)
	assert.True(t, held.Held)
	assert.Equal(t, card.Memory.Reps, held.Memory.Reps, "the review is not graded")
	assert.True(t, card.Memory.Due.Equal(held.Memory.Due), "and the schedule does not move")
	assert.Len(t, s.reviewLog(card.ID), len(before), "no review-log row is written")

	assert.Equal(t, 1, s.today().Remaining, "a held word is not in the finish line")
	assert.NotEqual(t, card.ID, s.currentCard().ID, "the next card is the other word")
	page := s.page()
	assert.Contains(t, page, "1 review left today.")
	assert.Contains(t, page, "1 word is held because you reported it.")

	// The other word is answered: Today finishes with the held word outstanding.
	s.grade(s.currentCard(), "got-it")
	assert.True(t, s.today().Finished)
	assert.Contains(t, s.page(), "Today is finished.")

	// Reporting the same card again, or after the word is held, yields one report.
	require.Equal(t, http.StatusSeeOther, s.report(card, "wrong_word", "again").Code)
	assert.Len(t, s.openReports(), 1)
}

func TestStudyReportedWordsListsOpenReportsOfTheLearnerAndLanguage(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	empty := getAs(t, s.h, "/de/study/reports", s.cookies)
	require.Equal(t, http.StatusOK, empty.Code)
	assert.Contains(t, empty.Body.String(), "No words are held.")

	card := s.currentCard()
	require.Equal(t, http.StatusSeeOther, s.report(card, "wrong_word", "wrong lemma").Code)
	list := getAs(t, s.h, "/de/study/reports", s.cookies)
	require.Equal(t, http.StatusOK, list.Code)
	body := list.Body.String()
	assert.Contains(t, body, "wrong lemma")
	assert.Contains(t, body, "2026-11-26", "the date of the report")
	assert.Contains(t, body, "Study reading", "the Book the occurrence is in")
	assert.Contains(t, body, s.reportedSentenceMark(card), "the sentence with the word marked")
	unprefixed := getAs(t, s.h, "/study/reports", s.cookies)
	assert.Equal(t, http.StatusSeeOther, unprefixed.Code, "the unprefixed route redirects to the language's page")
	assert.Equal(t, "/de/study/reports", unprefixed.Header().Get("Location"))
	assert.Contains(t, s.page(), "Reported words")

	other := createAccount(t, s.ctx, s.store, "other-learner", "other-password")
	otherCookies, _ := loginCookies(t, s.h, other.Username, "other-password")
	assert.NotContains(t, getAs(t, s.h, "/de/study/reports", otherCookies).Body.String(), "wrong lemma", "reports belong to their learner")
}

// reportedSentenceMark is the marked target word the Reported words page shows.
func (s *studyHarness) reportedSentenceMark(card domain.StudyProgress) string {
	s.t.Helper()
	material, err := s.store.GetStudyMaterial(s.ctx, s.owner.ID, card.MaterialID)
	require.NoError(s.t, err)
	return `<mark class="study-card__target">` + material.TargetWord + `</mark>`
}

func TestStudyReportIsRefusedForAStaleOrUnknownRequest(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()

	long := strings.Repeat("x", domain.StudyReportNoteMax+1)
	for _, tc := range []struct {
		name     string
		form     url.Values
		wantCode int
	}{
		{"an unknown kind", s.reportForm(card, "bug", ""), http.StatusBadRequest},
		{"a note over the bound", s.reportForm(card, "wrong_word", long), http.StatusBadRequest},
		{"a language the learner does not study", func() url.Values {
			form := s.reportForm(card, "wrong_word", "")
			form.Set("language", "la")
			return form
		}(), http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := s.post("/study/reports", tc.form)
			require.Equal(t, tc.wantCode, response.Code)
			if tc.wantCode == http.StatusBadRequest {
				require.Contains(t, response.Body.String(), "Back to Study", "the error says what to do next")
			}
			require.Empty(t, s.openReports())
		})
	}
	forged := perform(t, s.h, http.MethodPost, "/study/reports", url.Values{"csrf_token": {"forged"}, "language": {"de"}, "progress": {card.ID}, "reps": {"1"}, "kind": {"wrong_word"}}, s.cookies)
	assert.Equal(t, http.StatusForbidden, forged.Code)
	assert.Equal(t, http.StatusNotFound, s.report(domain.StudyProgress{ID: "00000000-0000-4000-8000-000000000000"}, "wrong_word", "").Code)
	assert.Empty(t, s.openReports())

	// A card graded since it was shown is not reported.
	s.grade(card, "got-it")
	require.Equal(t, http.StatusSeeOther, s.report(card, "wrong_word", "").Code)
	assert.Empty(t, s.openReports(), "a stale card is not reported")
	assert.False(t, s.progressByID(card.ID).Held)
}

func TestStudyCardOffersReportOnlyAfterShowAnswer(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	question := getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.NotContains(t, question, `name="kind"`, "Report waits for Show answer")
	answer := getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String()
	assert.Contains(t, answer, `action="/study/reports"`)
	assert.Contains(t, answer, `value="wrong_word"`)
	assert.Contains(t, answer, `value="unsuitable_context"`)
	assert.NotContains(t, answer, `value="unsuitable_context" disabled`, "both kinds are available")
}

func TestStudyTodayWithOpenReportsStaysFastOnTheScaleLibrary(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	s.report(s.currentCard(), "wrong_word", "")
	s.inflateLibrary(studyScaleIdentities)
	s.clock.Set(s.clock.Now().Add(time.Minute))
	s.settle()
	s.timedPage("Today with an open report")
	s.within("the Reported words page", func() int { return getAs(t, s.h, "/de/study/reports", s.cookies).Code }, http.StatusOK)
	s.timedPost("reporting a word", "/study/reports", url.Values{"language": {"de"}, "progress": {s.currentCard().ID}, "reps": {"1"}, "kind": {"wrong_word"}, "note": {"x"}})
}

func (s *studyHarness) dismiss(report domain.StudyReport) *httptest.ResponseRecorder {
	s.t.Helper()
	return s.post("/study/reports/"+report.ID+"/dismiss", url.Values{"language": {"de"}})
}

func TestStudyDismissingAReportReleasesTheWordLaterByTheTimeItWasHeld(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	require.Equal(t, http.StatusSeeOther, s.report(card, "wrong_word", "").Code)
	report := s.openReports()[0]
	logs := len(s.reviewLog(card.ID))
	materialBefore, err := s.store.GetStudyMaterial(s.ctx, s.owner.ID, card.MaterialID)
	require.NoError(t, err)

	held := 30 * time.Hour
	s.clock.Set(report.CreatedAt.Add(held))
	response := s.dismiss(report)
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/de/study/reports", response.Header().Get("Location"))

	assert.Empty(t, s.openReports(), "the entry leaves Reported words")
	ended, err := s.store.GetStudyReport(s.ctx, s.owner.ID, report.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyReportEndDismissed, ended.EndReason)
	word := s.progressByID(card.ID)
	assert.False(t, word.Held)
	assert.True(t, card.Memory.Due.Add(held).Equal(word.Memory.Due), "due moved from %v by %v, got %v", card.Memory.Due, held, word.Memory.Due)
	assert.Equal(t, card.Memory.Reps, word.Memory.Reps)
	assert.InDelta(t, card.Memory.Stability, word.Memory.Stability, 0)
	assert.Equal(t, card.Memory.State, word.Memory.State)
	assert.Equal(t, card.MaterialID, word.MaterialID, "the material is unchanged")
	assert.Equal(t, card.LearnedMeaning, word.LearnedMeaning, "the Learned meaning is unchanged")
	unchanged, shifted := card.Memory, word.Memory
	unchanged.Due, shifted.Due = time.Time{}, time.Time{}
	assert.Equal(t, unchanged, shifted, "the FSRS state is otherwise unchanged")
	assert.Len(t, s.reviewLog(card.ID), logs, "no review-log row is written")
	materialAfter, err := s.store.GetStudyMaterial(s.ctx, s.owner.ID, card.MaterialID)
	require.NoError(t, err)
	assert.Equal(t, materialBefore, materialAfter)

	// A second tap on the same entry changes nothing.
	require.Equal(t, http.StatusSeeOther, s.dismiss(report).Code)
	assert.True(t, card.Memory.Due.Add(held).Equal(s.progressByID(card.ID).Memory.Due))
}

func TestStudyDismissIsRefusedForAnotherLearnerLanguageOrForgedRequest(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	require.Equal(t, http.StatusSeeOther, s.report(card, "wrong_word", "").Code)
	report := s.openReports()[0]

	forged := perform(t, s.h, http.MethodPost, "/study/reports/"+report.ID+"/dismiss", url.Values{"csrf_token": {"forged"}, "language": {"de"}}, s.cookies)
	assert.Equal(t, http.StatusForbidden, forged.Code)
	assert.Equal(t, http.StatusNotFound, s.post("/study/reports/"+report.ID+"/dismiss", url.Values{"language": {"la"}}).Code)
	assert.Equal(t, http.StatusNotFound, s.post("/study/reports/"+report.ID+"/dismiss", url.Values{"language": {"it"}}).Code, "the report is of another language")
	assert.Equal(t, http.StatusNotFound, s.post("/study/reports/00000000-0000-4000-8000-000000000000/dismiss", url.Values{"language": {"de"}}).Code)
	assert.Len(t, s.openReports(), 1)

	other := createAccount(t, s.ctx, s.store, "other-learner", "other-password")
	otherCookies, _ := loginCookies(t, s.h, other.Username, "other-password")
	foreign := perform(t, s.h, http.MethodPost, "/study/reports/"+report.ID+"/dismiss", url.Values{"csrf_token": {otherCookies[0].Value}, "language": {"de"}}, otherCookies)
	assert.Equal(t, http.StatusNotFound, foreign.Code)
	assert.Len(t, s.openReports(), 1)
	assert.True(t, s.progressByID(card.ID).Held)
}

func TestStudyReportOfReplacedMaterialIsStaleUnderTheLock(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()

	form := s.reportForm(card, "wrong_word", "")
	form.Set("material", "00000000-0000-4000-8000-0000000000aa")
	response := s.post("/study/reports", form)
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/de/study", response.Header().Get("Location"))
	assert.Empty(t, s.openReports(), "a card of other material is not reported")
	assert.False(t, s.progressByID(card.ID).Held)
}
