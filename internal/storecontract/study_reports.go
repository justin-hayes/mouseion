package storecontract

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StudyReportStore is the Study report surface under test (ADR 0135). A report
// is of the word's current Study material; the store copies the reported
// occurrence, sentence, Gloss and translation from it. An open Wrong-word
// report holds the word (domain.StudyProgress.Held); an open
// Unsuitable-context report retires its occurrence.
//
// CreateStudyReport takes the word's lock as a grade does: a card answered
// since it was shown is domain.ErrStudyStale, and reporting the same material
// twice while a report is open returns that report. EndStudyReport ends a
// report because its occurrence was repaired or is gone; a repaired Wrong-word
// report releases its word, due at the start of the next Study day. DismissStudyReport
// ends a Wrong-word report without a repair and moves the word's due time later
// by the time it was held. UndoStudyReport ends an Unsuitable-context report
// and restores the word's material; it is domain.ErrStudyStale once another
// Study action followed the report. A report that is missing, foreign or ended
// is domain.ErrNotFound.
type StudyReportStore interface {
	CreateStudyReport(owner string, record domain.StudyReportRecord) (domain.StudyReport, error)
	GetStudyReport(owner, id string) (domain.StudyReport, error)
	GetOpenStudyReport(owner, language, progressID string, kind domain.StudyReportKind) (domain.StudyReport, error)
	ListOpenStudyReports(owner, language string, kind domain.StudyReportKind) ([]domain.StudyReport, error)
	ListOpenWrongWordReportsForBook(owner, bookID string) ([]domain.StudyReport, error)
	ListRetiredStudyOccurrences(owner, progressID string) ([]domain.StudyOccurrenceRef, error)
	EndStudyReport(owner, id string, reason domain.StudyReportEnd, now time.Time) error
	DismissStudyReport(owner, id string, now time.Time) error
	UndoStudyReport(owner, id string, now time.Time) error
	// StudyOccurrenceState says whether a reported occurrence is still in the
	// current analysis of a Book in the library, and whether the learner
	// excluded or corrected it.
	StudyOccurrenceState(owner, language string, source domain.StudyOccurrenceRef) (domain.StudyOccurrenceState, error)
}

var studyReportDay = time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)

func reportRecord(word domain.StudyProgress, kind domain.StudyReportKind, note string) domain.StudyReportRecord {
	return domain.StudyReportRecord{ProgressID: word.ID, ExpectedReps: word.Memory.Reps, Kind: kind, Note: note, CreatedAt: studyReportDay.Add(time.Hour)}
}

// requireSameMemory checks that a word's scheduling state is as it was.
func requireSameMemory(t *testing.T, want, got domain.StudyMemory) {
	t.Helper()
	assert.True(t, want.Due.Equal(got.Due), "due %v, want %v", got.Due, want.Due)
	assert.True(t, want.LastReview.Equal(got.LastReview), "last review %v, want %v", got.LastReview, want.LastReview)
	want.Due, got.Due, want.LastReview, got.LastReview = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	assert.Equal(t, want, got)
}

// gradedStudyWord introduces a word and grades it once, so it has a review log.
func gradedStudyWord(t *testing.T, h Harness, owner string, identity domain.StudyIdentity) domain.StudyProgress {
	t.Helper()
	word := introducedStudyWord(t, h, owner, identity)
	after := domain.StudyMemory{State: domain.StudyStateReview, Due: studyReportDay.Add(72 * time.Hour), Stability: 4.2, Difficulty: 5.1, ScheduledDays: 3, Reps: 1, LastReview: studyReportDay}
	require.NoError(t, h.StudyProgress().RecordStudyGrade(owner, domain.StudyGradeRecord{ProgressID: word.ID, Grade: domain.StudyGradeGotIt, ReviewedAt: studyReportDay, StudyDay: "2026-11-25", After: after}))
	word.Memory = after
	return word
}

