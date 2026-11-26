package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReviewStudyEvidence(t *testing.T) {
	source := StudyOccurrenceRef{BookID: "b", AnalysisRunID: "run-1", CorpusID: "c", SentenceOrdinal: 4, TokenOrdinal: 2}
	other := StudyOccurrenceRef{BookID: "b", AnalysisRunID: "run-1", CorpusID: "c", SentenceOrdinal: 9, TokenOrdinal: 1}
	updated := source
	updated.AnalysisRunID = "run-2"
	tests := []struct {
		name     string
		evidence StudyEvidence
		want     StudyEvidenceAction
	}{
		{"the introducing occurrence still resolves", StudyEvidence{Source: source, Valid: []StudyOccurrenceRef{other, source}}, StudyEvidenceNone},
		{"a correction moved it but another occurrence is valid", StudyEvidence{Source: source, Valid: []StudyOccurrenceRef{other}}, StudyEvidenceReintroduce},
		{"an Analysis update replaced its run but the same occurrence still resolves", StudyEvidence{Source: located(source), Valid: []StudyOccurrenceRef{located(updated)}}, StudyEvidenceNone},
		{"an Analysis update replaced its run and the occurrence resolves elsewhere now", StudyEvidence{Source: located(source), Valid: []StudyOccurrenceRef{other}}, StudyEvidenceReintroduce},
		{"an unlocated source cannot be matched across runs", StudyEvidence{Source: source, Valid: []StudyOccurrenceRef{updated}}, StudyEvidenceReintroduce},
		{"no valid occurrence remains", StudyEvidence{Source: source}, StudyEvidenceWithdraw},
		{"a withdrawn word with no evidence stays withdrawn", StudyEvidence{Withdrawn: true, Source: source}, StudyEvidenceNone},
		{"a withdrawn word returns when its source resolves again", StudyEvidence{Withdrawn: true, Source: source, Valid: []StudyOccurrenceRef{source}}, StudyEvidenceReturn},
		{"a withdrawn word returns and is re-introduced when evidence reappears elsewhere", StudyEvidence{Withdrawn: true, Source: source, Valid: []StudyOccurrenceRef{other}}, StudyEvidenceReturnReintroduce},
		{"a word withdrawn after unpreparable material does not return on the evidence that failed", StudyEvidence{Withdrawn: true, Source: source, Valid: []StudyOccurrenceRef{other}, CurrentUnpreparable: true, FailedEvidence: StudyEvidenceFingerprint([]StudyOccurrenceRef{other})}, StudyEvidenceNone},
		{"it returns when the evidence differs from what the failed attempt saw", StudyEvidence{Withdrawn: true, Source: source, Valid: []StudyOccurrenceRef{other, updated}, CurrentUnpreparable: true, FailedEvidence: StudyEvidenceFingerprint([]StudyOccurrenceRef{other})}, StudyEvidenceReturnReintroduce},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, ReviewStudyEvidence(test.evidence))
		})
	}
}

// located gives an occurrence the stable locator an Analysis update carries
// decisions across runs by: its Book, unit and token span.
func located(ref StudyOccurrenceRef) StudyOccurrenceRef {
	ref.UnitID, ref.StartOffset, ref.EndOffset = "unit-1", 40, 45
	return ref
}

func TestStudyRelocationFindsTheSameOccurrenceInANewRun(t *testing.T) {
	old := located(StudyOccurrenceRef{BookID: "b", AnalysisRunID: "run-1", CorpusID: "c1", SentenceOrdinal: 4, TokenOrdinal: 2})
	moved := located(StudyOccurrenceRef{BookID: "b", AnalysisRunID: "run-2", CorpusID: "c2", SentenceOrdinal: 7, TokenOrdinal: 1})
	got, ok := StudyRelocation(StudyEvidence{Source: old, Valid: []StudyOccurrenceRef{moved}})
	assert.True(t, ok)
	assert.Equal(t, moved, got)
	_, ok = StudyRelocation(StudyEvidence{Source: old, Valid: []StudyOccurrenceRef{old}})
	assert.False(t, ok, "an occurrence in the same run is already where it was")
	_, ok = StudyRelocation(StudyEvidence{Source: old, Valid: nil})
	assert.False(t, ok)
}

func TestStudyEvidenceFingerprintIgnoresOrderAndTracksEvidence(t *testing.T) {
	a := StudyOccurrenceRef{BookID: "b", AnalysisRunID: "r", CorpusID: "c", SentenceOrdinal: 1}
	b := StudyOccurrenceRef{BookID: "b", AnalysisRunID: "r", CorpusID: "c", SentenceOrdinal: 2}
	assert.Equal(t, StudyEvidenceFingerprint([]StudyOccurrenceRef{a, b}), StudyEvidenceFingerprint([]StudyOccurrenceRef{b, a}))
	assert.NotEqual(t, StudyEvidenceFingerprint([]StudyOccurrenceRef{a}), StudyEvidenceFingerprint([]StudyOccurrenceRef{a, b}))
	assert.NotEqual(t, StudyEvidenceFingerprint(nil), StudyEvidenceFingerprint([]StudyOccurrenceRef{a}))
}

func TestStudyMaterialSourceNamesItsOccurrence(t *testing.T) {
	material := StudyMaterial{BookID: "b", AnalysisRunID: "r", CorpusID: "c", SentenceOrdinal: 1, TokenOrdinal: 2}
	assert.Equal(t, StudyOccurrenceRef{BookID: "b", AnalysisRunID: "r", CorpusID: "c", SentenceOrdinal: 1, TokenOrdinal: 2}, material.Source())
}

func TestStudyCorrectedCardAndReintroducingWord(t *testing.T) {
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	review := StudyMemory{State: StudyStateReview, Due: now.Add(-time.Hour), Reps: 3, LastReview: now.Add(-48 * time.Hour)}
	word := StudyProgress{Memory: review}
	assert.False(t, word.Corrected(), "a word never corrected")
	word.CorrectedAt = now.Add(-time.Minute)
	assert.True(t, word.Corrected(), "its next card is the corrected re-introduction")
	word.Memory.LastReview = now
	assert.False(t, word.Corrected(), "answering it clears the mark")

	word = StudyProgress{Memory: review, Reintroducing: true}
	assert.False(t, StudyCardIsDue(word, now), "a word waiting for new material has no card to show")
	assert.False(t, StudyCountsToday(word, now), "and it does not hold the finish line open")
	assert.True(t, StudyAwaitsMaterial(word, now), "it is reported as waiting for new material")
}
