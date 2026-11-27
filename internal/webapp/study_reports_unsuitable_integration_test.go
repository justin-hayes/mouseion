//go:build integration

package webapp

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsuitableReports are the learner's open Unsuitable-context reports.
func (s *studyHarness) unsuitableReports() []domain.StudyReport {
	s.t.Helper()
	reports, err := s.store.ListOpenStudyReports(s.ctx, s.owner.ID, "de", domain.StudyReportUnsuitableContext)
	require.NoError(s.t, err)
	return reports
}

// retiredReportID is the ID of the word's Unsuitable-context report.
func (s *studyHarness) retiredReportID(progressID string) string {
	s.t.Helper()
	var id string
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT id::text FROM study_report WHERE owner_id=$1 AND progress_id=$2 AND kind='unsuitable_context'`, s.owner.ID, progressID).Scan(&id))
	return id
}

// waitForNewMaterial opens Study until the word is moved onto material other
// than the one it was reported in.
func (s *studyHarness) waitForNewMaterial(lemma, oldMaterialID string) domain.StudyProgress {
	s.t.Helper()
	testutil.Eventually(s.t, 60*time.Second, "the word to be re-introduced", func() (bool, string) {
		s.page()
		return s.word(lemma).MaterialID != oldMaterialID, "the word is still waiting for new material"
	})
	return s.word(lemma)
}

func TestStudyUnsuitableContextReplacesTheSentenceAndReintroducesTheWordNextDay(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	lemma := card.CanonicalLemma
	original := s.material(card.MaterialID)
	before := s.reviewLog(card.ID)

	response := s.report(card, "unsuitable_context", "part of an Idiom")
	require.Equal(t, http.StatusSeeOther, response.Code)
	location := response.Header().Get("Location")
	require.True(t, strings.HasPrefix(location, "/de/study?reported="), location)

	reports := s.unsuitableReports()
	require.Len(t, reports, 1)
	assert.Equal(t, "part of an Idiom", reports[0].Note)
	assert.Equal(t, original.Source(), reports[0].Source)
	assert.Equal(t, original.Gloss, reports[0].Gloss, "the rejected Gloss is kept as evidence")
	assert.Equal(t, original.SentenceTranslation, reports[0].SentenceTranslation)
	reported := s.progressByID(card.ID)
	assert.False(t, reported.Held, "an Unsuitable-context report does not hold the word")
	assert.Equal(t, card.Memory, reported.Memory, "the review is ungraded and the schedule does not move")
	assert.Len(t, s.reviewLog(card.ID), len(before), "no review-log row is written")
	assert.True(t, reported.Reintroducing, "the material is replaced")
	assert.Empty(t, s.openReports(), "it is not a Wrong-word report")

	// The reported card is shown once more with its answer side and a Next
	// control.
	shown := getAs(t, s.h, location, s.cookies).Body.String()
	assert.Contains(t, shown, `id="study-reported"`)
	assert.Contains(t, shown, original.Gloss)
	assert.Contains(t, shown, original.SentenceTranslation)
	assert.Contains(t, shown, `id="study-reported-next"`)
	assert.NotContains(t, shown, `name="kind"`, "the reported card offers no second report")
	other := getAs(t, s.h, "/de/study?reported=00000000-0000-4000-8000-000000000000", s.cookies).Body.String()
	assert.NotContains(t, other, `id="study-reported"`, "an unknown report shows nothing")

	// Next: the other word is the next card, and Today can finish without the
	// reported word.
	assert.Equal(t, 1, s.today().Remaining)
	next := s.currentCard()
	require.NotEqual(t, card.ID, next.ID)
	s.grade(next, "got-it")
	assert.True(t, s.today().Finished)

	// The study job prepares the replacement from another sentence. The word
	// is moved onto it but is not due again on the Study day it was reported.
	replaced := s.waitForNewMaterial(lemma, card.MaterialID)
	replacement := s.material(replaced.MaterialID)
	assert.NotEqual(t, original.SentenceOrdinal, replacement.SentenceOrdinal, "the new material is another sentence")
	assert.Equal(t, replacement.Gloss, replaced.LearnedMeaning, "the Learned meaning is re-anchored")
	assert.Equal(t, card.Memory, replaced.Memory, "the progress is kept")
	assert.True(t, s.today().Finished, "the word is not due again today")
	assert.Equal(t, original.Sentence, s.material(card.MaterialID).Sentence, "the rejected material stays stored")

	s.clock.Advance(24 * time.Hour)
	assert.Equal(t, 1, s.today().Remaining, "due at the start of the next Study day")
	assert.Equal(t, card.ID, s.currentCard().ID)
	page := s.page()
	assert.Contains(t, page, replacement.TargetWord)
	assert.NotContains(t, page, original.Sentence, "the retired sentence is never shown again")
	assert.Zero(t, s.counts().Preparing)
	retired, err := s.store.ListRetiredStudyOccurrences(s.ctx, s.owner.ID, card.ID)
	require.NoError(t, err)
	assert.Len(t, retired, 1, "the retirement stands, though the report ended once the learner moved on")
	assert.Empty(t, s.unsuitableReports(), "and the Undo window is over")
	ended, err := s.store.GetStudyReport(s.ctx, s.owner.ID, s.retiredReportID(card.ID))
	require.NoError(t, err)
	assert.Equal(t, domain.StudyReportEndRetired, ended.EndReason)
}

func TestStudyReportingTheSameCardTwiceYieldsOneUnsuitableContextReport(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	require.Equal(t, http.StatusSeeOther, s.report(card, "unsuitable_context", "").Code)
	require.Equal(t, http.StatusSeeOther, s.report(card, "unsuitable_context", "again").Code)
	assert.Len(t, s.unsuitableReports(), 1)
}

func TestStudyWordWithNoContextLeftIsWithdrawnWithItsOwnReasonAndReturnsWithANewBook(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.clock.Set(berlinTime(t, 25, 22, 0))
	s.introduceAllGotIt()
	s.clock.Set(berlinTime(t, 26, 9, 0))
	buch := s.word("buch")
	first := s.material(buch.MaterialID)

	// Buch has two sentences. The first is reported, the replacement is the
	// other one, and reporting that on the next day leaves none.
	require.Equal(t, http.StatusSeeOther, s.report(buch, "unsuitable_context", "").Code)
	second := s.waitForNewMaterial("buch", buch.MaterialID)
	require.NotEqual(t, first.SentenceOrdinal, s.material(second.MaterialID).SentenceOrdinal)
	s.clock.Set(berlinTime(t, 27, 9, 0))
	s.page()
	card := s.word("buch")
	require.Equal(t, second.MaterialID, card.MaterialID)
	require.Equal(t, http.StatusSeeOther, s.report(card, "unsuitable_context", "").Code)

	testutil.Eventually(t, 60*time.Second, "the word to be withdrawn", func() (bool, string) {
		s.page()
		return s.word("buch").Withdrawn, "the word is not withdrawn yet"
	})
	withdrawn := s.word("buch")
	assert.Equal(t, domain.StudyWithdrawnNoSuitableContext, withdrawn.WithdrawnReason)
	assert.Equal(t, buch.Memory, withdrawn.Memory, "its progress is kept")
	assert.Len(t, s.reviewLog(buch.ID), 1)
	page := s.page()
	assert.Contains(t, page, "No suitable context left")
	assert.Contains(t, page, `id="study-no-suitable-context"`)
	assert.NotContains(t, page, "Withdrawn by a correction")
	assert.NotContains(t, page, "Waiting for a usable sentence")

	// The same evidence is not tried again, and no provider is asked.
	testutil.Eventually(t, 30*time.Second, "preparation to settle", s.settled)
	calls := s.provider.Calls()
	for range 3 {
		s.page()
	}
	assert.True(t, s.word("buch").Withdrawn)
	assert.Equal(t, calls, s.provider.Calls(), "a page request calls no provider")

	// A new Book gives the word a context: it returns, in the new sentence.
	s.seedBook("library", testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 1})
	testutil.Eventually(t, 60*time.Second, "the word to return", func() (bool, string) {
		s.page()
		returned := s.word("buch")
		return !returned.Withdrawn && returned.MaterialID != card.MaterialID, "the word has not returned"
	})
	returned := s.word("buch")
	assert.Empty(t, returned.WithdrawnReason)
	assert.Equal(t, buch.Memory, returned.Memory)
	assert.Equal(t, "Study library", s.bookTitleOf(s.material(returned.MaterialID)), "it is introduced in the new Book's sentence, not a retired one")
}

// bookTitleOf is the title of the Book a material's sentence comes from.
func (s *studyHarness) bookTitleOf(material domain.StudyMaterial) string {
	s.t.Helper()
	book, err := s.store.GetBook(s.ctx, s.owner.ID, material.BookID)
	require.NoError(s.t, err)
	return book.Title
}

func TestStudyRetiredOccurrenceSurvivesAnAnalysisUpdate(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.clock.Set(berlinTime(t, 25, 22, 0))
	s.introduceAllGotIt()
	s.clock.Set(berlinTime(t, 26, 9, 0))
	haus := s.word("haus")
	original := s.material(haus.MaterialID)
	require.Equal(t, http.StatusSeeOther, s.report(haus, "unsuitable_context", "").Code)

	// Fluency keeps a context on the retired occurrence.
	fluency := domain.FluencyContextRecord{
		ProgressID: haus.ID, Agreement: domain.FluencyAgrees, Reason: domain.FluencyReasonSameMeaning,
		BookID: original.BookID, AnalysisRunID: original.AnalysisRunID, CorpusID: original.CorpusID,
		SentenceOrdinal: original.SentenceOrdinal, TokenOrdinal: original.TokenOrdinal,
		Sentence: original.Sentence, TargetWord: original.TargetWord, Gloss: original.Gloss, SentenceTranslation: original.SentenceTranslation,
	}
	require.NoError(t, s.store.RecordStudyFluencyContext(s.ctx, s.owner.ID, fluency))
	assert.Empty(t, s.fluencyContexts(), "an existing Fluency context on the sentence is no longer shown")

	// An Analysis update re-publishes the Book: the same token sits in a new
	// run at other ordinals.
	newRun := s.republish(bookID, "")
	newCorpus := s.corpusOfRun(newRun)
	retired, err := s.store.ListRetiredStudyOccurrencesOfWord(s.ctx, s.owner.ID, haus.StudyItem)
	require.NoError(t, err)
	moved := original.Source()
	moved.AnalysisRunID, moved.CorpusID, moved.SentenceOrdinal = newRun, newCorpus, original.SentenceOrdinal+10
	assert.Contains(t, retired, original.Source(), "the sentence in the run it was reported in")
	assert.Contains(t, retired, moved, "and the same token carried to the new run (same span, surface and source lemma)")
	otherToken := moved
	otherToken.TokenOrdinal++
	assert.True(t, domain.RetiredStudyOccurrence(retired, otherToken), "every token of the word in the retired sentence is retired")
	otherSentence := moved
	otherSentence.SentenceOrdinal++
	assert.False(t, domain.RetiredStudyOccurrence(retired, otherSentence), "another sentence is not")

	// A context stored in the new run, on another token of the same sentence, is
	// hidden too, and does not count towards the limit.
	fluency.AnalysisRunID, fluency.CorpusID, fluency.SentenceOrdinal, fluency.TokenOrdinal = newRun, newCorpus, moved.SentenceOrdinal, original.TokenOrdinal+1
	require.NoError(t, s.store.RecordStudyFluencyContext(s.ctx, s.owner.ID, fluency))
	assert.Empty(t, s.fluencyContexts(), "a Fluency context in the relocated sentence is hidden")
}

// corpusOfRun is the corpus an Analysis run read.
func (s *studyHarness) corpusOfRun(runID string) string {
	s.t.Helper()
	var corpusID string
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT corpus_id::text FROM analysis_runs WHERE owner_id=$1 AND id=$2`, s.owner.ID, runID).Scan(&corpusID))
	return corpusID
}

func TestStudyRepeatedUnsuitableReportReturnsToTheReportsUndoScreen(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	s.twoReviewsDue()
	card := s.currentCard()
	first := s.report(card, "unsuitable_context", "")
	require.Equal(t, http.StatusSeeOther, first.Code)
	report := s.unsuitableReports()[0]
	assert.Equal(t, "/de/study?reported="+report.ID, first.Header().Get("Location"))

	again := s.report(card, "unsuitable_context", "")
	require.Equal(t, http.StatusSeeOther, again.Code)
	assert.Equal(t, first.Header().Get("Location"), again.Header().Get("Location"), "a double tap returns to the same Undo screen")

	// The word has moved on to new material since; the repeat still finds the report.
	s.waitForNewMaterial(card.CanonicalLemma, card.MaterialID)
	late := s.report(card, "unsuitable_context", "")
	assert.Equal(t, first.Header().Get("Location"), late.Header().Get("Location"))
	assert.Len(t, s.unsuitableReports(), 1)
}