func studyReportScenarios() []Scenario {
	return []Scenario{
		{Name: "a Wrong-word report copies the occurrence, holds the word and grades nothing", Run: func(t *testing.T, h Harness) {
			owner, reports, progress := h.Seeds().Owner(), h.StudyReports(), h.StudyProgress()
			word := gradedStudyWord(t, h, owner, studyNoun("haus"))
			due := word.Memory.Due.Add(24 * time.Hour)

			report, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, "  kam … umhin  "))
			require.NoError(t, err)
			assert.NotEmpty(t, report.ID)
			assert.Equal(t, word.StudyIdentity, report.StudyIdentity)
			assert.Equal(t, word.ID, report.ProgressID)
			assert.Equal(t, word.MaterialID, report.MaterialID)
			assert.Equal(t, domain.StudyReportWrongWord, report.Kind)
			assert.Equal(t, "kam … umhin", report.Note, "the note is trimmed")
			assert.True(t, report.Open())
			assert.Equal(t, "Das Haus steht am Ende der Strasse.", report.Sentence)
			assert.Equal(t, "a building", report.Gloss)
			assert.Equal(t, "The house is at the end of the street.", report.SentenceTranslation)
			material, err := h.Study().GetStudyMaterial(owner, word.MaterialID)
			require.NoError(t, err)
			assert.Equal(t, material.Source(), report.Source, "the occurrence is copied from the material")

			listed, err := progress.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			assert.True(t, listed[0].Held)
			requireSameMemory(t, word.Memory, listed[0].Memory)
			logs, err := progress.ListStudyReviewLog(owner, word.ID)
			require.NoError(t, err)
			assert.Len(t, logs, 1, "the report writes no review-log row")
			_, err = progress.NextStudyCard(owner, Language, due)
			require.ErrorIs(t, err, domain.ErrNotFound, "a held word is not due")
			assert.False(t, domain.StudyCardIsDue(listed[0], due))
			assert.Zero(t, domain.StudyFinishLine(listed, due).Remaining)
		}},
		{Name: "reporting the same material twice while a report is open yields one report", Run: func(t *testing.T, h Harness) {
			owner, reports := h.Seeds().Owner(), h.StudyReports()
			word := introducedStudyWord(t, h, owner, studyNoun("haus"))
			first, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, "first"))
			require.NoError(t, err)
			again, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, "second"))
			require.NoError(t, err)
			assert.Equal(t, first.ID, again.ID)
			assert.Equal(t, "first", again.Note, "the first note stands")
			listed, err := reports.ListOpenStudyReports(owner, Language, domain.StudyReportWrongWord)
			require.NoError(t, err)
			assert.Len(t, listed, 1)

			require.NoError(t, reports.EndStudyReport(owner, first.ID, domain.StudyReportEndCorrected, studyReportDay.Add(2*time.Hour)))
			next, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, "third"))
			require.NoError(t, err)
			assert.NotEqual(t, first.ID, next.ID, "an ended report is not repeated")
		}},
		{Name: "a report is refused for a card answered since, a withdrawn word and a word that is not the learner's", Run: func(t *testing.T, h Harness) {
			owner, reports := h.Seeds().Owner(), h.StudyReports()
			word := gradedStudyWord(t, h, owner, studyNoun("haus"))
			stale := reportRecord(word, domain.StudyReportWrongWord, "")
			stale.ExpectedReps = 0
			_, err := reports.CreateStudyReport(owner, stale)
			require.ErrorIs(t, err, domain.ErrStudyStale, "the card was graded after it was shown")
			_, err = reports.CreateStudyReport(h.Seeds().OtherOwner(), reportRecord(word, domain.StudyReportWrongWord, ""))
			require.ErrorIs(t, err, domain.ErrNotFound)
			_, err = reports.CreateStudyReport(owner, domain.StudyReportRecord{ProgressID: "not-a-uuid", Kind: domain.StudyReportWrongWord})
			require.ErrorIs(t, err, domain.ErrNotFound)
			_, err = reports.CreateStudyReport(owner, domain.StudyReportRecord{ProgressID: word.ID, Kind: "other", ExpectedReps: 1})
			require.Error(t, err)
			long := make([]rune, domain.StudyReportNoteMax+1)
			for i := range long {
				long[i] = 'x'
			}
			_, err = reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, string(long)))
			require.Error(t, err, "a note has a bound")

			require.NoError(t, h.StudyCorrections().SetStudyWordWithdrawn(owner, word.ID, true, studyReportDay))
			_, err = reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, ""))
			require.ErrorIs(t, err, domain.ErrStudyStale, "a withdrawn word has no card")
		}},
		{Name: "open reports are listed for their learner, language, kind and Book", Run: func(t *testing.T, h Harness) {
			owner, reports := h.Seeds().Owner(), h.StudyReports()
			haus := introducedStudyWord(t, h, owner, studyNoun("haus"))
			hund := introducedStudyWord(t, h, owner, studyNoun("hund"))
			first, err := reports.CreateStudyReport(owner, reportRecord(haus, domain.StudyReportWrongWord, "a"))
			require.NoError(t, err)
			later := reportRecord(hund, domain.StudyReportWrongWord, "b")
			later.CreatedAt = later.CreatedAt.Add(time.Minute)
			second, err := reports.CreateStudyReport(owner, later)
			require.NoError(t, err)

			listed, err := reports.ListOpenStudyReports(owner, Language, domain.StudyReportWrongWord)
			require.NoError(t, err)
			require.Len(t, listed, 2)
			assert.Equal(t, []string{first.ID, second.ID}, []string{listed[0].ID, listed[1].ID}, "oldest first")
			other, err := reports.ListOpenStudyReports(h.Seeds().OtherOwner(), Language, domain.StudyReportWrongWord)
			require.NoError(t, err)
			assert.Empty(t, other, "reports belong to their learner")
			elsewhere, err := reports.ListOpenStudyReports(owner, "it", domain.StudyReportWrongWord)
			require.NoError(t, err)
			assert.Empty(t, elsewhere, "and their study language")
			unsuitable, err := reports.ListOpenStudyReports(owner, Language, domain.StudyReportUnsuitableContext)
			require.NoError(t, err)
			assert.Empty(t, unsuitable)
			byBook, err := reports.ListOpenWrongWordReportsForBook(owner, first.Source.BookID)
			require.NoError(t, err)
			require.Len(t, byBook, 1)
			assert.Equal(t, first.ID, byBook[0].ID)
			foreign, err := reports.ListOpenWrongWordReportsForBook(h.Seeds().OtherOwner(), first.Source.BookID)
			require.NoError(t, err)
			assert.Empty(t, foreign)
			open, err := reports.GetOpenStudyReport(owner, Language, haus.ID, domain.StudyReportWrongWord)
			require.NoError(t, err)
			assert.Equal(t, first.ID, open.ID)
			_, err = reports.GetOpenStudyReport(owner, Language, haus.ID, domain.StudyReportUnsuitableContext)
			require.ErrorIs(t, err, domain.ErrNotFound)
			_, err = reports.GetStudyReport(h.Seeds().OtherOwner(), first.ID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		}},
		{Name: "ending a report releases the word and records why", Run: func(t *testing.T, h Harness) {
			owner, reports, progress := h.Seeds().Owner(), h.StudyReports(), h.StudyProgress()
			word := introducedStudyWord(t, h, owner, studyNoun("haus"))
			report, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, ""))
			require.NoError(t, err)

			require.Error(t, reports.EndStudyReport(owner, report.ID, domain.StudyReportEndDismissed, studyReportDay), "dismissing has its own operation")
			require.ErrorIs(t, reports.EndStudyReport(h.Seeds().OtherOwner(), report.ID, domain.StudyReportEndCorrected, studyReportDay), domain.ErrNotFound)
			ended := studyReportDay.Add(3 * time.Hour)
			require.NoError(t, reports.EndStudyReport(owner, report.ID, domain.StudyReportEndExcluded, ended))
			require.ErrorIs(t, reports.EndStudyReport(owner, report.ID, domain.StudyReportEndCorrected, ended), domain.ErrNotFound, "a report ends once")

			got, err := reports.GetStudyReport(owner, report.ID)
			require.NoError(t, err)
			assert.False(t, got.Open())
			assert.Equal(t, domain.StudyReportEndExcluded, got.EndReason)
			assert.True(t, ended.Equal(got.EndedAt))
			listed, err := progress.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			assert.False(t, listed[0].Held, "the word is no longer held")
			assert.True(t, domain.NextStudyDayStart(ended).Equal(listed[0].Memory.Due), "a repaired word is due at the start of the next Study day, due %v", listed[0].Memory.Due)
			want := word.Memory
			want.Due = listed[0].Memory.Due
			requireSameMemory(t, want, listed[0].Memory)
			open, err := reports.ListOpenStudyReports(owner, Language, domain.StudyReportWrongWord)
			require.NoError(t, err)
			assert.Empty(t, open)
		}},
		{Name: "a reported occurrence is present until it is gone, and says whether the learner excluded or corrected it", Run: func(t *testing.T, h Harness) {
			owner, reports := h.Seeds().Owner(), h.StudyReports()
			bookID := seedReviewBook(t, h)
			all, err := h.StudyCorrections().ListStudyOccurrences(owner, studyNoun("weg"))
			require.NoError(t, err)
			require.Len(t, all, 2)

			state, err := reports.StudyOccurrenceState(owner, Language, all[0])
			require.NoError(t, err)
			assert.Equal(t, domain.StudyOccurrenceState{Present: true}, state)
			assert.Equal(t, domain.StudyReportEndAnalysisUpdate, state.EndReason())
			missing := all[0]
			missing.AnalysisRunID, missing.SentenceOrdinal, missing.TokenOrdinal, missing.UnitID, missing.StartOffset, missing.EndOffset = all[0].AnalysisRunID, 98, 97, "", 0, 0
			gone, err := reports.StudyOccurrenceState(owner, Language, missing)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyReportEndOccurrenceGone, gone.EndReason())
			foreign, err := reports.StudyOccurrenceState(h.Seeds().OtherOwner(), Language, all[0])
			require.NoError(t, err)
			assert.False(t, foreign.Present, "occurrences belong to their learner")

			corrected := reviewProposal(h, bookID, "correct", "pfad", reviewOccurrence(t, h, bookID, 0))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(corrected, previewProposal(t, h, corrected)))
			state, err = reports.StudyOccurrenceState(owner, Language, all[0])
			require.NoError(t, err)
			assert.Equal(t, domain.StudyReportEndCorrected, state.EndReason())
			excluded := reviewProposal(h, bookID, "exclude", "", reviewOccurrence(t, h, bookID, 1))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(excluded, previewProposal(t, h, excluded)))
			state, err = reports.StudyOccurrenceState(owner, Language, all[1])
			require.NoError(t, err)
			assert.Equal(t, domain.StudyReportEndExcluded, state.EndReason())
		}},
		{Name: "dismissing a Wrong-word report moves the word's due time later by the time it was held", Run: func(t *testing.T, h Harness) {
			owner, reports, progress := h.Seeds().Owner(), h.StudyReports(), h.StudyProgress()
			word := gradedStudyWord(t, h, owner, studyNoun("haus"))
			report, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, ""))
			require.NoError(t, err)
			held := 50 * time.Hour

			require.ErrorIs(t, reports.DismissStudyReport(h.Seeds().OtherOwner(), report.ID, report.CreatedAt.Add(held)), domain.ErrNotFound)
			require.NoError(t, reports.DismissStudyReport(owner, report.ID, report.CreatedAt.Add(held)))
			require.ErrorIs(t, reports.DismissStudyReport(owner, report.ID, report.CreatedAt.Add(held)), domain.ErrNotFound, "a report is dismissed once")
			got, err := reports.GetStudyReport(owner, report.ID)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyReportEndDismissed, got.EndReason)
			listed, err := progress.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			assert.False(t, listed[0].Held)
			assert.True(t, word.Memory.Due.Add(held).Equal(listed[0].Memory.Due), "due moved by %v, got %v", held, listed[0].Memory.Due)
			assert.Equal(t, word.Memory.Reps, listed[0].Memory.Reps, "nothing else changed")
			logs, err := progress.ListStudyReviewLog(owner, word.ID)
			require.NoError(t, err)
			assert.Len(t, logs, 1, "dismissing writes no review-log row")
		}},
		{Name: "an Unsuitable-context report retires its occurrence without holding the word, and Undo restores the material", Run: unsuitableContextScenario},
		{Name: "a held word cannot be graded, and showing an offer or another learner's action does not block Undo", Run: func(t *testing.T, h Harness) {
			owner, reports, progress := h.Seeds().Owner(), h.StudyReports(), h.StudyProgress()
			word := gradedStudyWord(t, h, owner, studyNoun("haus"))
			unsuitable, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportUnsuitableContext, ""))
			require.NoError(t, err)
			require.NoError(t, progress.RecordStudyBatchOffer(owner, Language, "2026-11-25", 1))
			require.NoError(t, progress.RecordStudyBatchOffer(owner, Language, "2026-11-26", 1))
			require.NoError(t, reports.UndoStudyReport(owner, unsuitable.ID, studyReportDay.Add(2*time.Hour)), "showing an offer is not a Study action")

			_, err = reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, ""))
			require.NoError(t, err)
			grade := domain.StudyGradeRecord{ProgressID: word.ID, ExpectedReps: word.Memory.Reps, Grade: domain.StudyGradeGotIt, ReviewedAt: studyReportDay.Add(3 * time.Hour), StudyDay: "2026-11-25", After: word.Memory}
			require.ErrorIs(t, progress.RecordStudyGrade(owner, grade), domain.ErrStudyStale, "a held word is not graded")
		}},
		{Name: "Undo is refused once another Study action followed the report", Run: func(t *testing.T, h Harness) {
			owner, reports := h.Seeds().Owner(), h.StudyReports()
			word := gradedStudyWord(t, h, owner, studyNoun("haus"))
			report, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportUnsuitableContext, ""))
			require.NoError(t, err)
			other := gradedStudyWord(t, h, owner, studyNoun("hund"))
			require.ErrorIs(t, reports.UndoStudyReport(owner, report.ID, studyReportDay.Add(2*time.Hour)), domain.ErrStudyStale, "a review was graded since")
			got, err := reports.GetStudyReport(owner, report.ID)
			require.NoError(t, err)
			assert.True(t, got.Open(), "the report stands")

			second, err := reports.CreateStudyReport(owner, reportRecord(other, domain.StudyReportUnsuitableContext, ""))
			require.NoError(t, err)
			require.NoError(t, reports.UndoStudyReport(owner, second.ID, studyReportDay.Add(2*time.Hour)), "nothing followed the latest report")
			third, err := reports.CreateStudyReport(owner, reportRecord(other, domain.StudyReportUnsuitableContext, ""))
			require.NoError(t, err)
			_, err = reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportWrongWord, "another report"))
			require.NoError(t, err)
			require.ErrorIs(t, reports.UndoStudyReport(owner, third.ID, studyReportDay.Add(2*time.Hour)), domain.ErrStudyStale, "another report followed")
			require.ErrorIs(t, reports.UndoStudyReport(h.Seeds().OtherOwner(), third.ID, studyReportDay), domain.ErrNotFound)
			require.ErrorIs(t, reports.UndoStudyReport(owner, second.ID, studyReportDay), domain.ErrNotFound, "an undone report is not undone again")
		}},
	}
}

