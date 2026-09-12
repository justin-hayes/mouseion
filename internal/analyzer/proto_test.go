package analyzer_test

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestProtoRoundTrip(t *testing.T) {
	namedEntity := "PERSON"
	want := analyzer.Result{
		SchemaVersion: "1.1.0",
		Language:      "de",
		Sentences: []analyzer.Sentence{{
			Text: "Er grüßte.",
			Tokens: []analyzer.Token{
				{
					Surface:        "Er",
					RawLemma:       "er",
					CanonicalLemma: "er",
					UPOS:           "PRON",
					Dependency:     "nsubj",
					Head:           1,
					Morphology:     map[string]string{"Case": "Nom"},
					Location:       analyzer.SourceLocation{SourceDocumentID: "doc-1", Chapter: "1", Section: "opening", StartOffset: 0, EndOffset: 2},
				},
				{
					Surface:        "grüßte",
					RawLemma:       "grüßen",
					CanonicalLemma: "grüßen",
					UPOS:           "VERB",
					Dependency:     "root",
					Head:           1,
					Morphology:     map[string]string{"Tense": "Past"},
					NamedEntity:    &namedEntity,
					Location:       analyzer.SourceLocation{SourceDocumentID: "doc-1", Chapter: "1", Section: "opening", StartOffset: 3, EndOffset: 9},
				},
			},
			Location: analyzer.SourceLocation{SourceDocumentID: "doc-1", Chapter: "1", Section: "opening", StartOffset: 0, EndOffset: 10},
		}},
		SourceDocuments: []analyzer.SourceDocumentMetadata{{ID: "doc-1", SourceIdentifier: "opds:book", Title: "Book"}},
		Analysis: analyzer.AnalysisProvenance{
			RunID:           "run-1",
			AnalyzedAt:      time.Date(2026, 8, 21, 12, 34, 56, 123, time.UTC),
			AnalyzerName:    "fake",
			AnalyzerVersion: "1.2.3",
		},
		NormalizationProfile: analyzer.NormalizationProfile{Name: "german-standard", Version: "1"},
	}

	message := analyzer.ToProto(want)
	require.Len(t, message.GetSentences(), 1)
	wireTokens := message.GetSentences()[0].GetTokens()
	require.Len(t, wireTokens, 2)
	assert.Equal(t, "nsubj", wireTokens[0].GetDependency())
	assert.Equal(t, uint32(1), wireTokens[0].GetHead())
	assert.Equal(t, "root", wireTokens[1].GetDependency())
	assert.Equal(t, uint32(1), wireTokens[1].GetHead())

	bytes, err := proto.Marshal(message)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes, "Marshal() returned empty transport payload")

	got, err := analyzer.FromProto(message)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	require.Len(t, got.Sentences[0].Tokens, 2)
	assert.Equal(t, "nsubj", got.Sentences[0].Tokens[0].Dependency)
	assert.Equal(t, uint32(1), got.Sentences[0].Tokens[0].Head)
}

func TestFromProtoRejectsInvalidAnalysisTime(t *testing.T) {
	message := analyzer.ToProto(analyzer.Result{})
	message.Analysis.AnalyzedAt = "not-a-time"

	_, err := analyzer.FromProto(message)
	assert.Error(t, err, "FromProto() error = nil, want invalid analysis time error")
}
