package analysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/require"
)

func TestConversionInterpretationVersionUsesThePublishedRun(t *testing.T) {
	candidate := func(runID, analyzerVersion, readingUnitVersion string) classicalConversionCandidate {
		material := persistence.LegacyClassicalSourceMaterial{
			AnalysisRunID: runID, AnalyzerVersion: analyzerVersion, InterpretationVersion: readingUnitVersion,
		}
		return classicalConversionCandidate{LegacyClassicalSourceMaterial: material}
	}
	cases := []struct {
		name      string
		candidate classicalConversionCandidate
		want      string
	}{
		{
			name:      "a published run wins over the reconciled Reading unit",
			candidate: candidate("run", "oga-agdt-upos/4+oga-agdt-feats/1+ancient-greek/1", "oga-agdt-upos/5"),
			want:      "oga-agdt-upos/4",
		},
		{
			name:      "a source without a published run keeps the Reading unit version",
			candidate: candidate("", "", "oga-agdt-upos/5"),
			want:      "oga-agdt-upos/5",
		},
		{
			name:      "an absent recorded version falls back to the Reading unit version",
			candidate: candidate("run", "", "oga-agdt-upos/5"),
			want:      "oga-agdt-upos/5",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, conversionInterpretationVersion(tt.candidate))
		})
	}
}

func conversionEvidenceResult() analyzer.Result {
	return analyzer.Result{Sentences: []analyzer.Sentence{{
		Text:      "λέγει Κῦρος",
		Citations: []string{"1.1.1"},
		Tokens: []analyzer.Token{
			{Surface: "λέγει", RawLemma: "λέγω", CanonicalLemma: "λέγω", UPOS: "VERB", Headword: "λέγω", LexicalDiscriminator: "morpheus:1"},
			{Surface: "Κῦρος", RawLemma: "κῦρος", CanonicalLemma: "κῦροσ", UPOS: "PROPN", Headword: "κῦρος"},
		},
	}}}
}

func conversionEvidenceStored() ([]conversionSentence, map[int64][]conversionToken) {
	return []conversionSentence{{text: "λέγει Κῦρος", citations: []string{"1.1.1"}}}, map[int64][]conversionToken{
		0: {
			{ordinal: 0, surface: "λέγει", rawLemma: "λέγω", canonicalLemma: "λέγω", upos: "VERB", headword: "λέγω", lexicalDiscriminator: "morpheus:1"},
			{ordinal: 1, surface: "Κῦρος", rawLemma: "κῦρος", canonicalLemma: "κῦροσ", upos: "PROPN", headword: "κῦρος"},
		},
	}
}

func TestPublishedEvidenceMatchesEveryComparedFact(t *testing.T) {
	t.Run("matching evidence", func(t *testing.T) {
		result := conversionEvidenceResult()
		sentences, tokens := conversionEvidenceStored()
		require.True(t, publishedEvidenceMatches(result, sentences, tokens))
	})

	tests := []struct {
		name   string
		mutate func(*analyzer.Result, *[]conversionSentence, map[int64][]conversionToken)
	}{
		{"sentence text", func(_ *analyzer.Result, s *[]conversionSentence, _ map[int64][]conversionToken) {
			(*s)[0].text = "other"
		}},
		{"citations", func(_ *analyzer.Result, s *[]conversionSentence, _ map[int64][]conversionToken) {
			(*s)[0].citations = []string{"1.1.2"}
		}},
		{"sentence count", func(_ *analyzer.Result, s *[]conversionSentence, _ map[int64][]conversionToken) {
			*s = append(*s, conversionSentence{})
		}},
		{"token count", func(_ *analyzer.Result, _ *[]conversionSentence, tokens map[int64][]conversionToken) {
			tokens[0] = tokens[0][:1]
		}},
		{"token ordinal", func(_ *analyzer.Result, _ *[]conversionSentence, tokens map[int64][]conversionToken) {
			tokens[0][1].ordinal = 3
		}},
		{"surface", func(_ *analyzer.Result, _ *[]conversionSentence, tokens map[int64][]conversionToken) {
			tokens[0][0].surface = "other"
		}},
		{"raw lemma", func(_ *analyzer.Result, _ *[]conversionSentence, tokens map[int64][]conversionToken) {
			tokens[0][0].rawLemma = "other"
		}},
		{"canonical lemma", func(_ *analyzer.Result, _ *[]conversionSentence, tokens map[int64][]conversionToken) {
			tokens[0][0].canonicalLemma = "other"
		}},
		{"upos", func(_ *analyzer.Result, _ *[]conversionSentence, tokens map[int64][]conversionToken) {
			tokens[0][0].upos = "NOUN"
		}},
		{"headword", func(_ *analyzer.Result, _ *[]conversionSentence, tokens map[int64][]conversionToken) {
			tokens[0][0].headword = "other"
		}},
		{"discriminator", func(_ *analyzer.Result, _ *[]conversionSentence, tokens map[int64][]conversionToken) {
			tokens[0][0].lexicalDiscriminator = "morpheus:2"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := conversionEvidenceResult()
			sentences, tokens := conversionEvidenceStored()
			tt.mutate(&result, &sentences, tokens)
			require.False(t, publishedEvidenceMatches(result, sentences, tokens))
		})
	}
}

func TestConvertLegacyClassicalBooksRequiresAStore(t *testing.T) {
	_, err := ConvertLegacyClassicalBooks(context.Background(), nil)
	require.ErrorContains(t, err, "store is required")
}

func TestConvertLegacyClassicalCandidateRefusesUnusableSources(t *testing.T) {
	t.Run("not the whole work file", func(t *testing.T) {
		converted, outcome := convertLegacyClassicalCandidate(context.Background(), nil, classicalConversionCandidate{
			content: []byte("unit text"), SourceFileSize: 100,
		})
		require.False(t, converted)
		require.Equal(t, conversionSkipped, outcome)
	})

	t.Run("unknown source format", func(t *testing.T) {
		content := []byte("source")
		converted, outcome := convertLegacyClassicalCandidate(context.Background(), nil, classicalConversionCandidate{
			content: content, SourceFileSize: int64(len(content)), SourceFileSHA256: sha256Hex(content),
			SourceFormat: "other-format",
		})
		require.False(t, converted)
		require.Equal(t, conversionSkipped, outcome)
	})

	t.Run("malformed source", func(t *testing.T) {
		content := []byte("not a conllu document")
		converted, outcome := convertLegacyClassicalCandidate(context.Background(), nil, classicalConversionCandidate{
			content: content, SourceFileSize: int64(len(content)), SourceFileSHA256: sha256Hex(content),
			SourceFormat: "oga-conllu",
		})
		require.False(t, converted)
		require.Equal(t, conversionSkipped, outcome)
	})

	t.Run("refused interpretation", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
		require.NoError(t, err)
		converted, outcome := convertLegacyClassicalCandidate(context.Background(), nil, classicalConversionCandidate{
			content: content, SourceFileSize: int64(len(content)), SourceFileSHA256: sha256Hex(content),
			SourceFormat: "oga-conllu",
		})
		require.False(t, converted)
		require.Equal(t, conversionSkipped, outcome)
	})
}
