package cardexport_test

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBestSentenceEvidenceRejectsMalformedCandidates pins the decode failures
// that make a candidate unrankable through the exported seam.
func TestBestSentenceEvidenceRejectsMalformedCandidates(t *testing.T) {
	for _, test := range []struct {
		name      string
		candidate domain.SelectionCandidate
	}{
		{
			name:      "malformed sentence references",
			candidate: domain.SelectionCandidate{ObservedForms: []byte(`["Haus"]`), SentenceReferences: []byte(`{`)},
		},
		{
			name:      "malformed observed forms",
			candidate: domain.SelectionCandidate{ObservedForms: []byte(`{`), SentenceReferences: []byte(`[{"text":"Das Haus steht dort.","location":{"start_offset":1}}]`)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			evidence, ok := cardexport.BestSentenceEvidence(test.candidate)
			assert.False(t, ok)
			assert.Equal(t, cardexport.SentenceEvidence{}, evidence)
		})
	}
}

// TestBestSentenceEvidenceFromCorpusSkipsUnresolvedReferences pins that a
// source reference absent from the persisted corpus is skipped, and that
// skipping every reference reports no evidence.
func TestBestSentenceEvidenceFromCorpusSkipsUnresolvedReferences(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CorpusID:       "corpus-1",
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":7,"text":"stale","location":{"start_offset":70}},
			{"sentence_index":2,"text":"stale","location":{"start_offset":20}}
		]`),
	}
	sentences := map[int64]analyzer.Sentence{2: {Text: "Das alte Haus steht heute ganz ruhig dort."}}

	evidence, ok := cardexport.BestSentenceEvidenceFromCorpus(candidate, sentences)
	require.True(t, ok)
	assert.Equal(t, int64(2), evidence.SentenceOrdinal)
	assert.Equal(t, "Das alte Haus steht heute ganz ruhig dort.", evidence.Sentence)

	none, ok := cardexport.BestSentenceEvidenceFromCorpus(candidate, map[int64]analyzer.Sentence{})
	assert.False(t, ok)
	assert.Equal(t, cardexport.SentenceEvidence{}, none)
}

// TestBestSentenceEvidenceDefaultsMissingLocation pins that an unreadable
// source location is captured as the invalid offset the quality gate rejects.
func TestBestSentenceEvidenceDefaultsMissingLocation(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma:     "haus",
		ObservedForms:      []byte(`["Haus"]`),
		SentenceReferences: []byte(`[{"sentence_index":0,"text":"Das Haus steht dort.","location":{}}]`),
	}

	evidence, ok := cardexport.BestSentenceEvidence(candidate)
	require.True(t, ok)
	assert.Equal(t, int64(-1), evidence.FirstEncounter)
	assert.Contains(t, evidence.Quality.Reasons, "invalid source location")
}

// TestBestSentenceEvidenceBreaksTiesBySentenceOrdinal pins the ordinal
// tie-break that wins before sentence text.
func TestBestSentenceEvidenceBreaksTiesBySentenceOrdinal(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":2,"text":"Das Haus steht heute am alten See.","location":{"start_offset":20}},
			{"sentence_index":1,"text":"Zum Haus gehen heute viele Kinder.","location":{"start_offset":20}}
		]`),
	}

	evidence, ok := cardexport.BestSentenceEvidence(candidate)
	require.True(t, ok)
	assert.Equal(t, int64(1), evidence.SentenceOrdinal)
	assert.Equal(t, "Zum Haus gehen heute viele Kinder.", evidence.Sentence)
}

// TestBestSentenceEvidenceBreaksTiesBySentenceText pins the final tie-break
// when two references share an ordinal and a first encounter.
func TestBestSentenceEvidenceBreaksTiesBySentenceText(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":1,"text":"Zum Haus gehen heute viele Kinder.","location":{"start_offset":20}},
			{"sentence_index":1,"text":"Das Haus steht heute am alten See.","location":{"start_offset":20}}
		]`),
	}

	evidence, ok := cardexport.BestSentenceEvidence(candidate)
	require.True(t, ok)
	assert.Equal(t, "Das Haus steht heute am alten See.", evidence.Sentence)
}
