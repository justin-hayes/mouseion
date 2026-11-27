//go:build integration

package webapp

import (
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// idiomDueNextDay introduces the words and the Idiom, answers them Got it and
// moves to the next Study day, when every review is due.
func (s *studyHarness) idiomsDueNextDay() {
	s.t.Helper()
	s.preparedIdiom()
	s.clock.Set(berlinTime(s.t, 25, 22, 0))
	s.introduceAllGotIt()
	s.clock.Set(berlinTime(s.t, 26, 9, 0))
	require.Equal(s.t, 3, s.today().Remaining)
}

// idiomItem is the Idiom's progress.
func (s *studyHarness) idiomItem() domain.StudyProgress {
	s.t.Helper()
	item, ok := s.progressOfIdiom()
	require.True(s.t, ok, "the Idiom is in study")
	return item
}

// waitForNewIdiomMaterial opens Study until the Idiom is moved onto material
// other than the one it was reported in.
func (s *studyHarness) waitForNewIdiomMaterial(oldMaterialID string) domain.StudyProgress {
	s.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		s.page()
		if item := s.idiomItem(); item.MaterialID != oldMaterialID {
			return item
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Fail(s.t, "the Idiom was not re-introduced")
	return domain.StudyProgress{}
}

func TestStudyUnsuitableContextRetiresAnIdiomsSentenceAndReintroducesItNextDay(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	card := s.showIdiomCard()
	original := s.material(card.MaterialID)
	before := s.reviewLog(card.ID)

	response := s.report(card, "unsuitable_context", "not the idiom here")
	require.Equal(t, http.StatusSeeOther, response.Code)
	reports := s.unsuitableReports()
	require.Len(t, reports, 1)
	assert.True(t, reports[0].IsIdiom())
	assert.Equal(t, "not the idiom here", reports[0].Note, "the note is kept as evidence")
	reported := s.progressByID(card.ID)
	assert.False(t, reported.Held, "the report does not hold the Idiom")
	assert.Equal(t, card.Memory, reported.Memory)
	assert.Len(t, s.reviewLog(card.ID), len(before), "an ungraded report writes no review")
	assert.True(t, reported.Reintroducing)
	assert.Empty(t, s.openReports(), "it is not a Wrong-word report")

	replaced := s.waitForNewIdiomMaterial(card.MaterialID)
	replacement := s.material(replaced.MaterialID)
	assert.NotEqual(t, original.SentenceOrdinal, replacement.SentenceOrdinal, "another sentence")
	assert.Equal(t, card.Memory, replaced.Memory, "the progress is kept")
	assert.Equal(t, replacement.Gloss, replaced.LearnedMeaning)
	assert.Zero(t, s.today().Remaining, "the Idiom is not due again on the Study day it was reported")

	s.clock.Advance(24 * time.Hour)
	assert.Equal(t, 1, s.today().Remaining, "due at the start of the next Study day")
	assert.NotContains(t, s.page(), original.Sentence, "the retired sentence is never shown again")
	retired, err := s.store.ListRetiredStudyOccurrencesOfWord(s.ctx, s.owner.ID, card.StudyItem)
	require.NoError(t, err)
	assert.NotEmpty(t, retired, "the retirement is the Idiom's")
}

func TestStudyUndoRestoresAReportedIdiom(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	card := s.showIdiomCard()
	original := s.material(card.MaterialID)

	location := s.report(card, "unsuitable_context", "").Header().Get("Location")
	report := s.unsuitableReports()[0]
	shown := getAs(t, s.h, location, s.cookies).Body.String()
	assert.Contains(t, shown, `id="study-reported-undo"`, "the reported Idiom card offers Undo")
	assert.Contains(t, shown, "<strong>haus</strong>", "the matched words are marked")
	s.waitForNewIdiomMaterial(card.MaterialID)

	require.Equal(t, http.StatusSeeOther, s.undo(report.ID).Code)
	restored := s.progressByID(card.ID)
	assert.Equal(t, original.ID, restored.MaterialID)
	assert.False(t, restored.Reintroducing)
	assert.Equal(t, card.Memory, restored.Memory)
	assert.Empty(t, s.unsuitableReports())
	retired, err := s.store.ListRetiredStudyOccurrencesOfWord(s.ctx, s.owner.ID, card.StudyItem)
	require.NoError(t, err)
	assert.Empty(t, retired, "Undo lifts the retirement")
}

func TestStudyReportOnAMemberWordLeavesTheIdiomAlone(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	idiom := s.idiomItem()
	haus := s.word("haus")
	require.NotEqual(t, idiom.ID, haus.ID)

	require.Equal(t, http.StatusSeeOther, s.report(haus, "unsuitable_context", "").Code)
	s.waitForNewMaterial("haus", haus.MaterialID)

	after := s.idiomItem()
	assert.Equal(t, idiom.MaterialID, after.MaterialID, "the Idiom's material is untouched")
	assert.Equal(t, idiom.Memory, after.Memory)
	assert.False(t, after.Reintroducing)
	retired, err := s.store.ListRetiredStudyOccurrencesOfWord(s.ctx, s.owner.ID, idiom.StudyItem)
	require.NoError(t, err)
	assert.Empty(t, retired, "the sentence is retired for the word only")
}

func TestStudyReportOnAnIdiomLeavesMemberWordsAlone(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	card := s.showIdiomCard()
	haus := s.word("haus")

	require.Equal(t, http.StatusSeeOther, s.report(card, "unsuitable_context", "").Code)
	s.waitForNewIdiomMaterial(card.MaterialID)

	after := s.progressByID(haus.ID)
	assert.Equal(t, haus.MaterialID, after.MaterialID, "the word's material is untouched")
	assert.Equal(t, haus.Memory, after.Memory)
	retired, err := s.store.ListRetiredStudyOccurrencesOfWord(s.ctx, s.owner.ID, haus.StudyItem)
	require.NoError(t, err)
	assert.Empty(t, retired, "the sentence is retired for the Idiom only")
}

// idiomBookID is the Book of the Idiom's introducing sentence.
func (s *studyHarness) idiomBook(material domain.StudyMaterial) string { return material.BookID }

func TestStudyExclusionThatBreaksAnIdiomsMatchReintroducesItWithItsProgressKept(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	idiom := s.showIdiomCard() // the words are graded, so the Idiom is the next review
	original := s.material(idiom.MaterialID)
	log := len(s.reviewLog(idiom.ID))

	// The learner excludes "haus" in the introducing sentence, so the sentence
	// no longer holds "mit dem haus".
	s.decide(s.idiomBook(original), "haus", original.SentenceOrdinal, "exclude", "")

	s.page()
	waiting := s.progressByID(idiom.ID)
	assert.True(t, waiting.Reintroducing, "the Idiom's material was replaced")
	assert.Equal(t, idiom.Memory, waiting.Memory, "its progress is kept")
	replaced := s.waitForNewIdiomMaterial(idiom.MaterialID)
	replacement := s.material(replaced.MaterialID)
	assert.NotEqual(t, original.SentenceOrdinal, replacement.SentenceOrdinal, "another sentence")
	assert.True(t, replaced.Corrected())
	assert.Equal(t, idiom.Memory, replaced.Memory)
	assert.Len(t, s.reviewLog(idiom.ID), log, "a correction never creates a review")
	assert.False(t, replaced.Withdrawn)
}

func TestStudyAnIdiomWhoseIntroducingMatchStandsIsLeftAlone(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	idiom := s.showIdiomCard() // the words are graded, so the Idiom is the next review
	original := s.material(idiom.MaterialID)

	// A decision on a sentence that is not the introducing one changes nothing.
	other := int64(0)
	for other == original.SentenceOrdinal {
		other++
	}
	s.decide(s.idiomBook(original), "haus", other, "exclude", "")
	s.page()

	after := s.progressByID(idiom.ID)
	assert.Equal(t, idiom.MaterialID, after.MaterialID)
	assert.False(t, after.Reintroducing)
	assert.False(t, after.Withdrawn)
}

func TestStudyRemovingTheOnlyBookWithCandidatesWithdrawsAnIdiomAndAddingOneBringsItBack(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	idiom := s.showIdiomCard() // the words are graded, so the Idiom is the next review
	original := s.material(idiom.MaterialID)
	log := len(s.reviewLog(idiom.ID))

	// Another Book, with no candidate, stays in My Books.
	lookup := newFakeIdiomLookup(germanIdiom("mit dem haus"))
	s.buildIdiomCounts(s.seedBook("other", testutil.CorpusWord{Lemma: "baum", UPOS: "NOUN", Occurrences: 1}), lookup)
	require.NoError(t, s.store.RemoveBookFromMyBooks(s.ctx, s.owner.ID, original.BookID))
	s.page()
	withdrawn := s.progressByID(idiom.ID)
	assert.True(t, withdrawn.Withdrawn, "no candidate is left in the Corpus")
	assert.Equal(t, idiom.Memory, withdrawn.Memory, "its progress is kept")
	assert.Len(t, s.reviewLog(idiom.ID), log)
	assert.NotContains(t, s.page(), original.Sentence)

	// A Book with a candidate brings it back, introduced in the new Book's sentence.
	library := s.seedBook("library", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 1})
	s.buildIdiomCounts(library, lookup)
	returned := s.waitForIdiomToReturn(idiom.MaterialID)
	assert.False(t, returned.Withdrawn)
	assert.Equal(t, idiom.Memory, returned.Memory)
	assert.Equal(t, library.ID, s.material(returned.MaterialID).BookID)
}

func TestStudyAnIdiomWithABookStillBeingCountedIsNotWithdrawn(t *testing.T) {
	s, _ := idiomTodayHarness(t)
	s.idiomsDueNextDay()
	idiom := s.showIdiomCard() // the words are graded, so the Idiom is the next review

	// A Book whose Idiom counts are not ready must not count as zero.
	s.seedBook("pending", testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 1})
	require.NoError(t, s.store.RemoveBookFromMyBooks(s.ctx, s.owner.ID, s.material(idiom.MaterialID).BookID))
	s.page()

	assert.False(t, s.progressByID(idiom.ID).Withdrawn, "the pending Book may hold candidates")
}

