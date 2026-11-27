//go:build integration

package webapp

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reportedHaus prepares a Book of two words, answers both once, and reports the
// due word "haus" as a wrong word on the next Study day. It returns the Book,
// the word as reported and the sentence it was reported in.
func (s *studyHarness) reportedHaus(occurrences int) (bookID string, word domain.StudyProgress, sentence int64) {
	s.t.Helper()
	bookID = s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: occurrences}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	s.clock.Advance(24 * time.Hour)
	word = s.word("haus")
	require.Equal(s.t, http.StatusSeeOther, s.report(word, "wrong_word", "kam … umhin").Code)
	require.True(s.t, s.word("haus").Held)
	return bookID, word, s.material(word.MaterialID).SentenceOrdinal
}

// removeOccurrence deletes the token the material was prepared from, as if its
// Book's analysis no longer held it.
func (s *studyHarness) removeOccurrence(source domain.StudyMaterial) {
	s.t.Helper()
	_, err := s.store.Pool().Exec(s.ctx, `DELETE FROM corpus_tokens WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=$3 AND token_ordinal=$4`,
		s.owner.ID, source.AnalysisRunID, source.SentenceOrdinal, source.TokenOrdinal)
	require.NoError(s.t, err)
}

func (s *studyHarness) reportOf(id string) domain.StudyReport {
	s.t.Helper()
	report, err := s.store.GetStudyReport(s.ctx, s.owner.ID, id)
	require.NoError(s.t, err)
	return report
}

// requireReleased checks a repaired word: its report ended for the reason, it is
// no longer held, and it is due at the start of the next Study day with the rest
// of its Study progress kept.
func (s *studyHarness) requireReleased(id string, before domain.StudyProgress, reason domain.StudyReportEnd) {
	s.t.Helper()
	report := s.reportOf(id)
	assert.Equal(s.t, reason, report.EndReason)
	assert.False(s.t, report.Open())
	released := s.word("haus")
	assert.False(s.t, released.Held)
	assert.True(s.t, domain.NextStudyDayStart(s.clock.Now()).Equal(released.Memory.Due), "due %v", released.Memory.Due)
	memory, was := released.Memory, before.Memory
	memory.Due, was.Due = time.Time{}, time.Time{}
	assert.Equal(s.t, was, memory, "the FSRS state is kept")
	assert.Len(s.t, s.reviewLog(before.ID), 1)
	assert.Empty(s.t, s.openReports())
}

func TestStudyLemmaCorrectionOfTheReportedOccurrenceEndsTheReportAndReintroducesTheWord(t *testing.T) {
	s := newStudyHarness(t)
	bookID, word, sentence := s.reportedHaus(3)
	id := s.openReports()[0].ID

	// The report links to its remediation screen, which shows its occurrence.
	remediation := "/study/reports/" + id + "/remediation"
	assert.Contains(t, getAs(t, s.h, "/de/study/reports", s.cookies).Body.String(), remediation, "the list links to the remediation screen")
	assert.NotContains(t, getAs(t, s.h, "/de/study/reports", s.cookies).Body.String(), "/lemma-review", "not to the Book's Lemma review")
	opened := getAs(t, s.h, remediation, s.cookies).Body.String()
	assert.Contains(t, opened, "kam … umhin")
	assert.Equal(t, 1, strings.Count(opened, "<strong>Observed form:</strong>"), "only the reported occurrence is shown")

	// A decision about another occurrence of the form leaves the report open.
	other := sentence + 1
	if other >= 3 {
		other = sentence - 1
	}
	s.decide(bookID, "haus", other, "keep", "")
	assert.True(t, s.reportOf(id).Open())

	s.remediate(id, "correct", "hof")
	s.requireReleased(id, word, domain.StudyReportEndCorrected)
	assert.Equal(t, 1, s.today().Remaining, "only the other word is due today")

	// At the start of the next Study day it is re-introduced in new material.
	s.clock.Advance(24 * time.Hour)
	s.grade(s.word("buch"), "got-it")
	s.page()
	assert.True(t, s.word("haus").Reintroducing)
	testutil.Eventually(t, 60*time.Second, "the word to be re-introduced", func() (bool, string) {
		s.page()
		return s.word("haus").MaterialID != word.MaterialID, "the word is still waiting for new material"
	})
	reintroduced := s.word("haus")
	assert.True(t, reintroduced.Corrected())
	assert.Equal(t, s.material(reintroduced.MaterialID).Gloss, reintroduced.LearnedMeaning, "the learned meaning is re-anchored")
	assert.Equal(t, word.ID, reintroduced.ID, "the same Study progress")
	assert.Equal(t, word.ID, s.currentCard().ID)
}

