//go:build integration

package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStudyAlreadyKnownOnAnIntroductionEntersTheWordMatureWithoutAGrade(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.post("/study/batches/take", url.Values{"language": {"de"}})
	taken := s.clock.Now()

	page := getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String()
	assert.Contains(t, page, "Already known", "offered on the introduction review")

	first := s.currentCard()
	answer := func(card domain.StudyProgress) *httptest.ResponseRecorder {
		return s.post("/study/already-known", url.Values{"language": {"de"}, "progress": {card.ID}, "reps": {strconv.FormatInt(card.Memory.Reps, 10)}})
	}
	require.Equal(t, http.StatusSeeOther, answer(first).Code)

	known := s.progressByID(first.ID)
	assert.Equal(t, domain.StudyStateReview, known.Memory.State)
	assert.GreaterOrEqual(t, known.Memory.ScheduledDays, int64(domain.FluencyMaturityDays))
	assert.True(t, domain.IsStudyMature(known), "Mature at once")
	assert.Empty(t, s.reviewLog(first.ID), "never a grade")
	events, err := s.store.ListStudyAlreadyKnownEvents(s.ctx, s.owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, first.StudyItem, events[0].StudyItem)
	assert.Equal(t, domain.StudyDay(taken), events[0].StudyDay)

	// A repeated tap answers it once.
	answer(first)
	events, err = s.store.ListStudyAlreadyKnownEvents(s.ctx, s.owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, events, 1)

	// Its place in the batch is complete: the other word is the next card, and
	// the answered one is not due until its interval has passed.
	second := s.currentCard()
	require.NotEqual(t, first.ID, second.ID)

	// Not offered on a later review: Again keeps the second word learning, and
	// answering it Already known is refused.
	s.post("/study/grades", url.Values{"language": {"de"}, "progress": {second.ID}, "reps": {"0"}, "grade": {"again"}})
	s.clock.Advance(5 * time.Minute)
	page = getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String()
	assert.NotContains(t, page, "Already known")
	repeated := s.progressByID(second.ID)
	answer(repeated)
	assert.Equal(t, domain.StudyStateLearning, s.progressByID(second.ID).Memory.State)

	// Later reviews are ordinary, and a lapse ends maturity.
	s.clock.Advance(time.Duration(known.Memory.ScheduledDays) * 24 * time.Hour)
	card := s.progressByID(first.ID)
	require.True(t, domain.StudyCardIsDue(card, s.clock.Now()))
	assert.NotContains(t, getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String(), "Already known")
	s.post("/study/grades", url.Values{"language": {"de"}, "progress": {card.ID}, "reps": {strconv.FormatInt(card.Memory.Reps, 10)}, "grade": {"again"}})
	lapsed := s.progressByID(first.ID)
	assert.Equal(t, domain.StudyStateRelearning, lapsed.Memory.State)
	assert.False(t, domain.IsStudyMature(lapsed))
	assert.Len(t, s.reviewLog(first.ID), 1)
}
