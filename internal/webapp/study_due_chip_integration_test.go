//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dueChipPattern = regexp.MustCompile(`aria-label="Study, (\d+) reviews? due"`)

// dueChip is the number the Study tab's due chip shows on a page; ok is false
// when the chip is absent.
func dueChip(t *testing.T, page string) (count int, ok bool) {
	t.Helper()
	match := dueChipPattern.FindStringSubmatch(page)
	if match == nil {
		return 0, false
	}
	count, err := strconv.Atoi(match[1])
	require.NoError(t, err)
	return count, true
}

// seedWord puts one word in study in a language with the given scheduling
// state, due time and last answer (nil for none).
func (s *studyHarness) seedWord(language, lemma, state string, due time.Time, lastReview *time.Time) string {
	s.t.Helper()
	var id string
	err := s.store.Pool().QueryRow(s.ctx, `
WITH m AS (
  INSERT INTO study_material (owner_id, language, canonical_lemma, upos, state, omission_reason)
  VALUES ($1, $2, $3, 'NOUN', 'unpreparable', 'sentence-quality') RETURNING id)
INSERT INTO study_progress (owner_id, language, canonical_lemma, upos, material_id, introduced_at, learned_meaning, state, due, stability, difficulty, scheduled_days, reps, last_review)
SELECT $1, $2, $3, 'NOUN', id, $4, 'seeded', $5, $6, 5, 5, 1, 1, $7 FROM m RETURNING id::text`,
		s.owner.ID, language, lemma, due.Add(-72*time.Hour), state, due, lastReview).Scan(&id)
	require.NoError(s.t, err)
	return id
}

// seedIdiom puts an Idiom review in study.
func (s *studyHarness) seedIdiom(lemma string, due time.Time) {
	s.t.Helper()
	s.exec(`
WITH m AS (
  INSERT INTO study_material (owner_id, language, canonical_lemma, upos, item_kind, state, omission_reason)
  VALUES ($1, 'de', $2, '', 'idiom', 'unpreparable', 'sentence-quality') RETURNING id)
INSERT INTO study_progress (owner_id, language, canonical_lemma, upos, item_kind, material_id, introduced_at, learned_meaning, state, due, stability, difficulty, scheduled_days, reps)
SELECT $1, 'de', $2, '', 'idiom', id, $3, 'seeded', 'review', $4, 5, 5, 1, 1 FROM m`,
		s.owner.ID, lemma, due.Add(-72*time.Hour), due)
}

func (s *studyHarness) exec(query string, args ...any) {
	s.t.Helper()
	_, err := s.store.Pool().Exec(s.ctx, query, args...)
	require.NoError(s.t, err)
}

// assertChipMatchesToday is the parity proof: the chip is the count Today uses.
func (s *studyHarness) assertChipMatchesToday(want int, why string) {
	s.t.Helper()
	remaining := s.today().Remaining
	require.Equal(s.t, want, remaining, why+": Today's count")
	count, ok := dueChip(s.t, getAs(s.t, s.h, "/de/library", s.cookies).Body.String())
	if want == 0 {
		assert.False(s.t, ok, why+": no chip at zero")
		return
	}
	require.True(s.t, ok, why+": chip present")
	assert.Equal(s.t, want, count, why+": chip equals Today's count")
}