func TestStudyOccurrenceExclusionEndsTheReportAndWithdrawsAWordWithNoOtherOccurrence(t *testing.T) {
	s := newStudyHarness(t)
	_, word, _ := s.reportedHaus(1)
	id := s.openReports()[0].ID

	s.remediate(id, "exclude", "")
	s.requireReleased(id, word, domain.StudyReportEndExcluded)
	assert.True(t, s.word("haus").Withdrawn, "no valid occurrence is left")
	assert.Equal(t, word.LearnedMeaning, s.word("haus").LearnedMeaning)
}

func TestStudyAnalysisUpdateEndsAReportOnlyWhenTheOccurrenceChangesIdentity(t *testing.T) {
	s := newStudyHarness(t)
	bookID, word, sentence := s.reportedHaus(3)
	id := s.openReports()[0].ID

	// An update that leaves the identity unchanged keeps the report open.
	newRun := s.republish(bookID, "")
	s.page()
	assert.True(t, s.reportOf(id).Open())
	assert.True(t, s.word("haus").Held)
	assert.Equal(t, newRun, s.material(s.word("haus").MaterialID).AnalysisRunID, "the held word's material follows the occurrence")
	review := getAs(t, s.h, "/reading/books/"+bookID+"/lemma-review?form=haus", s.cookies).Body.String()
	assert.Equal(t, 1, strings.Count(review, "Reported as a wrong word in Study."), "the marker follows the occurrence into the new run")

	// An update that gives the occurrence another lemma ends it.
	s.republish(bookID, "hof", sentence+10)
	s.page()
	s.requireReleased(id, word, domain.StudyReportEndAnalysisUpdate)
}

func TestStudyOccurrenceThatDisappearsEndsTheReport(t *testing.T) {
	s := newStudyHarness(t)
	_, word, _ := s.reportedHaus(3)
	id := s.openReports()[0].ID
	source := s.material(word.MaterialID)

	s.removeOccurrence(source)
	s.page()
	s.requireReleased(id, word, domain.StudyReportEndOccurrenceGone)
}

// The planning job reconciles reports too, and can end one before the page
// request does: the word it releases is due at the start of the Study day after
// the app's clock, not the wall clock's (#2786).
func TestStudyReportEndedFirstByPlanningReleasesTheWordForTheNextStudyDay(t *testing.T) {
	s := newStudyHarness(t)
	_, word, _ := s.reportedHaus(3)
	id := s.openReports()[0].ID
	if domain.NextStudyDayStart(time.Now()).Equal(domain.NextStudyDayStart(s.clock.Now())) {
		// The wall clock is in the harness's Study day; move the harness to
		// another, so a plan by the wall clock is caught.
		s.clock.Advance(24 * time.Hour)
	}
	source := s.material(word.MaterialID)

	s.removeOccurrence(source)
	require.NoError(t, s.planner.PlanPreparation(s.ctx, s.owner.ID, "de"))
	s.requireReleased(id, word, domain.StudyReportEndOccurrenceGone)
}

