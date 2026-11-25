//go:build integration

package webapp

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// preparedBook seeds an analyzed Book, opens Study so its words' material is
// enqueued and waits until every word is settled. It returns the Book's ID.
func (s *studyHarness) preparedBook(words ...testutil.CorpusWord) string {
	s.t.Helper()
	book := s.seedBook("reading", words...)
	s.page()
	require.NoError(s.t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(s.t, "study River client", s.client.Stop)
	testutil.Eventually(s.t, 60*time.Second, "material to settle", func() (bool, string) {
		return s.counts().Preparing == 0, "preparing words remain"
	})
	return book.ID
}

// word is the learner's progress for one lemma.
func (s *studyHarness) word(lemma string) domain.StudyProgress {
	s.t.Helper()
	for _, item := range s.progress() {
		if item.CanonicalLemma == lemma {
			return item
		}
	}
	s.t.Fatalf("%s is not in study", lemma)
	return domain.StudyProgress{}
}

// material is one Study material record.
func (s *studyHarness) material(id string) domain.StudyMaterial {
	s.t.Helper()
	item, err := s.store.GetStudyMaterial(s.ctx, s.owner.ID, id)
	require.NoError(s.t, err)
	return item
}

// decide drives the existing lemma-review flow over HTTP for the occurrence of
// the word's surface form that is in the given sentence.
func (s *studyHarness) decide(bookID, surface string, sentence int64, decision, lemma string) {
	s.t.Helper()
	occurrences, err := s.store.ListLemmaReviewOccurrences(s.ctx, s.owner.ID, bookID, surface)
	require.NoError(s.t, err)
	for _, occurrence := range occurrences {
		if occurrence.SentenceOrdinal == sentence {
			confirmLemmaDecisionForm(s.t, s.h, "/reading/books/"+bookID+"/lemma-review", s.cookies, s.cookies[0].Value, surface, decision, lemma, occurrence.ID())
			return
		}
	}
	s.t.Fatalf("no occurrence of %s in sentence %d", surface, sentence)
}

func TestStudyCorrectionReintroducesAWordInNewMaterialAndKeepsItsProgress(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	haus := s.word("haus")
	original := s.material(haus.MaterialID)
	require.EqualValues(t, 1, haus.Memory.Reps)

	// The learner confirms that the sentence Study introduced the word in
	// actually held another word.
	s.decide(bookID, "haus", original.SentenceOrdinal, "correct", "hof")
	s.clock.Advance(24 * time.Hour)
	assert.Equal(t, 2, s.today().Remaining, "both words are due at the start of the next Study day")
	// The other word comes first and its sentence is still valid.
	require.Equal(t, s.word("buch").ID, s.currentCard().ID)
	s.grade(s.word("buch"), "got-it")

	// Opening Study checks the word that is due next: its sentence no longer
	// belongs to it, so the review waits for new material and keeps its place
	// in Today.
	page := s.page()
	waiting := s.word("haus")
	assert.True(t, waiting.Reintroducing, "its material was replaced")
	assert.Equal(t, haus.MaterialID, waiting.MaterialID)
	assert.Equal(t, haus.Memory, waiting.Memory, "the FSRS state is unchanged")
	assert.Contains(t, page, "waiting for a new sentence after a correction")
	assert.NotContains(t, page, original.Sentence, "the corrected sentence is not shown")

	// The study job prepares the replacement; the next visit moves the word
	// onto it.
	testutil.Eventually(t, 60*time.Second, "the word to be re-introduced", func() (bool, string) {
		s.page()
		return s.word("haus").MaterialID != haus.MaterialID, "the word is still waiting for new material"
	})
	reintroduced := s.word("haus")
	replacement := s.material(reintroduced.MaterialID)
	assert.NotEqual(t, original.SentenceOrdinal, replacement.SentenceOrdinal, "the new sentence is another occurrence")
	assert.Equal(t, domain.StudyMaterialReady, replacement.State)
	assert.Equal(t, replacement.Gloss, reintroduced.LearnedMeaning, "the learned meaning is re-anchored to the new Gloss")
	assert.True(t, reintroduced.Corrected(), "the card is marked as corrected")
	assert.False(t, reintroduced.Reintroducing)
	assert.Equal(t, haus.Memory, reintroduced.Memory, "the FSRS state, due time and counts are unchanged")
	assert.Equal(t, haus.IntroducedAt, reintroduced.IntroducedAt)
	assert.Len(t, s.reviewLog(haus.ID), 1, "the review history is kept")

	card := s.currentCard()
	require.Equal(t, haus.ID, card.ID)
	page = getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, `id="study-corrected"`, "the card says it was corrected")
	assert.Contains(t, page, replacement.TargetWord)

	// The word the correction points to is never put into study.
	for _, item := range s.progress() {
		assert.NotEqual(t, "hof", item.CanonicalLemma)
	}
	s.grade(card, "got-it")
	assert.False(t, s.word("haus").Corrected(), "answering the card clears the mark")
	assert.Len(t, s.reviewLog(haus.ID), 2)
}

func TestStudyExclusionsWithdrawAWordWithNoValidOccurrenceAndItsProgressIsKept(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "baum", UPOS: "NOUN", Occurrences: 1}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	baum := s.word("baum")
	source := s.material(baum.MaterialID)

	s.decide(bookID, "baum", source.SentenceOrdinal, "exclude", "")
	s.clock.Advance(24 * time.Hour)
	assert.Equal(t, 2, s.today().Remaining)

	page := s.page()
	withdrawn := s.word("baum")
	assert.True(t, withdrawn.Withdrawn, "no valid occurrence remains")
	assert.Equal(t, baum.Memory, withdrawn.Memory, "its progress is kept")
	assert.Equal(t, baum.LearnedMeaning, withdrawn.LearnedMeaning)
	assert.Contains(t, page, "Withdrawn by a correction")
	assert.Contains(t, page, `id="study-withdrawn"`)
	assert.Contains(t, page, "<li>Baum</li>")
	assert.Equal(t, 1, s.today().Remaining, "a withdrawn word is not due and is not counted")
	assert.Contains(t, page, "1 review left today.")
	assert.Equal(t, s.word("buch").ID, s.currentCard().ID, "only the other word is shown")
	assert.Len(t, s.reviewLog(baum.ID), 1)

	// It stays withdrawn on later days.
	s.clock.Advance(24 * time.Hour)
	s.page()
	assert.True(t, s.word("baum").Withdrawn)
	assert.Empty(t, s.progressForUnknownIdentity("baum"), "no second progress row is created for it")
}

// progressForUnknownIdentity lists the learner's progress rows for a lemma.
func (s *studyHarness) progressForUnknownIdentity(lemma string) []domain.StudyProgress {
	var rows []domain.StudyProgress
	for _, item := range s.progress() {
		if item.CanonicalLemma == lemma {
			rows = append(rows, item)
		}
	}
	if len(rows) <= 1 {
		return nil
	}
	return rows
}
