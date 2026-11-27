//go:build integration

package webapp

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// idiomTodayHarness is idiomStudyHarness with a switch the test can turn off
// later, as the operator can.
func idiomTodayHarness(t *testing.T) (*studyHarness, *atomic.Bool) {
	t.Helper()
	lookup := newFakeIdiomLookup(germanIdiom("mit dem haus"))
	avoidance := germanIdiomAvoidance(t, lookup, true)
	var on atomic.Bool
	on.Store(true)
	enabled := avoidance.Enabled
	avoidance.Enabled = func(language string) bool { return on.Load() && enabled(language) }
	s := newStudyHarnessWith(t, studyHarnessOptions{capabilities: readyGerman(), idioms: avoidance})
	book := s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.buildIdiomCounts(book, lookup)
	return s, &on
}

// preparedIdiom opens the page and waits until the words and the Idiom are
// prepared.
func (s *studyHarness) preparedIdiom() {
	s.t.Helper()
	s.startStudy()
	testutil.Eventually(s.t, 60*time.Second, "material to settle", func() (bool, string) { return s.idiomSettled() })
}

// progressOfIdiom is the Idiom's Study progress, if it is in study.
func (s *studyHarness) progressOfIdiom() (domain.StudyProgress, bool) {
	s.t.Helper()
	for _, item := range s.progress() {
		if item.IsIdiom() {
			return item, true
		}
	}
	return domain.StudyProgress{}, false
}

// showIdiomCard grades the word cards ahead of the Idiom's Got it and returns
// when the Idiom's card is the one due.
func (s *studyHarness) showIdiomCard() domain.StudyProgress {
	s.t.Helper()
	for range 5 {
		card := s.currentCard()
		if card.IsIdiom() {
			return card
		}
		s.grade(card, "got-it")
	}
	require.Fail(s.t, "the Idiom's card never came up")
	return domain.StudyProgress{}
}

func TestStudyTodayIntroducesAnIdiomAndShowsItsIdiomCard(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.preparedIdiom()

	assert.Empty(t, s.progress(), "no member word of the Idiom is in study")
	assert.Equal(t, domain.StudyMaterialCounts{Ready: 3}, s.counts(), "the Idiom counts as an item")
	assert.Contains(t, s.page(), "3 new words are ready to learn.")
	s.take()

	introduced, ok := s.progressOfIdiom()
	require.True(t, ok, "the batch introduces the prepared Idiom")
	assert.Equal(t, "mit dem haus", introduced.CanonicalLemma)
	assert.Equal(t, "the idiom mit dem haus", introduced.LearnedMeaning, "the introducing Gloss is the Learned meaning")
	assert.Equal(t, domain.StudyStateNew, introduced.Memory.State)
	assert.Len(t, s.progress(), 3)

	var card domain.StudyProgress
	for range 3 {
		card = s.currentCard()
		if card.IsIdiom() {
			break
		}
		wordBack := getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String()
		assert.Contains(t, wordBack, "wrong_word", "a word's Report still offers Wrong word")
		assert.NotContains(t, wordBack, `id="study-kind"`)
		s.grade(card, "got-it")
	}
	require.True(t, card.IsIdiom())
	front := s.page()
	for _, word := range []string{"mit", "dem", "haus"} {
		assert.Contains(t, front, `<mark class="study-card__target"><strong>`+word+`</strong></mark>`, "every matched word is marked")
	}
	assert.NotContains(t, front, "study-kind", "the idiom label is on the answer side")
	assert.NotContains(t, front, "study-gloss")

	back := getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String()
	assert.Contains(t, back, `id="study-headword"`)
	assert.Contains(t, back, `id="study-kind"`)
	assert.Contains(t, back, "the idiom mit dem haus")
	assert.Contains(t, back, "<b>mit</b> <b>dem</b> <b>haus</b>", "the translation is highlighted")
	assert.NotContains(t, back, "Morphology")
	assert.Contains(t, back, `value="unsuitable_context" checked`, "Report offers Unsuitable context")
	assert.NotContains(t, back, "wrong_word", "never Wrong word")
}

