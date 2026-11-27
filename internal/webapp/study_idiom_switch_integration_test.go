//go:build integration

package webapp

import (
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// answerPage is the Study page with the current card's answer shown.
func (s *studyHarness) answerPage() string {
	s.t.Helper()
	return getAs(s.t, s.h, "/de/study?answer=1", s.cookies).Body.String()
}

func TestStudyAnIdiomCardOffersNoReportWhileTheSwitchIsOff(t *testing.T) {
	s, on := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	card := s.showIdiomCard()
	require.True(t, card.IsIdiom())

	t.Run("the Report control is offered while the switch is on", func(t *testing.T) {
		require.Contains(t, s.answerPage(), `id="study-report"`)
	})
	on.Store(false)
	t.Run("the Report control is withheld once it is off", func(t *testing.T) {
		page := s.answerPage()
		require.Contains(t, page, `id="study-gloss"`, "the answer is still shown")
		require.NotContains(t, page, `id="study-report"`)
	})
}

func TestStudyAnIdiomWaitingForNewMaterialStaysReviewableWhenTheSwitchIsTurnedOff(t *testing.T) {
	s, on := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	card := s.showIdiomCard()
	s.provider.answerIdioms("mit dem haus", idiomFailure)
	require.Equal(t, http.StatusSeeOther, s.report(card, "unsuitable_context", "").Code)
	s.page()
	require.True(t, s.progressByID(card.ID).Reintroducing, "the replacement is not prepared")
	s.clock.Advance(24 * time.Hour)

	on.Store(false)
	s.page()

	kept := s.progressByID(card.ID)
	t.Run("the Idiom is back on its last material", func(t *testing.T) {
		require.False(t, kept.Reintroducing)
		require.Equal(t, card.MaterialID, kept.MaterialID)
	})
	t.Run("its review is due and is graded", func(t *testing.T) {
		require.Equal(t, 1, s.today().Remaining)
		s.grade(s.showIdiomCard(), "got-it")
		require.Len(t, s.reviewLog(card.ID), 2)
	})
}

func TestStudyTodayCountsAnIdiomInStudyLikeAWordWhileTheSwitchIsOff(t *testing.T) {
	s, on := idiomTodayHarness(t)
	s.preparedIdiom()
	testutil.Eventually(t, 30*time.Second, "the words and the Idiom to be ready", func() (bool, string) {
		return s.counts().Ready == 3, "material is still being prepared"
	})
	s.take()
	_, ok := s.progressOfIdiom()
	require.True(t, ok, "the Idiom is in study")
	on.Store(false)

	assert.Contains(t, s.page(), "2 new words and 1 idiom are ready.", "an Idiom in study is not subtracted")
}