func unsuitableContextScenario(t *testing.T, h Harness) {
	t.Helper()
	owner, reports, progress, material, corrections := h.Seeds().Owner(), h.StudyReports(), h.StudyProgress(), h.Study(), h.StudyCorrections()
	word := gradedStudyWord(t, h, owner, studyNoun("haus"))
	report, err := reports.CreateStudyReport(owner, reportRecord(word, domain.StudyReportUnsuitableContext, "part of an Idiom"))
	require.NoError(t, err)
	assert.Equal(t, word.MaterialID, report.PreviousMaterialID)
	assert.Equal(t, "a building", report.PreviousLearnedMeaning)
	assert.True(t, report.PreviousCorrectedAt.IsZero())

	listed, err := progress.ListStudyProgress(owner, Language)
	require.NoError(t, err)
	assert.False(t, listed[0].Held, "an Unsuitable-context report does not hold the word")
	retired, err := reports.ListRetiredStudyOccurrences(owner, word.ID)
	require.NoError(t, err)
	assert.Equal(t, []domain.StudyOccurrenceRef{report.Source}, retired)
	none, err := reports.ListRetiredStudyOccurrences(h.Seeds().OtherOwner(), word.ID)
	require.NoError(t, err)
	assert.Empty(t, none)
	wrong, err := reports.ListOpenWrongWordReportsForBook(owner, report.Source.BookID)
	require.NoError(t, err)
	assert.Empty(t, wrong, "only Wrong-word reports are listed for Lemma review")
	require.ErrorIs(t, reports.DismissStudyReport(owner, report.ID, studyReportDay), domain.ErrNotFound, "dismiss is for Wrong-word reports")

	// The word is re-introduced from another context, then the report is undone.
	require.NoError(t, corrections.SupersedeStudyMaterial(owner, word.MaterialID))
	replacement, created, err := material.ReserveStudyMaterial(owner, word.StudyIdentity)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, material.RecordStudyMaterialReady(owner, replacement, studyReadyContent()))
	require.NoError(t, corrections.CompleteStudyReintroductions(owner, Language, studyReportDay.Add(2*time.Hour)))
	moved, err := progress.ListStudyProgress(owner, Language)
	require.NoError(t, err)
	require.Equal(t, replacement, moved[0].MaterialID)

	require.NoError(t, reports.UndoStudyReport(owner, report.ID, studyReportDay.Add(3*time.Hour)))
	got, err := reports.GetStudyReport(owner, report.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StudyReportEndUndone, got.EndReason)
	restored, err := progress.ListStudyProgress(owner, Language)
	require.NoError(t, err)
	require.Len(t, restored, 1)
	assert.Equal(t, word.MaterialID, restored[0].MaterialID, "the word returns to the material it had")
	assert.Equal(t, "a building", restored[0].LearnedMeaning)
	assert.True(t, restored[0].CorrectedAt.IsZero())
	assert.False(t, restored[0].Reintroducing)
	requireSameMemory(t, word.Memory, restored[0].Memory)
	current, err := material.ListCurrentStudyMaterial(owner, Language)
	require.NoError(t, err)
	require.Len(t, current, 1)
	assert.Equal(t, word.MaterialID, current[0].ID, "and its material is current again, the replacement is not")
	retired, err = reports.ListRetiredStudyOccurrences(owner, word.ID)
	require.NoError(t, err)
	assert.Empty(t, retired, "the occurrence is no longer retired")
}