func TestStudyRemovingTheBookEndsTheReportAsOccurrenceGone(t *testing.T) {
	s := newStudyHarness(t)
	bookID, word, _ := s.reportedHaus(3)
	id := s.openReports()[0].ID
	// Another Book keeps German the learner's active Study language.
	s.seedBook("library", testutil.CorpusWord{Lemma: "tisch", UPOS: "NOUN", Occurrences: 2})

	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO book_membership(owner_id, book_id, state, removed_at) VALUES ($1, $2, 'removed', now())
		ON CONFLICT (owner_id, book_id) DO UPDATE SET state = 'removed', removed_at = now()`, s.owner.ID, bookID)
	require.NoError(t, err)
	s.page()
	s.requireReleased(id, word, domain.StudyReportEndOccurrenceGone)
}

func TestStudyOccurrenceRemovedInANewRunEndsTheReportAsGoneNotAsAnUpdate(t *testing.T) {
	s := newStudyHarness(t)
	bookID, word, _ := s.reportedHaus(3)
	id := s.openReports()[0].ID
	newRun := s.republish(bookID, "")
	s.page()
	source := s.material(s.word("haus").MaterialID)
	require.Equal(t, newRun, source.AnalysisRunID)

	s.removeOccurrence(source)
	s.page()
	s.requireReleased(id, word, domain.StudyReportEndOccurrenceGone)
}

func TestStudyDecisionEndsOnlyTheReportsItCoversForTheirOwnReason(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 3})
	s.introduceAllGotIt()
	s.clock.Advance(24 * time.Hour)
	haus, buch := s.word("haus"), s.word("buch")
	require.Equal(t, http.StatusSeeOther, s.report(haus, "wrong_word", "").Code)
	require.Equal(t, http.StatusSeeOther, s.report(buch, "wrong_word", "").Code)
	var hausReport, buchReport string
	for _, report := range s.openReports() {
		if report.CanonicalLemma == "haus" {
			hausReport = report.ID
		} else {
			buchReport = report.ID
		}
	}

	// An Analysis update gives buch's occurrence another lemma, unseen so far.
	s.republish(bookID, "hof", s.material(buch.MaterialID).SentenceOrdinal)
	// Correcting haus does not decide buch: its report ends for its own cause.
	s.decide(bookID, "haus", s.material(haus.MaterialID).SentenceOrdinal+10, "correct", "hof")
	assert.Equal(t, domain.StudyReportEndCorrected, s.reportOf(hausReport).EndReason)
	assert.Equal(t, domain.StudyReportEndAnalysisUpdate, s.reportOf(buchReport).EndReason)
}

// reportedNewHaus takes a batch and reports the new word "haus" on its
// introduction card, so the report outlives its word's Study progress.
func (s *studyHarness) reportedNewHaus() (bookID string, report domain.StudyReport, source domain.StudyMaterial) {
	s.t.Helper()
	bookID = s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.take()
	word := s.word("haus")
	require.Zero(s.t, word.Memory.Reps)
	require.Equal(s.t, http.StatusSeeOther, s.report(word, "wrong_word", "").Code)
	require.NotContains(s.t, s.studiedLemmas(), "haus", "the reported new word has no Study progress")
	reports := s.openReports()
	require.Len(s.t, reports, 1)
	require.Empty(s.t, reports[0].ProgressID)
	return bookID, reports[0], s.material(word.MaterialID)
}

func TestStudyLemmaCorrectionEndsTheReportOfANewWordWithNoStudyProgress(t *testing.T) {
	s := newStudyHarness(t)
	bookID, report, source := s.reportedNewHaus()

	s.decide(bookID, "haus", source.SentenceOrdinal, "correct", "hof")

	ended := s.reportOf(report.ID)
	assert.False(t, ended.Open())
	assert.Equal(t, domain.StudyReportEndCorrected, ended.EndReason)
	assert.Empty(t, s.openReports())
	assert.NotContains(t, s.page(), "held because you reported it")
}

func TestStudyOccurrenceThatDisappearsEndsTheReportOfANewWordWithNoStudyProgress(t *testing.T) {
	s := newStudyHarness(t)
	_, report, source := s.reportedNewHaus()

	s.removeOccurrence(source)
	s.page()

	ended := s.reportOf(report.ID)
	assert.False(t, ended.Open())
	assert.Equal(t, domain.StudyReportEndOccurrenceGone, ended.EndReason)
	assert.Empty(t, s.openReports())
}

// remediate previews and confirms a decision on the report's occurrence through
// its remediation screen, as the learner does.
func (s *studyHarness) remediate(reportID, decision, lemma string) {
	s.t.Helper()
	path := "/study/reports/" + reportID + "/remediation"
	csrf := s.cookies[0].Value
	preview := perform(s.t, s.h, "POST", path, url.Values{"csrf_token": {csrf}, "stage": {"preview"}, "decision": {decision}, "lemma": {lemma}}, s.cookies)
	require.Equal(s.t, http.StatusOK, preview.Code, preview.Body.String())
	assert.Contains(s.t, preview.Body.String(), "This is a preview only. Nothing changes until you confirm.")
	assert.True(s.t, s.reportOf(reportID).Open(), "a preview changes nothing")
	confirm := perform(s.t, s.h, "POST", path, url.Values{
		"csrf_token": {csrf}, "stage": {"confirm"}, "decision": {decision}, "lemma": {lemma},
		"fingerprint": {previewFingerprint(s.t, preview.Body.String())},
	}, s.cookies)
	require.Equal(s.t, http.StatusSeeOther, confirm.Code, confirm.Body.String())
	assert.Equal(s.t, "/de/study/reports", confirm.Header().Get("Location"))
}

func TestStudyRemediationRepairsOnlyTheReportedOccurrence(t *testing.T) {
	s := newStudyHarness(t)
	bookID, _, sentence := s.reportedHaus(3)
	id := s.openReports()[0].ID
	other := sentence + 1
	if other >= 3 {
		other = sentence - 1
	}
	occurrences, err := s.store.ListLemmaReviewOccurrences(s.ctx, s.owner.ID, bookID, "haus")
	require.NoError(t, err)
	var otherID string
	for _, occurrence := range occurrences {
		if occurrence.SentenceOrdinal == other {
			otherID = occurrence.ID()
		}
	}
	require.NotEmpty(t, otherID)

	// A form naming another occurrence still decides the reported one only.
	path := "/study/reports/" + id + "/remediation"
	csrf := s.cookies[0].Value
	preview := perform(t, s.h, "POST", path, url.Values{"csrf_token": {csrf}, "stage": {"preview"}, "decision": {"exclude"}, "target": {otherID}, "also": {otherID}}, s.cookies)
	require.Equal(t, http.StatusOK, preview.Code)
	assert.Contains(t, preview.Body.String(), "<strong>Selected occurrences:</strong> 1")

	// Another learner's, an unknown and an ended report have no screen.
	assert.Equal(t, http.StatusNotFound, getAs(t, s.h, "/study/reports/unknown/remediation", s.cookies).Code)
	s.remediate(id, "exclude", "")
	assert.Equal(t, http.StatusNotFound, getAs(t, s.h, path, s.cookies).Code, "an ended report has no screen")
	assert.Empty(t, s.openReports())
}

// matchValues lists the ticks a remediation page offers or carries: the
// value of every input named match.
func matchValues(body string) []string {
	var values []string
	for _, found := range regexp.MustCompile(`name="match" value="([^"]+)"`).FindAllStringSubmatch(body, -1) {
		values = append(values, found[1])
	}
	return values
}

