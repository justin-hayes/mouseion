//go:build integration

package webapp

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *studyHarness) grade(card domain.StudyProgress, grade string) {
	s.t.Helper()
	response := s.post("/study/grades", url.Values{"language": {"de"}, "progress": {card.ID}, "reps": {strconv.FormatInt(card.Memory.Reps, 10)}, "grade": {grade}})
	require.Equal(s.t, http.StatusSeeOther, response.Code)
}

func (s *studyHarness) page() string {
	s.t.Helper()
	return getAs(s.t, s.h, "/de/study", s.cookies).Body.String()
}

func (s *studyHarness) today() domain.StudyToday {
	return domain.StudyFinishLine(s.progress(), s.clock.Now())
}

// berlinTime is a wall-clock time in the Study day's zone.
func berlinTime(t *testing.T, day, hour, minute int) time.Time {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	return time.Date(2026, time.November, day, hour, minute, 0, 0, zone)
}

// introduce takes today's batch and answers every word Got it.
func (s *studyHarness) introduceAllGotIt() {
	s.t.Helper()
	require.Equal(s.t, http.StatusSeeOther, s.post("/study/batches/take", url.Values{"language": {"de"}}).Code)
	for range s.progress() {
		s.grade(s.currentCard(), "got-it")
	}
}

func TestStudyRepeatBeforeTheBoundaryIsNotLostAndTheDayCountsReviewsAfterMidnight(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	// Day one: introduce both words and answer them Got it. Both are due when
	// the next Study day starts.
	s.clock.Set(berlinTime(t, 25, 22, 0))
	s.introduceAllGotIt()
	dayTwo := berlinTime(t, 26, 5, 0)

	// Just before the boundary the finished day stays finished.
	s.clock.Set(dayTwo.Add(-time.Minute))
	assert.True(t, s.today().Finished)
	assert.Contains(t, s.page(), "Today is finished.")

	// The new Study day starts with both reviews due.
	s.clock.Set(dayTwo)
	assert.Equal(t, 2, s.today().Remaining)
	assert.Contains(t, s.page(), "2 reviews left today.", "the count is shown while a card is answered")

	// Day two runs past midnight: a review answered at 00:30 on 27 November
	// still belongs to the day that began on 26 November.
	s.clock.Set(berlinTime(t, 27, 0, 30))
	assert.Equal(t, 2, s.today().Remaining, "the Study day that is ending still has its reviews")
	first := s.currentCard()
	s.grade(first, "got-it")
	logs := s.reviewLog(first.ID)
	require.Len(t, logs, 2)
	assert.Equal(t, "2026-11-26", logs[1].StudyDay, "a review after midnight counts towards the day that is ending")
	assert.Equal(t, 1, s.today().Remaining)

	// The other review is forgotten five minutes before the boundary. Its
	// repeat would fall after 05:00, but it stays part of Today and is due as
	// the new Study day starts.
	s.clock.Set(berlinTime(t, 27, 4, 55))
	second := s.currentCard()
	require.NotEqual(t, first.ID, second.ID)
	s.grade(second, "again")
	assert.Equal(t, domain.StudyStateRelearning, s.progressByID(second.ID).Memory.State)
	assert.Equal(t, 1, s.today().Remaining, "the repeat is still to answer")
	assert.NotContains(t, s.page(), "Today is finished.")
	assert.NotContains(t, s.page(), "Take these words")
	assert.Contains(t, s.page(), "1 review left today.")

	boundary := berlinTime(t, 27, 5, 0)
	s.clock.Set(boundary.Add(-time.Second))
	_, err := s.store.NextStudyCard(s.ctx, s.owner.ID, "de", s.clock.Now())
	require.ErrorIs(t, err, domain.ErrNotFound)
	s.clock.Set(boundary)
	assert.Equal(t, second.ID, s.currentCard().ID, "the repeat is not pushed to a later Study day")
	s.grade(s.currentCard(), "got-it")
	assert.True(t, s.today().Finished)
	assert.Contains(t, s.page(), "Today is finished.")
}

