package storecontract

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StudyCorrectionsStore is the surface that lets Study react to a Lemma
// correction or an Occurrence exclusion (ADR 0134 §6). ListStudyOccurrences
// reads the occurrences that resolve to a word's identity under current
// evidence. SupersedeStudyMaterial replaces a word's current material, and
// CompleteStudyReintroductions moves a word onto its ready replacement. A word
// is withdrawn and returned with SetStudyWordWithdrawn.
type StudyCorrectionsStore interface {
	ListStudyOccurrences(owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error)
	// FindStudyOccurrences returns what the correction check decides from
	// without listing every occurrence: the given source when it still resolves
	// to the identity, otherwise one valid occurrence, otherwise none.
	FindStudyOccurrences(owner string, identity domain.StudyItem, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, error)
	SupersedeStudyMaterial(owner, materialID string) error
	SetStudyWordWithdrawn(owner, progressID string, withdrawn bool, now time.Time) error
	CompleteStudyReintroductions(owner, language string, now time.Time) error
	// StudyOccurrenceLocator completes an occurrence with the unit and token
	// span an Analysis update carries decisions across runs by. A token that no
	// longer exists is domain.ErrNotFound. RelocateStudyMaterial names the
	// occurrence a word's ready, current material now comes from, and
	// SetStudyMaterialEvidence records what a preparation attempt saw on a
	// pending record.
	StudyOccurrenceLocator(owner string, ref domain.StudyOccurrenceRef) (domain.StudyOccurrenceRef, error)
	RelocateStudyMaterial(owner, materialID string, ref domain.StudyOccurrenceRef) error
	SetStudyMaterialEvidence(owner, materialID, evidence string) error
	// RecordStudyMaterialNoDictionaryEntry settles a pending record as
	// unpreparable for having no dictionary entry, with the dictionary pin that
	// was asked. ReopenStudyMaterialNoDictionaryEntry makes it pending again
	// only for a different pin; anything else is domain.ErrNotFound.
	RecordStudyMaterialNoDictionaryEntry(owner, materialID, pin string) error
	ReopenStudyMaterialNoDictionaryEntry(owner, materialID, pin string) error
}