// matchFingerprints lists the preview fingerprints a remediation preview carries
// for the matches' Books.
func matchFingerprints(body string) []string {
	var values []string
	for _, found := range regexp.MustCompile(`name="match_fingerprint" value="([^"]+)"`).FindAllStringSubmatch(body, -1) {
		values = append(values, found[1])
	}
	return values
}

// hausAt is the Book's occurrence of haus in the sentence.
func (s *studyHarness) hausAt(bookID string, sentence int64) domain.LemmaReviewOccurrence {
	s.t.Helper()
	for _, occurrence := range s.hausOccurrences(bookID) {
		if occurrence.SentenceOrdinal == sentence {
			return occurrence
		}
	}
	s.t.Fatalf("no occurrence of haus in sentence %d", sentence)
	return domain.LemmaReviewOccurrence{}
}

// otherHaus is an occurrence of haus in the Book other than the sentence's.
func (s *studyHarness) otherHaus(bookID string, sentence int64) domain.LemmaReviewOccurrence {
	s.t.Helper()
	for _, occurrence := range s.hausOccurrences(bookID) {
		if occurrence.SentenceOrdinal != sentence {
			return occurrence
		}
	}
	s.t.Fatalf("no other occurrence of haus than sentence %d", sentence)
	return domain.LemmaReviewOccurrence{}
}

