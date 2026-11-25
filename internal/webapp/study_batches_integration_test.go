//go:build integration

package webapp

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// twelveWords are German nouns whose occurrence counts fall one by one, so the
// stream order is fixed: the first batch of ten leaves the last two.
var twelveWords = []string{"baum", "blume", "brot", "fisch", "garten", "kind", "katze", "licht", "mond", "stadt", "wald", "wasser"}

// preparedTwelve seeds a Book with twelve words and waits for their Study
// material, so a batch of ten and a short batch of two both exist.
func (s *studyHarness) preparedTwelve() {
	s.t.Helper()
	words := make([]testutil.CorpusWord, 0, len(twelveWords))
	for i, lemma := range twelveWords {
		words = append(words, testutil.CorpusWord{Lemma: lemma, UPOS: "NOUN", Occurrences: len(twelveWords) - i})
	}
	s.seedBook("reading", words...)
	require.Equal(s.t, http.StatusOK, getAs(s.t, s.h, "/de/study", s.cookies).Code)
	require.NoError(s.t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(s.t, "study River client", s.client.Stop)
	testutil.Eventually(s.t, 60*time.Second, "material to settle", func() (bool, string) {
		return s.counts().Ready == len(twelveWords), "words are still being prepared"
	})
}

// seedReviews puts n words in study on a day-scale schedule, each due at due.
func (s *studyHarness) seedReviews(n int, due time.Time) {
	s.t.Helper()
	_, err := s.store.Pool().Exec(s.ctx, `
WITH m AS (
  INSERT INTO study_material (owner_id, language, canonical_lemma, upos, state, omission_reason)
  SELECT $1, 'de', 'seed' || g, 'NOUN', 'unpreparable', 'sentence-quality' FROM generate_series(1, $2::int) g
  RETURNING id, canonical_lemma)
INSERT INTO study_progress (owner_id, language, canonical_lemma, upos, material_id, introduced_at, learned_meaning, state, due, stability, difficulty, scheduled_days, reps, last_review)
SELECT $1, 'de', canonical_lemma, 'NOUN', id, $3, 'seeded', 'review', $4, 5, 5, 1, 1, $3 FROM m`,
		s.owner.ID, n, due.Add(-48*time.Hour), due)
	require.NoError(s.t, err)
}

func (s *studyHarness) seedLedger(studyDay string, taken int) {
	s.t.Helper()
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO study_batch_ledger (owner_id, language, study_day, batches_offered, batches_taken) VALUES ($1, 'de', $2::date, $3, $3)`, s.owner.ID, studyDay, taken)
	require.NoError(s.t, err)
}

func (s *studyHarness) ledger() domain.StudyBatchLedger {
	s.t.Helper()
	ledger, err := s.store.StudyBatchLedger(s.ctx, s.owner.ID, "de", domain.StudyDay(s.clock.Now()))
	require.NoError(s.t, err)
	return ledger
}

func (s *studyHarness) take() {
	s.t.Helper()
	require.Equal(s.t, http.StatusSeeOther, s.post("/study/batches/take", url.Values{"language": {"de"}}).Code)
}

// gradeAll answers every card that is due now Got it.
func (s *studyHarness) gradeAll() {
	s.t.Helper()
	for {
		card, err := s.store.NextStudyCard(s.ctx, s.owner.ID, "de", s.clock.Now())
		if err != nil {
			return
		}
		s.grade(card.Progress, "got-it")
	}
}

func TestStudyOffersTwoBatchesFromTwelveWordsAndRecordsThemInTheLedger(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.clock.Set(berlinTime(t, 25, 9, 0))

	page := s.page()
	assert.Contains(t, page, "10 new words are ready to learn.")
	assert.NotContains(t, page, "study-short-batch", "a full batch needs no explanation")
	assert.Equal(t, domain.StudyBatchLedger{StudyDay: "2026-11-25", Offered: 1}, s.ledger(), "showing a batch records it as offered")
	s.page()
	assert.Equal(t, 1, s.ledger().Offered, "showing it again records nothing more")

	s.take()
	assert.Len(t, s.progress(), 10)
	s.gradeAll()
	page = s.page()
	assert.Contains(t, page, "2 new words are ready to learn.", "the next batch holds the words that remain")
	assert.NotContains(t, page, "study-short-batch", "nothing more is being prepared, so a short batch is not explained")
	assert.Equal(t, domain.StudyBatchLedger{StudyDay: "2026-11-25", Offered: 2, Taken: 1}, s.ledger())
	s.take()
	s.gradeAll()
	assert.Equal(t, domain.StudyBatchLedger{StudyDay: "2026-11-25", Offered: 2, Taken: 2}, s.ledger())
	assert.Len(t, s.progress(), 12)
}

func TestStudyOffersNoFourthBatchInAStudyDay(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.clock.Set(berlinTime(t, 25, 9, 0))

	s.seedLedger("2026-11-25", 2)
	assert.Contains(t, s.page(), "Take these words", "a third batch is still offered")

	s.take()
	assert.Len(t, s.progress(), 10)
	assert.Equal(t, 3, s.ledger().Taken)
	s.gradeAll()
	page := s.page()
	assert.NotContains(t, page, "Take these words", "three batches are taken")
	assert.Contains(t, page, "Nothing to study right now.")
	s.take()
	assert.Len(t, s.progress(), 10, "a fourth batch cannot be taken")
	assert.Equal(t, 3, s.ledger().Taken)

	s.clock.Set(berlinTime(t, 26, 9, 0))
	s.gradeAll()
	assert.Contains(t, s.page(), "2 new words are ready to learn.", "the next Study day offers again")
}

func TestStudyBacklogBrakeLimitsBatchesByTheNextDaysForecast(t *testing.T) {
	cases := []struct {
		name    string
		reviews int
		batches int
	}{
		{"99 reviews leave the full cap", 99, 3},
		{"100 reviews allow one batch", 100, 1},
		{"149 reviews allow one batch", 149, 1},
		{"150 reviews allow none", 150, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStudyHarness(t)
			s.preparedTwelve()
			s.clock.Set(berlinTime(t, 25, 9, 0))
			s.seedReviews(tc.reviews, berlinTime(t, 26, 20, 0))
			assert.Equal(t, tc.reviews, domain.StudyForecast(s.progress(), s.clock.Now()))

			if tc.batches == 0 {
				assert.NotContains(t, s.page(), "Take these words")
				s.take()
				assert.Empty(t, s.progress()[tc.reviews:], "no batch can be taken")
				assert.Zero(t, s.ledger().Offered, "nothing was offered")
				return
			}
			assert.Contains(t, s.page(), "Take these words")
			s.take()
			s.gradeAll()
			assert.Equal(t, 1, s.ledger().Taken)
			page := s.page()
			if tc.batches == 1 {
				assert.NotContains(t, page, "Take these words", "the brake leaves one batch")
				s.take()
				assert.Equal(t, 1, s.ledger().Taken)
				assert.Len(t, s.progress(), tc.reviews+10)
				return
			}
			// The ten answered words join the forecast, which passes 100 and
			// brakes the day's remaining batches.
			assert.Len(t, s.progress(), tc.reviews+10)
			assert.Equal(t, tc.reviews+10, domain.StudyForecast(s.progress(), s.clock.Now()))
		})
	}
}

func TestStudyWordsTakenButNotIntroducedLeadTheNextStudyDay(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.take()
	// Three of the ten words are introduced; seven are left unanswered.
	for range 3 {
		card := s.currentCard()
		s.grade(card, "got-it")
	}
	waiting := domain.StudyUnintroduced(s.progress())
	require.Len(t, waiting, 7)

	// The next Study day: its reviews first, then the carried-over words, and no
	// new batch until every one of them is answered.
	s.clock.Set(berlinTime(t, 26, 9, 0))
	assert.Equal(t, 3, s.today().Remaining)
	for range 3 {
		card := s.currentCard()
		assert.Positive(t, card.Memory.Reps, "reviews come first")
		s.grade(card, "got-it")
	}
	assert.True(t, s.today().Finished)
	assert.Zero(t, s.ledger().Taken, "the carried-over words belong to no batch of the new day")
	waitingIDs := map[string]bool{}
	for _, item := range waiting {
		waitingIDs[item.ID] = true
	}
	for range 7 {
		page := s.page()
		assert.NotContains(t, page, "Take these words", "the words taken earlier lead the batch")
		assert.Contains(t, page, "New word")
		card := s.currentCard()
		assert.True(t, waitingIDs[card.ID], "the card is one of the words taken the day before")
		s.grade(card, "got-it")
	}
	assert.Empty(t, domain.StudyUnintroduced(s.progress()))
	assert.Contains(t, s.page(), "2 new words are ready to learn.", "the batch is offered after them")
}

func TestStudyShortBatchSaysNewWordsAreStillBeingPrepared(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.take()
	s.gradeAll()

	// Two ready words remain, and one more is being prepared.
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO study_material (owner_id, language, canonical_lemma, upos, state) VALUES ($1, 'de', 'zeitung', 'NOUN', 'pending')`, s.owner.ID)
	require.NoError(t, err)
	page := s.page()
	assert.Contains(t, page, "2 new words are ready to learn.")
	assert.Contains(t, page, `id="study-short-batch"`)
	assert.Contains(t, page, "still being prepared")
}
