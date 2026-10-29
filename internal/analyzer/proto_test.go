package analyzer_test

import (
	"testing"
	"time"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestProtoRoundTrip(t *testing.T) {
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

func TestFromProtoTreatsRecoveredAdvmodParticleAsCompound(t *testing.T) {
	corpus, err := analyzer.FromProto(&mouseionv1.NormalizedCorpus{
		SchemaVersion: "1.1.0",
		Language:      "de",
		Sentences: []*mouseionv1.Sentence{{
			Text: "Großmutter sieht fern.",
			Tokens: []*mouseionv1.Token{
				{Surface: "Großmutter", CanonicalLemma: "großmutter", Pos: "NOUN", Dependency: "nsubj", Head: 1},
				{Surface: "sieht", CanonicalLemma: "fernsehen", Pos: "VERB", Dependency: "root", Head: 1},
				{Surface: "fern", CanonicalLemma: "fern", Pos: "ADV", Dependency: "advmod", Head: 1},
			},
		}},
	})
	require.NoError(t, err)

	tokens := corpus.Sentences[0].Tokens
	assert.Equal(t, "compound:prt", tokens[2].Dependency, "a recovered particle is a verb component for every consumer")
}

func TestFromProtoLeavesTrueAdverbDependencyAlone(t *testing.T) {
	corpus, err := analyzer.FromProto(&mouseionv1.NormalizedCorpus{
		SchemaVersion: "1.1.0",
		Language:      "de",
		Sentences: []*mouseionv1.Sentence{{
			Text: "Das Dorf liegt fern der Stadt.",
			Tokens: []*mouseionv1.Token{
				{Surface: "Das", CanonicalLemma: "der", Pos: "DET", Dependency: "det", Head: 1},
				{Surface: "Dorf", CanonicalLemma: "dorf", Pos: "NOUN", Dependency: "nsubj", Head: 2},
				{Surface: "liegt", CanonicalLemma: "liegen", Pos: "VERB", Dependency: "root", Head: 2},
				{Surface: "fern", CanonicalLemma: "fern", Pos: "ADV", Dependency: "advmod", Head: 2},
				{Surface: "der", CanonicalLemma: "der", Pos: "DET", Dependency: "det", Head: 5},
				{Surface: "Stadt", CanonicalLemma: "stadt", Pos: "NOUN", Dependency: "nmod", Head: 3},
			},
		}},
	})
	require.NoError(t, err)

	tokens := corpus.Sentences[0].Tokens
	assert.Equal(t, "advmod", tokens[3].Dependency, "a true adverb keeps its analyzer relation")
}

func TestFromProtoDoesNotNormalizeAdvmodOutsideGerman(t *testing.T) {
	corpus, err := analyzer.FromProto(&mouseionv1.NormalizedCorpus{
		SchemaVersion: "1.1.0",
		Language:      "fr",
		Sentences: []*mouseionv1.Sentence{{
			Text: "Elle voit fern.",
			Tokens: []*mouseionv1.Token{
				{Surface: "Elle", CanonicalLemma: "elle", Pos: "PRON", Dependency: "nsubj", Head: 1},
				{Surface: "voit", CanonicalLemma: "fernsehen", Pos: "VERB", Dependency: "root", Head: 1},
				{Surface: "fern", CanonicalLemma: "fern", Pos: "ADV", Dependency: "advmod", Head: 1},
			},
		}},
	})
	require.NoError(t, err)

	tokens := corpus.Sentences[0].Tokens
	assert.Equal(t, "advmod", tokens[2].Dependency, "only German recovery is normalized")
}

func TestFromProtoRejectsInvalidAnalysisTime(t *testing.T) {
	message := analyzer.ToProto(analyzer.Result{})
	message.Analysis.AnalyzedAt = "not-a-time"

	_, err := analyzer.FromProto(message)
	assert.Error(t, err, "FromProto() error = nil, want invalid analysis time error")
}