func (s *studyHarness) hausOccurrences(bookID string) []domain.LemmaReviewOccurrence {
	s.t.Helper()
	occurrences, err := s.store.ListLemmaReviewOccurrences(s.ctx, s.owner.ID, bookID, "haus")
	require.NoError(s.t, err)
	return occurrences
}

func TestStudyRemediationDecidesOnlyTheTickedMatchesEachInItsOwnBook(t *testing.T) {
	s := newStudyHarness(t)
	firstBook, _, sentence := s.reportedHaus(3)
	secondBook := s.seedBook("second", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 2}).ID
	id := s.openReports()[0].ID
	path := "/study/reports/" + id + "/remediation"
	csrf := s.cookies[0].Value

	// The screen offers the other occurrences in every Book, none ticked.
	opened := getAs(t, s.h, path, s.cookies).Body.String()
	offered := matchValues(opened)
	assert.Len(t, offered, 4, "two other occurrences in the reported Book and two in the other")
	assert.NotContains(t, opened, "checked")
	for _, book := range []string{firstBook, secondBook} {
		assert.Contains(t, strings.Join(offered, " "), book+"/")
	}
	assert.Contains(t, opened, "Study second")
	assert.NotContains(t, strings.Join(offered, " "), s.hausAt(firstBook, sentence).ID(), "the reported occurrence is not offered again")

	// One match in each Book is ticked, with the decision's own preview.
	first := s.otherHaus(firstBook, sentence)
	second := s.hausOccurrences(secondBook)[0]
	ticked := []string{matchValue(firstBook, first.ID()), matchValue(secondBook, second.ID())}
	preview := perform(t, s.h, "POST", path, url.Values{"csrf_token": {csrf}, "stage": {"preview"}, "decision": {"correct"}, "lemma": {"hof"}, "match": ticked}, s.cookies)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	assert.ElementsMatch(t, ticked, matchValues(preview.Body.String()), "the preview carries what was ticked")
	assert.Empty(t, s.hausOccurrences(secondBook)[0].CorrectedLemma, "a preview saves nothing")

	confirm := perform(t, s.h, "POST", path, url.Values{
		"csrf_token": {csrf}, "stage": {"confirm"}, "decision": {"correct"}, "lemma": {"hof"},
		"fingerprint": {previewFingerprint(t, preview.Body.String())}, "match": matchValues(preview.Body.String()),
		"match_fingerprint": matchFingerprints(preview.Body.String()),
	}, s.cookies)
	require.Equal(t, http.StatusSeeOther, confirm.Code, confirm.Body.String())

	corrected := map[string]string{}
	for _, book := range []string{firstBook, secondBook} {
		for _, occurrence := range s.hausOccurrences(book) {
			corrected[matchValue(book, occurrence.ID())] = occurrence.CorrectedLemma
		}
	}
	want := map[string]string{}
	for _, value := range offered {
		want[value] = ""
	}
	for _, value := range ticked {
		want[value] = "hof"
	}
	want[matchValue(firstBook, s.hausAt(firstBook, sentence).ID())] = "hof"
	assert.Equal(t, want, corrected, "the reported and the ticked occurrences are corrected; every other one is unchanged")
	assert.False(t, s.reportOf(id).Open())
}