func studyCorrectionsScenarios() []Scenario {
	return []Scenario{
		{Name: "study occurrences follow corrections and exclusions, and a correction puts nothing into study", Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedReviewBook(t, h)
			weg, pfad := studyNoun("weg"), studyNoun("pfad")

			before, err := h.StudyCorrections().ListStudyOccurrences(owner, weg)
			require.NoError(t, err)
			require.Len(t, before, 2)

			corrected := reviewProposal(h, bookID, "correct", "pfad", reviewOccurrence(t, h, bookID, 0))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(corrected, previewProposal(t, h, corrected)))
			after, err := h.StudyCorrections().ListStudyOccurrences(owner, weg)
			require.NoError(t, err)
			assert.Equal(t, before[1:], after, "a correction moves the occurrence out of the word")
			moved, err := h.StudyCorrections().ListStudyOccurrences(owner, pfad)
			require.NoError(t, err)
			assert.Equal(t, before[:1], moved, "and into the identity it points to")

			excluded := reviewProposal(h, bookID, "exclude", "", reviewOccurrence(t, h, bookID, 1))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(excluded, previewProposal(t, h, excluded)))
			none, err := h.StudyCorrections().ListStudyOccurrences(owner, weg)
			require.NoError(t, err)
			assert.Empty(t, none, "an exclusion removes the occurrence")

			progress, err := h.StudyProgress().ListStudyProgress(owner, Language)
			require.NoError(t, err)
			assert.Empty(t, progress, "the identity a correction points to is never put into study")
			other, err := h.StudyCorrections().ListStudyOccurrences(h.Seeds().OtherOwner(), pfad)
			require.NoError(t, err)
			assert.Empty(t, other, "occurrences belong to their learner")
		}},
		{Name: "the correction check finds the source, else one valid occurrence, as the full listing decides", Run: func(t *testing.T, h Harness) {
			owner, corrections := h.Seeds().Owner(), h.StudyCorrections()
			bookID := seedReviewBook(t, h)
			weg, pfad := studyNoun("weg"), studyNoun("pfad")
			all, err := corrections.ListStudyOccurrences(owner, weg)
			require.NoError(t, err)
			require.Len(t, all, 2)
			bare := func(ref domain.StudyOccurrenceRef) domain.StudyOccurrenceRef {
				ref.UnitID, ref.StartOffset, ref.EndOffset = "", 0, 0
				return ref
			}

			for _, source := range all {
				found, err := corrections.FindStudyOccurrences(owner, weg, source)
				require.NoError(t, err)
				assert.Equal(t, []domain.StudyOccurrenceRef{source}, found, "a source that resolves is its own witness")
			}
			found, err := corrections.FindStudyOccurrences(owner, weg, bare(all[1]))
			require.NoError(t, err)
			assert.Equal(t, []domain.StudyOccurrenceRef{all[1]}, found, "an unlocated source is still found in its own run")

			corrected := reviewProposal(h, bookID, "correct", "pfad", reviewOccurrence(t, h, bookID, 0))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(corrected, previewProposal(t, h, corrected)))
			found, err = corrections.FindStudyOccurrences(owner, weg, all[0])
			require.NoError(t, err)
			assert.Equal(t, all[1:], found, "a corrected-away source yields another valid occurrence")
			found, err = corrections.FindStudyOccurrences(owner, pfad, all[0])
			require.NoError(t, err)
			assert.Equal(t, all[:1], found, "and the identity a correction points to resolves it")

			other := all[0]
			other.SentenceOrdinal, other.TokenOrdinal = 98, 97
			found, err = corrections.FindStudyOccurrences(owner, weg, other)
			require.NoError(t, err)
			assert.Equal(t, all[1:], found, "a source that does not exist yields another valid occurrence")

			excluded := reviewProposal(h, bookID, "exclude", "", reviewOccurrence(t, h, bookID, 1))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(excluded, previewProposal(t, h, excluded)))
			found, err = corrections.FindStudyOccurrences(owner, weg, all[0])
			require.NoError(t, err)
			assert.Empty(t, found, "no valid occurrence remains")
			found, err = corrections.FindStudyOccurrences(owner, weg, all[1])
			require.NoError(t, err)
			assert.Empty(t, found, "an excluded source does not resolve")
			found, err = corrections.FindStudyOccurrences(h.Seeds().OtherOwner(), pfad, all[0])
			require.NoError(t, err)
			assert.Empty(t, found, "occurrences belong to their learner")
		}},
		{Name: "a re-introduced word moves onto new material and keeps its scheduling state", Run: func(t *testing.T, h Harness) {
			owner, material, progressStore, corrections := h.Seeds().Owner(), h.Study(), h.StudyProgress(), h.StudyCorrections()
			bookID := seedReviewBook(t, h)
			weg := studyNoun("weg")
			occurrences, err := corrections.ListStudyOccurrences(owner, weg)
			require.NoError(t, err)
			require.Len(t, occurrences, 2)

			first := studyCorrectionMaterial(t, material, owner, weg, occurrences[0])
			now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
			_, err = progressStore.TakeStudyBatch(owner, Language, "2026-11-25", now, 1, []domain.StudyBatchItem{{StudyItem: weg, MaterialID: first}})
			require.NoError(t, err)
			kept := domain.StudyMemory{State: domain.StudyStateReview, Due: now.Add(72 * time.Hour), Stability: 4.2, Difficulty: 5.1, ScheduledDays: 3, Reps: 2, Lapses: 1, LastReview: now}
			listed, err := progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			require.NoError(t, progressStore.RecordStudyGrade(owner, domain.StudyGradeRecord{ProgressID: listed[0].ID, Grade: domain.StudyGradeGotIt, ReviewedAt: now, StudyDay: "2026-11-25", After: kept}))
			word := listed[0]
			assert.False(t, word.Corrected())

			corrected := reviewProposal(h, bookID, "correct", "pfad", reviewOccurrence(t, h, bookID, 0))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(corrected, previewProposal(t, h, corrected)))
			require.NoError(t, corrections.SupersedeStudyMaterial(owner, first))
			require.ErrorIs(t, corrections.SupersedeStudyMaterial(owner, first), domain.ErrNotFound, "material is replaced once")
			require.ErrorIs(t, corrections.SupersedeStudyMaterial(h.Seeds().OtherOwner(), first), domain.ErrNotFound, "another learner cannot replace it")

			current, err := material.ListCurrentStudyMaterial(owner, Language)
			require.NoError(t, err)
			assert.Empty(t, current, "the replaced material is no longer current")
			old, err := material.GetStudyMaterial(owner, first)
			require.NoError(t, err)
			assert.Equal(t, "a way", old.Gloss, "but it is kept for the review log")
			listed, err = progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			assert.True(t, listed[0].Reintroducing, "the word waits for new material")
			assert.False(t, domain.StudyCardIsDue(listed[0], kept.Due.Add(time.Hour)), "and has no card until it is ready")
			require.NoError(t, corrections.CompleteStudyReintroductions(owner, Language, now.Add(time.Hour)))
			listed, err = progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, first, listed[0].MaterialID, "a word is not moved before its new material is ready")

			replacement, created, err := material.ReserveStudyMaterial(owner, weg)
			require.NoError(t, err)
			require.True(t, created, "a word whose material was replaced is reserved again")
			require.NoError(t, corrections.CompleteStudyReintroductions(owner, Language, now.Add(time.Hour)))
			listed, err = progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, first, listed[0].MaterialID, "nor while it is still being prepared")

			content := studyReadyContent()
			content.BookID, content.AnalysisRunID, content.CorpusID = occurrences[1].BookID, occurrences[1].AnalysisRunID, occurrences[1].CorpusID
			content.SentenceOrdinal, content.TokenOrdinal, content.Gloss = occurrences[1].SentenceOrdinal, occurrences[1].TokenOrdinal, "a road"
			require.NoError(t, material.RecordStudyMaterialReady(owner, replacement, content))
			require.NoError(t, corrections.CompleteStudyReintroductions(owner, Language, now.Add(time.Hour)))

			listed, err = progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			reintroduced := listed[0]
			assert.Equal(t, replacement, reintroduced.MaterialID)
			assert.Equal(t, "a road", reintroduced.LearnedMeaning, "the learned meaning is re-anchored to the new Gloss")
			assert.False(t, reintroduced.Reintroducing)
			assert.True(t, reintroduced.Corrected(), "the card is marked as corrected")
			assert.True(t, now.Add(time.Hour).Equal(reintroduced.CorrectedAt))
			assert.Equal(t, word.ID, reintroduced.ID)
			assert.True(t, kept.Due.Equal(reintroduced.Memory.Due), "the FSRS state is kept")
			assert.Equal(t, kept.State, reintroduced.Memory.State)
			assert.InDelta(t, kept.Stability, reintroduced.Memory.Stability, 1e-9)
			assert.InDelta(t, kept.Difficulty, reintroduced.Memory.Difficulty, 1e-9)
			assert.Equal(t, kept.Reps, reintroduced.Memory.Reps)
			assert.Equal(t, kept.Lapses, reintroduced.Memory.Lapses)
			logs, err := progressStore.ListStudyReviewLog(owner, word.ID)
			require.NoError(t, err)
			require.Len(t, logs, 1, "the review history is kept")
			assert.Equal(t, first, logs[0].MaterialID)

			card, err := progressStore.NextStudyCard(owner, Language, kept.Due)
			require.NoError(t, err)
			assert.Equal(t, replacement, card.Material.ID, "the next review shows the new material")
			require.NoError(t, progressStore.RecordStudyGrade(owner, domain.StudyGradeRecord{ProgressID: word.ID, ExpectedReps: 2, Grade: domain.StudyGradeGotIt, ReviewedAt: kept.Due, StudyDay: "2026-11-28",
				After: domain.StudyMemory{State: domain.StudyStateReview, Due: kept.Due.Add(240 * time.Hour), Stability: 9, Difficulty: 5, ScheduledDays: 10, Reps: 3, Lapses: 1, LastReview: kept.Due}}))
			listed, err = progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			assert.False(t, listed[0].Corrected(), "answering the corrected card clears the mark")
		}},
		{Name: "a word with no valid occurrence is withdrawn with its progress kept, and returns", Run: func(t *testing.T, h Harness) {
			owner, material, progressStore, corrections := h.Seeds().Owner(), h.Study(), h.StudyProgress(), h.StudyCorrections()
			seedReviewBook(t, h)
			weg := studyNoun("weg")
			occurrences, err := corrections.ListStudyOccurrences(owner, weg)
			require.NoError(t, err)
			require.NotEmpty(t, occurrences)
			id := studyCorrectionMaterial(t, material, owner, weg, occurrences[0])
			now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
			_, err = progressStore.TakeStudyBatch(owner, Language, "2026-11-25", now, 1, []domain.StudyBatchItem{{StudyItem: weg, MaterialID: id}})
			require.NoError(t, err)
			listed, err := progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			require.Len(t, listed, 1)

			require.NoError(t, corrections.SetStudyWordWithdrawn(owner, listed[0].ID, true, now))
			require.NoError(t, corrections.SetStudyWordWithdrawn(owner, listed[0].ID, true, now), "withdrawing twice changes nothing")
			listed, err = progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			require.Len(t, listed, 1, "its progress is kept")
			assert.True(t, listed[0].Withdrawn)
			assert.Equal(t, "a way", listed[0].LearnedMeaning)
			_, err = progressStore.NextStudyCard(owner, Language, now.Add(time.Hour))
			require.ErrorIs(t, err, domain.ErrNotFound, "a withdrawn word is not due")
			require.ErrorIs(t, corrections.SetStudyWordWithdrawn(h.Seeds().OtherOwner(), listed[0].ID, false, now), domain.ErrNotFound)

			require.NoError(t, corrections.SetStudyWordWithdrawn(owner, listed[0].ID, false, now))
			card, err := progressStore.NextStudyCard(owner, Language, now.Add(time.Hour))
			require.NoError(t, err)
			assert.Equal(t, weg, card.Progress.StudyItem, "it returns when evidence reappears")
		}},
		{Name: "a word whose replacement material cannot be prepared is withdrawn", Run: func(t *testing.T, h Harness) {
			owner, material, progressStore, corrections := h.Seeds().Owner(), h.Study(), h.StudyProgress(), h.StudyCorrections()
			seedReviewBook(t, h)
			weg := studyNoun("weg")
			occurrences, err := corrections.ListStudyOccurrences(owner, weg)
			require.NoError(t, err)
			require.NotEmpty(t, occurrences)
			id := studyCorrectionMaterial(t, material, owner, weg, occurrences[0])
			now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
			_, err = progressStore.TakeStudyBatch(owner, Language, "2026-11-25", now, 1, []domain.StudyBatchItem{{StudyItem: weg, MaterialID: id}})
			require.NoError(t, err)

			require.NoError(t, corrections.SupersedeStudyMaterial(owner, id))
			replacement, created, err := material.ReserveStudyMaterial(owner, weg)
			require.NoError(t, err)
			require.True(t, created)
			require.NoError(t, material.RecordStudyMaterialUnpreparable(owner, replacement, domain.StudyOmissionSentenceQuality))
			require.NoError(t, corrections.CompleteStudyReintroductions(owner, Language, now))
			listed, err := progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			assert.True(t, listed[0].Withdrawn, "no usable occurrence is left")
			assert.Empty(t, listed[0].WithdrawnReason, "a correction's withdrawal has no other reason")
			assert.Equal(t, id, listed[0].MaterialID)
			assert.Equal(t, int64(0), listed[0].Memory.Reps)
		}},
		{Name: "an occurrence has a locator, material follows it to a new run, and an attempt records what it saw", Run: func(t *testing.T, h Harness) {
			owner, material, corrections := h.Seeds().Owner(), h.Study(), h.StudyCorrections()
			seedReviewBook(t, h)
			weg := studyNoun("weg")
			occurrences, err := corrections.ListStudyOccurrences(owner, weg)
			require.NoError(t, err)
			require.Len(t, occurrences, 2)
			assert.NotEmpty(t, occurrences[0].UnitID, "an occurrence carries the locator an Analysis update carries decisions by")
			assert.NotEqual(t, occurrences[0].StartOffset, occurrences[1].StartOffset)

			bare := occurrences[0]
			bare.UnitID, bare.StartOffset, bare.EndOffset = "", 0, 0
			located, err := corrections.StudyOccurrenceLocator(owner, bare)
			require.NoError(t, err)
			assert.Equal(t, occurrences[0], located)
			missing := bare
			missing.TokenOrdinal = 99
			_, err = corrections.StudyOccurrenceLocator(owner, missing)
			require.ErrorIs(t, err, domain.ErrNotFound)

			id := studyCorrectionMaterial(t, material, owner, weg, occurrences[0])
			require.NoError(t, corrections.RelocateStudyMaterial(owner, id, occurrences[1]))
			moved, err := material.GetStudyMaterial(owner, id)
			require.NoError(t, err)
			assert.Equal(t, occurrences[1].SentenceOrdinal, moved.SentenceOrdinal)
			assert.Equal(t, occurrences[1].TokenOrdinal, moved.TokenOrdinal)
			assert.Equal(t, occurrences[1].AnalysisRunID, moved.AnalysisRunID)
			assert.Equal(t, "a way", moved.Gloss, "the material's content is kept")
			assert.Equal(t, studyReadyContent().Sentence, moved.Sentence)
			require.ErrorIs(t, corrections.RelocateStudyMaterial(h.Seeds().OtherOwner(), id, occurrences[0]), domain.ErrNotFound)

			pending, created, err := material.ReserveStudyMaterial(owner, studyNoun("pfad"))
			require.NoError(t, err)
			require.True(t, created)
			require.ErrorIs(t, corrections.RelocateStudyMaterial(owner, pending, occurrences[0]), domain.ErrNotFound, "only ready material is relocated")
			require.NoError(t, corrections.SetStudyMaterialEvidence(owner, pending, "fingerprint-1"))
			require.NoError(t, material.RecordStudyMaterialUnpreparable(owner, pending, domain.StudyOmissionMeaning))
			skipped, err := material.GetStudyMaterial(owner, pending)
			require.NoError(t, err)
			assert.Equal(t, "fingerprint-1", skipped.EvidenceFingerprint)
			require.ErrorIs(t, corrections.SetStudyMaterialEvidence(owner, pending, "fingerprint-2"), domain.ErrNotFound, "a settled record is not rewritten")
		}},
		{Name: "a no-dictionary-entry omission records its pin and is reopened only by a different pin", Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			material, corrections := h.Study(), h.StudyCorrections()
			weg := studyNoun("weg")
			id, created, err := material.ReserveStudyMaterial(owner, weg)
			require.NoError(t, err)
			require.True(t, created)
			require.ErrorIs(t, corrections.ReopenStudyMaterialNoDictionaryEntry(owner, id, "pin-2"), domain.ErrNotFound, "a pending record is not reopened")
			require.NoError(t, corrections.SetStudyMaterialEvidence(owner, id, "fingerprint-1"))
			require.NoError(t, corrections.RecordStudyMaterialNoDictionaryEntry(owner, id, "pin-1"))
			require.ErrorIs(t, corrections.RecordStudyMaterialNoDictionaryEntry(owner, id, "pin-1"), domain.ErrNotFound, "a settled record is not rewritten")
			skipped, err := material.GetStudyMaterial(owner, id)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialUnpreparable, skipped.State)
			assert.Equal(t, domain.StudyOmissionNoDictionaryEntry, skipped.OmissionReason)
			assert.Equal(t, "pin-1", skipped.OmissionDictionaryPin)

			require.ErrorIs(t, corrections.ReopenStudyMaterialNoDictionaryEntry(owner, id, "pin-1"), domain.ErrNotFound, "the same pin is not retried")
			require.ErrorIs(t, corrections.ReopenStudyMaterialNoDictionaryEntry(h.Seeds().OtherOwner(), id, "pin-2"), domain.ErrNotFound)
			require.NoError(t, corrections.ReopenStudyMaterialNoDictionaryEntry(owner, id, "pin-2"))
			reopened, err := material.GetStudyMaterial(owner, id)
			require.NoError(t, err)
			assert.Equal(t, id, reopened.ID, "the same record is prepared again")
			assert.Equal(t, domain.StudyMaterialPending, reopened.State)
			assert.Empty(t, reopened.OmissionReason)
			assert.Empty(t, reopened.OmissionDictionaryPin)
			assert.Empty(t, reopened.EvidenceFingerprint)

			other, created, err := material.ReserveStudyMaterial(owner, studyNoun("pfad"))
			require.NoError(t, err)
			require.True(t, created)
			require.NoError(t, material.RecordStudyMaterialUnpreparable(owner, other, domain.StudyOmissionMeaning))
			require.ErrorIs(t, corrections.ReopenStudyMaterialNoDictionaryEntry(owner, other, "pin-2"), domain.ErrNotFound, "a meaning omission is never reopened")
			require.ErrorIs(t, corrections.ReopenStudyMaterialNoDictionaryEntry(owner, "missing", "pin-2"), domain.ErrNotFound)
			require.ErrorIs(t, corrections.RecordStudyMaterialNoDictionaryEntry(owner, "missing", "pin-2"), domain.ErrNotFound)
		}},
	}
}

// studyCorrectionMaterial prepares a word's material from one occurrence.
func studyCorrectionMaterial(t *testing.T, store StudyStore, owner string, identity domain.StudyItem, source domain.StudyOccurrenceRef) string {
	t.Helper()
	id, created, err := store.ReserveStudyMaterial(owner, identity)
	require.NoError(t, err)
	require.True(t, created)
	content := studyReadyContent()
	content.BookID, content.AnalysisRunID, content.CorpusID = source.BookID, source.AnalysisRunID, source.CorpusID
	content.SentenceOrdinal, content.TokenOrdinal, content.Gloss = source.SentenceOrdinal, source.TokenOrdinal, "a way"
	require.NoError(t, store.RecordStudyMaterialReady(owner, id, content))
	return id
}