func TestStudyTodayStaysFinishedWhenMoreMaterialBecomesReady(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.introduceAllGotIt()
	s.clock.Set(berlinTime(t, 26, 9, 0))
	require.Equal(t, 2, s.today().Remaining)
	for range 2 {
		s.grade(s.currentCard(), "got-it")
	}
	require.True(t, s.today().Finished)

	// More words are analysed and prepared; the finish line does not move.
	s.seedBook("more", testutil.CorpusWord{Lemma: "tisch", UPOS: "NOUN", Occurrences: 2}, testutil.CorpusWord{Lemma: "stuhl", UPOS: "NOUN", Occurrences: 2})
	require.Equal(t, http.StatusOK, getAs(t, s.h, "/de/study", s.cookies).Code)
	testutil.Eventually(t, 60*time.Second, "new material to settle", func() (bool, string) {
		return s.counts().Preparing == 0, "preparing words remain"
	})
	for _, now := range []time.Time{berlinTime(t, 26, 9, 30), berlinTime(t, 26, 23, 59), berlinTime(t, 27, 0, 30), berlinTime(t, 27, 4, 59)} {
		s.clock.Set(now)
		assert.Equal(t, domain.StudyToday{Finished: true}, s.today(), now.String())
		_, err := s.store.NextStudyCard(s.ctx, s.owner.ID, "de", now)
		require.ErrorIs(t, err, domain.ErrNotFound, now.String())
	}
	// The new Study day brings the reviews back.
	s.clock.Set(berlinTime(t, 27, 5, 0))
	assert.Equal(t, domain.StudyToday{Remaining: 0, Finished: true}, s.today(), "Got it reviews are scheduled days ahead")
	s.clock.Set(berlinTime(t, 26, 9, 30))
	page := s.page()
	assert.Contains(t, page, `id="study-finished"`, "the finish line is stated even when a batch is on offer")
	assert.Contains(t, page, "Take these words")
}

func TestStudyAfterThreeMissedDaysEveryOverdueReviewIsDue(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.introduceAllGotIt()
	s.clock.Set(berlinTime(t, 29, 8, 0))
	assert.Equal(t, 2, s.today().Remaining, "both words are overdue")
	page := s.page()
	assert.Contains(t, page, "Show answer")
	assert.NotContains(t, page, "Take these words", "new words wait for the reviews")
	s.grade(s.currentCard(), "got-it")
	s.grade(s.currentCard(), "got-it")
	assert.True(t, s.today().Finished)
}

func TestStudyAcrossDaylightSavingChangesTheBoundaryStaysAtFiveBerlin(t *testing.T) {
	cases := []struct {
		name         string
		answered     time.Time
		utcBoundary  time.Time
		studyDayName string
	}{
		{"spring forward", time.Date(2026, 3, 28, 21, 0, 0, 0, time.UTC), time.Date(2026, 3, 29, 3, 0, 0, 0, time.UTC), "2026-03-28"},
		{"autumn back", time.Date(2026, 10, 24, 21, 0, 0, 0, time.UTC), time.Date(2026, 10, 25, 4, 0, 0, 0, time.UTC), "2026-10-24"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStudyHarness(t)
			s.prepared()
			s.clock.Set(tc.answered)
			s.introduceAllGotIt()
			logs := s.reviewLog(s.progress()[0].ID)
			require.Len(t, logs, 1)
			assert.Equal(t, tc.studyDayName, logs[0].StudyDay)
			due := s.progress()[0].Memory.Due
			assert.True(t, tc.utcBoundary.Equal(due), "Got it on first sight is due at 05:00 Berlin: %v", due)

			s.clock.Set(tc.utcBoundary.Add(-time.Second))
			assert.True(t, s.today().Finished, "the day that is ending is finished")
			assert.Contains(t, s.page(), "Today is finished.")
			s.clock.Set(tc.utcBoundary)
			assert.Equal(t, 2, s.today().Remaining, "the new Study day starts with the reviews due")
			for range 2 {
				s.grade(s.currentCard(), "got-it")
			}
			assert.True(t, s.today().Finished)
			for _, item := range s.progress() {
				assert.Len(t, s.reviewLog(item.ID), 2, "each review is answered once, never doubled")
			}
		})
	}
}