func TestStudyADictionaryRefreshThatDropsAnIdiomsHeadwordWithdrawsItWithItsProgress(t *testing.T) {
	lookup := newFakeIdiomLookup(germanIdiom("mit dem haus"))
	avoidance := germanIdiomAvoidance(t, lookup, true)
	s := newStudyHarnessWith(t, studyHarnessOptions{capabilities: readyGerman(), idioms: avoidance})
	book := s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.buildIdiomCounts(book, lookup)
	s.idiomsDueNextDay()
	idiom := s.showIdiomCard() // the words are graded, so the Idiom is the next review

	// The refresh drops the headword: the index version changes and the counts
	// are rebuilt against it.
	refreshed := newFakeIdiomLookup()
	avoidance.Index, avoidance.IndexVersion = refreshed, "idiom-test-index-2"
	s.page()
	assert.False(t, s.progressByID(idiom.ID).Withdrawn, "until the counts are rebuilt the Book is pending, not empty")
	s.buildIdiomCountsAt(book, refreshed, "idiom-test-index-2")

	s.page()
	withdrawn := s.progressByID(idiom.ID)
	assert.True(t, withdrawn.Withdrawn)
	assert.Equal(t, idiom.Memory, withdrawn.Memory, "its progress is kept")
	assert.Len(t, s.reviewLog(idiom.ID), 1)
}

// waitForIdiomToReturn opens Study until the withdrawn Idiom is back on new
// material.
func (s *studyHarness) waitForIdiomToReturn(oldMaterialID string) domain.StudyProgress {
	s.t.Helper()
	testutil.Eventually(s.t, 60*time.Second, "the Idiom to return", func() (bool, string) {
		s.page()
		item := s.idiomItem()
		return !item.Withdrawn && item.MaterialID != oldMaterialID, "the Idiom has not returned"
	})
	return s.idiomItem()
}
