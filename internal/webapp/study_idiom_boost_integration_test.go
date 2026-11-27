//go:build integration

package webapp

import (
	"fmt"
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

// An Idiom answered Already known enters study Mature, and a later lapse ends
// its maturity, as for a word (ADR 0136 §1, ADR 0137).
func TestStudyAlreadyKnownOnAnIdiomEntersItMatureUntilALapse(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.preparedIdiom()
	s.take()
	idiom, ok := s.progressOfIdiom()
	require.True(t, ok)

	assert.Equal(t, http.StatusSeeOther, s.post("/study/already-known", url.Values{
		"language": {"de"}, "progress": {idiom.ID}, "reps": {strconv.FormatInt(idiom.Memory.Reps, 10)},
	}).Code)

	known := s.progressByID(idiom.ID)
	assert.True(t, domain.IsStudyMature(known), "Mature at once")
	assert.Empty(t, s.reviewLog(idiom.ID), "never a grade")
	events, err := s.store.ListStudyAlreadyKnownEvents(s.ctx, s.owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, idiom.StudyItem, events[0].StudyItem)

	// Its first review is a lapse: Again ends the maturity.
	s.clock.Advance(time.Duration(known.Memory.ScheduledDays+1) * 24 * time.Hour)
	card := s.currentCard()
	require.Equal(t, idiom.ID, card.ID)
	s.grade(card, "again")
	assert.False(t, domain.IsStudyMature(s.progressByID(idiom.ID)))
}

// While the Current reading is below Target coverage its Idioms come first
// with its words, ahead of the library's more frequent words.
func TestStudyBoostsTheCurrentReadingsIdiomWithItsWords(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	library := make([]testutil.CorpusWord, 0, 20)
	for i := range 20 {
		library = append(library, testutil.CorpusWord{Lemma: fmt.Sprintf("lib%02d", i), UPOS: "NOUN", Occurrences: 50})
	}
	s.seedBook("library", library...)
	var reading string
	require.NoError(t, s.store.Pool().QueryRow(s.ctx, `SELECT id::text FROM books WHERE owner_id=$1 AND title='Study reading'`, s.owner.ID).Scan(&reading))
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, s.owner.ID, reading)
	require.NoError(t, err)

	s.preparedIdiom()
	s.take()

	idiom, ok := s.progressOfIdiom()
	require.True(t, ok, "the Current reading's Idiom is in the first batch")
	assert.Equal(t, "mit dem haus", idiom.CanonicalLemma)
	lemmas := s.introducedLemmas()
	assert.Contains(t, lemmas, "haus")
	assert.Contains(t, lemmas, "buch")
}
