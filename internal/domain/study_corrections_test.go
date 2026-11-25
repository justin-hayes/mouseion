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
		{"an Analysis update replaced its run", StudyEvidence{Source: source, Valid: []StudyOccurrenceRef{updated}}, StudyEvidenceReintroduce},
		{"no valid occurrence remains", StudyEvidence{Source: source}, StudyEvidenceWithdraw},
		{"a withdrawn word with no evidence stays withdrawn", StudyEvidence{Withdrawn: true, Source: source}, StudyEvidenceNone},
		{"a withdrawn word returns when its source resolves again", StudyEvidence{Withdrawn: true, Source: source, Valid: []StudyOccurrenceRef{source}}, StudyEvidenceReturn},
		{"a withdrawn word returns and is re-introduced when evidence reappears elsewhere", StudyEvidence{Withdrawn: true, Source: source, Valid: []StudyOccurrenceRef{other}}, StudyEvidenceReturnReintroduce},
		{"a word withdrawn after unpreparable material does not return", StudyEvidence{Withdrawn: true, Source: source, Valid: []StudyOccurrenceRef{other}, CurrentUnpreparable: true}, StudyEvidenceNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, ReviewStudyEvidence(test.evidence))
		})
	}
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
	assert.True(t, StudyCountsToday(word, now), "but its review stays on the finish line")
	next, ok := NextStudyReview([]StudyProgress{word, {Memory: StudyMemory{State: StudyStateNew, Due: now}}}, now)
	assert.False(t, ok, "an introduction waits behind it: %v", next)
}