func TestStudyProgressIsUnchangedByTheReadingLifecycle(t *testing.T) {
	s := newStudyHarness(t)
	book := s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	other := s.seedBook("other", testutil.CorpusWord{Lemma: "tisch", UPOS: "NOUN", Occurrences: 1})
	require.Equal(t, http.StatusOK, getAs(t, s.h, "/de/study", s.cookies).Code)
	require.NoError(t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(t, "study River client", s.client.Stop)
	testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) {
		return s.counts().Preparing == 0, "preparing words remain"
	})
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.introduceAllGotIt()
	s.clock.Set(berlinTime(t, 26, 9, 0))
	s.grade(s.currentCard(), "again")
	before := s.progress()
	today := s.today()
	require.Equal(t, 3, today.Remaining)

	current := func() domain.CurrentReading {
		reading, err := s.store.GetCurrentReading(s.ctx, s.owner.ID, "de")
		require.NoError(t, err)
		return reading
	}
	start := perform(t, s.h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {s.cookies[0].Value}}, s.cookies)
	require.Equal(t, http.StatusSeeOther, start.Code, start.Body.String())
	reading := current()
	require.Equal(t, book.ID, reading.BookID)
	assert.Equal(t, before, s.progress(), "starting a reading changes no word")

	expect := func(r domain.CurrentReading) url.Values {
		return url.Values{"csrf_token": {s.cookies[0].Value}, "expected_current_book_id": {r.BookID}, "expected_current_snapshot_id": {r.SnapshotID}}
	}
	switched := perform(t, s.h, http.MethodPost, "/reading/books/"+other.ID+"/switch", expect(reading), s.cookies)
	require.Equal(t, http.StatusSeeOther, switched.Code, switched.Body.String())
	reading = current()
	require.Equal(t, other.ID, reading.BookID)
	assert.Equal(t, before, s.progress(), "switching the reading changes no word")

	ended := perform(t, s.h, http.MethodPost, "/reading/end", expect(reading), s.cookies)
	require.Equal(t, http.StatusSeeOther, ended.Code, ended.Body.String())
	assert.Empty(t, current().BookID)
	assert.Equal(t, before, s.progress(), "ending a reading changes no word")

	require.Equal(t, http.StatusSeeOther, perform(t, s.h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {s.cookies[0].Value}}, s.cookies).Code)
	reading = current()
	finished := perform(t, s.h, http.MethodPost, "/reading/finish", expect(reading), s.cookies)
	require.Equal(t, http.StatusOK, finished.Code, finished.Body.String())
	assert.Empty(t, current().BookID)
	assert.Equal(t, before, s.progress(), "finishing a Book takes none of its words out of study and changes no due time")
	assert.Equal(t, today, s.today())
}

// A review belongs to the Study day that contains its due time, so one due in
// the evening is due from the start of that day.
func TestStudyReviewDueInTheEveningIsDueFromTheStartOfThatStudyDay(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.introduceAllGotIt()
	s.clock.Set(berlinTime(t, 26, 20, 0))
	first := s.currentCard()
	s.grade(first, "got-it")
	s.grade(s.currentCard(), "got-it")
	due := s.progressByID(first.ID).Memory.Due
	dayStart := domain.StudyDayStart(due)
	assert.Equal(t, 20, due.In(dayStart.Location()).Hour(), "FSRS due is the review time plus whole days")

	s.clock.Set(dayStart.Add(-time.Second))
	_, err := s.store.NextStudyCard(s.ctx, s.owner.ID, "de", s.clock.Now())
	require.ErrorIs(t, err, domain.ErrNotFound, "the day before, nothing of it is due")
	s.clock.Set(dayStart)
	assert.Equal(t, first.ID, s.currentCard().ID, "due at 20:00, the review is due from 05:00 that day")
	assert.NotZero(t, s.today().Remaining)
	assert.Contains(t, s.page(), "reviews left today.")
	for !s.today().Finished {
		s.grade(s.currentCard(), "got-it")
	}
}
