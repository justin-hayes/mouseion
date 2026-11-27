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

// Removing a Book from My Books takes it out of the Corpus, yet never costs
// the learner Study progress (ADR 0136 §2): its words keep their progress and
// Study material, reviews continue, and Concordance no longer searches it.
func TestRemovingABookKeepsStudyProgressAndMaterial(t *testing.T) {
	s := newStudyHarness(t)
	reading := s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	other := s.seedBook("other", testutil.CorpusWord{Lemma: "katze", UPOS: "NOUN", Occurrences: 2})
	require.NoError(t, s.store.MarkBookPreviouslyRead(s.ctx, s.owner.ID, reading.ID))
	require.Equal(t, http.StatusOK, getAs(t, s.h, "/de/study", s.cookies).Code)
	require.NoError(t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(t, "study River client", s.client.Stop)
	testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) { return s.settled() })

	concordanceBooks := func(lemma string) map[string]bool {
		result, err := s.store.ListVocabularyConcordance(s.ctx, s.owner.ID, "de", domain.ConcordanceLookup{Term: lemma, Match: domain.ConcordanceMatchLemma})
		require.NoError(t, err)
		found := map[string]bool{}
		for _, occurrence := range result.Occurrences {
			found[occurrence.BookID] = true
		}
		return found
	}
	assert.Equal(t, map[string]bool{reading.ID: true}, concordanceBooks("haus"), "a Read Book is in the Corpus")

	require.Equal(t, http.StatusSeeOther, s.post("/study/batches/take", url.Values{"language": {"de"}}).Code)
	introduced := s.progress()
	require.NotEmpty(t, introduced)
	var fromReading []domain.StudyProgress
	for _, word := range introduced {
		if word.CanonicalLemma == "haus" || word.CanonicalLemma == "buch" {
			fromReading = append(fromReading, word)
		}
	}
	require.NotEmpty(t, fromReading)
	materialBefore := s.counts()

	removed := s.post("/library/books/"+reading.ID+"/remove", url.Values{})
	require.Equal(t, http.StatusSeeOther, removed.Code)
	var members int
	require.NoError(t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1 AND book_id=$2 AND state='active'`, s.owner.ID, reading.ID).Scan(&members))
	require.Zero(t, members, "Remove ends membership")

	assert.Len(t, s.progress(), len(introduced), "Remove keeps Study progress")
	assert.Equal(t, materialBefore, s.counts(), "Remove keeps Study material")
	assert.Empty(t, concordanceBooks("haus"), "Concordance no longer searches a removed Book")
	assert.Equal(t, map[string]bool{other.ID: true}, concordanceBooks("katze"), "Concordance searches the Books that remain")

	// Reviews continue: the card still shows and a grade is recorded.
	page := getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, "Show answer")
	card := s.currentCard()
	graded := s.post("/study/grades", url.Values{"language": {"de"}, "progress": {card.ID}, "reps": {strconv.FormatInt(card.Memory.Reps, 10)}, "grade": {"got-it"}})
	require.Equal(t, http.StatusSeeOther, graded.Code)
	assert.Len(t, s.reviewLog(card.ID), 1, "a word of the removed Book is still reviewed")
}