func TestStudyDueChipAppliesTodaysRuleAtEveryBoundary(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue() // 26 November 09:00 Berlin; the Study day ends 27 November 05:00.
	words := s.progress()
	require.Len(t, words, 2)
	s.assertChipMatchesToday(2, "two reviews due")

	nextDay := berlinTime(t, 27, 5, 0)
	s.seedWord("de", "edge-in", "review", nextDay.Add(-time.Second), nil)
	s.assertChipMatchesToday(3, "due 04:59:59 is on this Study day")
	s.seedWord("de", "edge-out", "review", nextDay, nil)
	s.assertChipMatchesToday(3, "due 05:00:00 is tomorrow's")

	answered := berlinTime(t, 26, 7, 0)
	s.seedWord("de", "answered-today", "review", berlinTime(t, 26, 8, 0), &answered)
	s.assertChipMatchesToday(3, "a review answered this Study day is not due again")
	earlier := berlinTime(t, 25, 22, 0)
	s.seedWord("de", "answered-before", "review", berlinTime(t, 26, 8, 0), &earlier)
	s.assertChipMatchesToday(4, "a review answered before today is due")

	s.seedWord("de", "again-late", "relearning", berlinTime(t, 30, 12, 0), &earlier)
	s.assertChipMatchesToday(5, "a relearning word counts whatever its due time")
	s.seedWord("de", "fresh", "new", berlinTime(t, 26, 8, 0), nil)
	s.seedWord("de", "stepping", "learning", berlinTime(t, 26, 8, 0), &earlier)
	s.assertChipMatchesToday(5, "new and learning words are not counted")

	withdrawn := s.seedWord("de", "withdrawn", "review", berlinTime(t, 26, 8, 0), nil)
	s.exec(`UPDATE study_progress SET withdrawn_at = now() WHERE id = $1`, withdrawn)
	s.assertChipMatchesToday(5, "a withdrawn word is not counted")

	reintroducing := s.seedWord("de", "reintroducing", "review", berlinTime(t, 26, 8, 0), nil)
	s.exec(`UPDATE study_material SET superseded_at = now() WHERE id = (SELECT material_id FROM study_progress WHERE id = $1)`, reintroducing)
	s.assertChipMatchesToday(5, "a word awaiting new material is not counted")

	s.seedIdiom("auf einmal", berlinTime(t, 26, 8, 0))
	s.assertChipMatchesToday(6, "an Idiom review counts")

	// A held word and a word whose context was reported today leave the count.
	first, second := words[0], words[1]
	require.Equal(t, http.StatusSeeOther, s.report(first, "wrong_word", "held").Code)
	s.assertChipMatchesToday(5, "a held word is not counted")
	require.Equal(t, http.StatusSeeOther, s.report(s.progressByID(second.ID), "unsuitable_context", "").Code)
	s.assertChipMatchesToday(4, "a context reported today is not counted")
}

func TestStudyDueChipSingularAndAbsentAtZero(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.clock.Set(berlinTime(t, 26, 9, 0))
	s.seedWord("de", "only", "review", berlinTime(t, 26, 8, 0), nil)
	page := getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, `aria-label="Study, 1 review due"`)

	s.exec(`UPDATE study_progress SET last_review = $1`, berlinTime(t, 26, 7, 0))
	_, ok := dueChip(t, getAs(t, s.h, "/de/study", s.cookies).Body.String())
	assert.False(t, ok, "the chip is absent once nothing is due")
}

func TestStudyDueChipFollowsTheActiveStudyLanguage(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.clock.Set(berlinTime(t, 26, 9, 0))
	for i := range 3 {
		s.seedWord("de", fmt.Sprintf("de-%d", i), "review", berlinTime(t, 26, 8, 0), nil)
	}
	s.seedWord("it", "casa", "review", berlinTime(t, 26, 8, 0), nil)

	german, ok := dueChip(t, getAs(t, s.h, "/de/library", s.cookies).Body.String())
	require.True(t, ok)
	assert.Equal(t, 3, german)
	italian, ok := dueChip(t, getAs(t, s.h, "/it/library", s.cookies).Body.String())
	require.True(t, ok)
	assert.Equal(t, 1, italian, "the count is the URL's language, not the stored one")
	german, ok = dueChip(t, getAs(t, s.h, "/de/library", s.cookies).Body.String())
	require.True(t, ok)
	assert.Equal(t, 3, german)
}

func TestStudyDueCountReportsADatabaseFailure(t *testing.T) {
	s := newStudyHarness(t)
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	_, err := s.store.CountStudyReviewsDue(ctx, s.owner.ID, "de", s.clock.Now())
	require.Error(t, err)
}

func TestStudyDueChipFollowsTodayAfterAContextReportedYesterday(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.progress()[0]
	require.Equal(t, http.StatusSeeOther, s.report(card, "unsuitable_context", "").Code)
	s.assertChipMatchesToday(1, "a context reported today is not counted")
	// The report no longer replaces the context on the next Study day; whatever
	// Today then counts, the chip counts.
	s.clock.Set(berlinTime(t, 27, 9, 0))
	s.assertChipMatchesToday(s.today().Remaining, "the next Study day")
}
