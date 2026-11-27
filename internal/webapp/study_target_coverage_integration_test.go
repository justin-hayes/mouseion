//go:build integration

package webapp

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// introducedLemmas are the lemmas the learner has taken, sorted.
func (s *studyHarness) introducedLemmas() []string {
	s.t.Helper()
	var lemmas []string
	for _, word := range s.progress() {
		lemmas = append(lemmas, word.CanonicalLemma)
	}
	slices.Sort(lemmas)
	return lemmas
}

// The Current reading's words come first until its in-study coverage reaches
// Target coverage; after that frequency across the To Read Books decides, and
// the boost returns when coverage falls below the target again (ADR 0136 §2).
func TestNewWordBatchesBoostTheCurrentReadingUntilTargetCoverage(t *testing.T) {
	s := newStudyHarness(t)
	readingWords := []string{"baum", "blume", "brot", "fisch", "garten", "katze", "kind", "licht", "mond", "stadt", "wald", "wasser"} // by key
	// 22 library words, so more than a batch of them remains after the second.
	var libraryWords []string
	for i := range 22 {
		libraryWords = append(libraryWords, fmt.Sprintf("lib%02d", i))
	}
	// "haus" is 88 of the Current reading's 100 tokens and already in study.
	words := []testutil.CorpusWord{{Lemma: "haus", UPOS: "NOUN", Occurrences: 88}}
	for _, lemma := range readingWords {
		words = append(words, testutil.CorpusWord{Lemma: lemma, UPOS: "NOUN", Occurrences: 1})
	}
	reading := s.seedBook("reading", words...)
	library := make([]testutil.CorpusWord, 0, len(libraryWords))
	for _, lemma := range libraryWords {
		library = append(library, testutil.CorpusWord{Lemma: lemma, UPOS: "NOUN", Occurrences: 5})
	}
	s.seedBook("library", library...)
	// The Book the learner later switches to: most of its tokens are not in study.
	secondWords := make([]string, 0, 12)
	second := []testutil.CorpusWord{{Lemma: "haus", UPOS: "NOUN", Occurrences: 40}}
	for i := range 12 {
		lemma := fmt.Sprintf("zz%02d", i)
		secondWords = append(secondWords, lemma)
		second = append(second, testutil.CorpusWord{Lemma: lemma, UPOS: "NOUN", Occurrences: 1})
	}
	secondBook := s.seedBook("second", second...)
	// A Read Book is in the Corpus but adds no priority, however frequent.
	readBook := s.seedBook("finished", testutil.CorpusWord{Lemma: "mauer", UPOS: "NOUN", Occurrences: 500})
	require.NoError(t, s.store.MarkBookPreviouslyRead(s.ctx, s.owner.ID, readBook.ID))
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, s.owner.ID, reading.ID)
	require.NoError(t, err)
	s.seedReviewedWord("haus")

	require.Equal(t, http.StatusOK, getAs(t, s.h, "/de/study", s.cookies).Code)
	require.NoError(t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(t, "study River client", s.client.Stop)
	testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) { return s.settled() })
	require.GreaterOrEqual(t, s.counts().Ready, 25, "the preparation window is ready")
	s.clock.Set(berlinTime(t, 25, 9, 0))

	// At 88% the Current reading is below target: its ten most frequent words
	// (all tied, so the first ten by key) come before every library word.
	s.page()
	s.take()
	first := append([]string{"haus"}, readingWords[:10]...)
	slices.Sort(first)
	assert.Equal(t, first, s.introducedLemmas(), "below target the Current reading's words come first")
	s.gradeAll()

	// 98 of 100 tokens are now in study: exactly at the target, so the boost is
	// off and the To Read frequency decides. The Read Book's 500 (mauer) add nothing: it is left behind.
	assert.Equal(t, 98, s.currentReadingInStudyTokens(reading.ID))
	s.page()
	s.take()
	want := append(append([]string{"haus"}, readingWords[:10]...), libraryWords[:10]...)
	slices.Sort(want)
	assert.Equal(t, want, s.introducedLemmas(), "at target the To Read Books' frequency decides")
	s.gradeAll()

	// The learner switches to a Book of which most is not in study: its words
	// come first again, ahead of the library words that occur more often.
	_, err = s.store.Pool().Exec(s.ctx, `UPDATE primary_goals SET book_id=$2 WHERE owner_id=$1 AND language='de'`, s.owner.ID, secondBook.ID)
	require.NoError(t, err)
	s.clock.Set(berlinTime(t, 26, 9, 0))
	s.gradeAll() // the day-after reviews are done first
	s.page()
	testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) { return s.settled() })
	before := len(s.progress())
	s.take()
	got := s.introducedLemmas()
	assert.Len(t, got, before+10)
	for _, lemma := range secondWords[:6] {
		assert.Contains(t, got, lemma, "below target again, the new Current reading's words come first")
	}
	// Without the boost the one-occurrence words of the old reading, which sort
	// before these, would have filled the batch ahead of them.
	assert.NotContains(t, got, "wald")
	assert.NotContains(t, got, "wasser")
}

// currentReadingInStudyTokens counts the Current reading's tokens whose word is
// in study.
func (s *studyHarness) currentReadingInStudyTokens(bookID string) int {
	s.t.Helper()
	var inStudy int
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `
SELECT COALESCE(sum(c.occurrence_count), 0)::int FROM vocabulary_browse_counts c
 JOIN study_progress p ON p.owner_id = c.owner_id AND p.canonical_lemma = c.canonical_lemma AND p.upos = c.upos AND p.withdrawn_at IS NULL
WHERE c.owner_id = $1 AND c.book_id = $2`, s.owner.ID, bookID).Scan(&inStudy))
	return inStudy
}

// seedReviewedWord puts a word in study as one the learner already reviews.
func (s *studyHarness) seedReviewedWord(lemma string) {
	s.t.Helper()
	_, err := s.store.Pool().Exec(s.ctx, `
WITH m AS (
  INSERT INTO study_material (owner_id, language, canonical_lemma, upos, state, omission_reason)
  VALUES ($1, 'de', $2, 'NOUN', 'unpreparable', 'sentence-quality') RETURNING id)
INSERT INTO study_progress (owner_id, language, canonical_lemma, upos, material_id, introduced_at, learned_meaning, state, due, stability, difficulty, scheduled_days, reps, last_review)
SELECT $1, 'de', $2, 'NOUN', id, $3, 'seeded', 'review', $4, 5, 5, 1, 1, $3 FROM m`,
		s.owner.ID, lemma, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(s.t, err)
}