func TestStudyRemediationWithoutTicksDecidesNothingElse(t *testing.T) {
	s := newStudyHarness(t)
	firstBook, _, sentence := s.reportedHaus(3)
	secondBook := s.seedBook("second", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 2}).ID
	id := s.openReports()[0].ID

	s.remediate(id, "exclude", "")

	for _, book := range []string{firstBook, secondBook} {
		for _, occurrence := range s.hausOccurrences(book) {
			reported := book == firstBook && occurrence.SentenceOrdinal == sentence
			assert.Equal(t, reported, occurrence.Excluded, "only the reported occurrence is excluded")
		}
	}
}

func TestStudyRemediationRejectsATickThatIsNotAMatch(t *testing.T) {
	s := newStudyHarness(t)
	firstBook, _, sentence := s.reportedHaus(3)
	buch := s.hausOccurrences(firstBook)[0]
	other := s.seedBook("second", testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 1}).ID
	id := s.openReports()[0].ID
	path := "/study/reports/" + id + "/remediation"

	foreign, err := s.store.ListLemmaReviewOccurrences(s.ctx, s.owner.ID, other, "buch")
	require.NoError(t, err)
	require.NotEmpty(t, foreign)
	preview := perform(t, s.h, "POST", path, url.Values{
		"csrf_token": {s.cookies[0].Value}, "stage": {"preview"}, "decision": {"exclude"},
		"match": {matchValue(other, foreign[0].ID()), matchValue(firstBook, s.hausAt(firstBook, sentence).ID()), matchValue(firstBook, buch.ID()+"x")},
	}, s.cookies)
	assert.Equal(t, http.StatusBadRequest, preview.Code, "another word's occurrence and the reported one are not matches")
	assert.True(t, s.reportOf(id).Open())
}

func TestStudyRemediationSavesNothingWhenATickedBookChangedAfterThePreview(t *testing.T) {
	s := newStudyHarness(t)
	firstBook, _, sentence := s.reportedHaus(3)
	secondBook := s.seedBook("second", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 2}).ID
	id := s.openReports()[0].ID
	path := "/study/reports/" + id + "/remediation"
	csrf := s.cookies[0].Value
	occurrences := s.hausOccurrences(secondBook)

	preview := perform(t, s.h, "POST", path, url.Values{
		"csrf_token": {csrf}, "stage": {"preview"}, "decision": {"exclude"}, "match": {matchValue(secondBook, occurrences[0].ID())},
	}, s.cookies)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())

	// The learner decides another occurrence of the form in that Book elsewhere.
	s.decide(secondBook, "haus", occurrences[1].SentenceOrdinal, "exclude", "")

	confirm := perform(t, s.h, "POST", path, url.Values{
		"csrf_token": {csrf}, "stage": {"confirm"}, "decision": {"exclude"},
		"fingerprint": {previewFingerprint(t, preview.Body.String())}, "match": matchValues(preview.Body.String()),
		"match_fingerprint": matchFingerprints(preview.Body.String()),
	}, s.cookies)
	assert.Equal(t, http.StatusConflict, confirm.Code, confirm.Body.String())
	assert.True(t, s.reportOf(id).Open(), "the reported occurrence is not decided either")
	assert.False(t, s.hausAt(firstBook, sentence).Excluded)
	assert.False(t, s.hausOccurrences(secondBook)[0].Excluded)
}