func TestStudyTodayGradesAnIdiomLikeAWordWithItsOwnReviewLog(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.preparedIdiom()
	s.take()
	for range 3 {
		s.grade(s.currentCard(), "again")
	}

	idiomProgress, ok := s.progressOfIdiom()
	require.True(t, ok)
	var word domain.StudyProgress
	for _, item := range s.progress() {
		if !item.IsIdiom() && item.CanonicalLemma == "buch" {
			word = item
		}
	}
	assert.Equal(t, word.Memory, idiomProgress.Memory, "Again schedules the Idiom exactly as it schedules a word")
	assert.Equal(t, domain.StudyStateLearning, idiomProgress.Memory.State)
	idiomLog := s.reviewLog(idiomProgress.ID)
	require.Len(t, idiomLog, 1, "its review-log rows are its own")
	assert.Equal(t, domain.StudyGradeAgain, idiomLog[0].Grade)
	assert.Equal(t, idiomProgress.MaterialID, idiomLog[0].MaterialID)
	assert.Equal(t, "the idiom mit dem haus", idiomProgress.LearnedMeaning)

	s.clock.Set(s.clock.Now().Add(time.Hour))
	s.gradeAll()
	idiomProgress, _ = s.progressOfIdiom()
	assert.Len(t, s.reviewLog(idiomProgress.ID), 2)
	assert.Equal(t, domain.StudyStateReview, idiomProgress.Memory.State, "Got it graduates it like a word")
}

func TestStudyTodayReviewsAnIdiomAfterTheSwitchIsTurnedOff(t *testing.T) {
	s, on := idiomTodayHarness(t)
	s.preparedIdiom()
	s.clock.Set(berlinTime(t, 25, 22, 0))
	s.introduceAllGotIt()
	on.Store(false)
	s.clock.Set(berlinTime(t, 26, 9, 0))

	assert.Equal(t, 3, s.today().Remaining, "the Idiom's review is due with the words'")
	card := s.showIdiomCard()
	assert.Equal(t, http.StatusOK, getAs(t, s.h, "/de/study", s.cookies).Code)
	assert.Contains(t, s.page(), `<mark class="study-card__target"><strong>mit</strong></mark>`)
	s.grade(card, "got-it")
	assert.Len(t, s.reviewLog(card.ID), 2)
}

func TestStudyTodayOffersNoNewIdiomWhileTheSwitchIsOff(t *testing.T) {
	s, on := idiomTodayHarness(t)
	s.preparedIdiom()
	on.Store(false)

	assert.Contains(t, s.page(), "2 new words are ready to learn.", "the ready Idiom is not offered")
	s.take()
	_, ok := s.progressOfIdiom()
	assert.False(t, ok)
	assert.Len(t, s.progress(), 2)
}

func TestStudyTodayCountsAnIdiomStillBeingPrepared(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.provider.answerIdioms("mit dem haus", idiomFailure)
	s.startStudy()
	testutil.Eventually(t, 60*time.Second, "the words to be ready and the Idiom to fail once", func() (bool, string) {
		return s.counts().Ready == 2 && len(s.provider.idiomRequests()) > 0, "words or the Idiom still preparing"
	})

	assert.Equal(t, 1, s.counts().Preparing, "the pending Idiom counts as an item")
	page := s.page()
	assert.Contains(t, page, "2 new words are ready to learn.")
	assert.Contains(t, page, `id="study-short-batch"`, "the batch is short because the Idiom is still being prepared")
}

func TestStudyTodayAnIdiomInABatchCountsAgainstTheDailyCap(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.preparedIdiom()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.seedLedger("2026-11-25", 2)

	s.take()
	_, ok := s.progressOfIdiom()
	require.True(t, ok, "the third batch carries the Idiom")
	assert.Equal(t, 3, s.ledger().Taken, "the batch with the Idiom is one of the day's batches")
	s.gradeAll()
	assert.NotContains(t, s.page(), "Take these words", "the cap is reached")
}

func TestStudyTodayAPendingIdiomDoesNotHoldTheNoteUpWhileTheSwitchIsOff(t *testing.T) {
	s, on := idiomTodayHarness(t)
	s.provider.answerIdioms("mit dem haus", idiomFailure)
	s.startStudy()
	testutil.Eventually(t, 60*time.Second, "the words to be ready and the Idiom to fail once", func() (bool, string) {
		return s.counts().Ready == 2 && len(s.provider.idiomRequests()) > 0, "words or the Idiom still preparing"
	})
	on.Store(false)

	page := s.page()
	assert.NotContains(t, page, `id="study-short-batch"`, "an Idiom that will not be prepared is not waited for")
	assert.NotContains(t, page, `id="study-preparing"`)
}
