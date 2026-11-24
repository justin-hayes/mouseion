package ancientgreek

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spanBaselineToken is the part of a printed token the interpretation versions
// must agree on: surface, lemma identity, UPOS and source offsets (A2).
type spanBaselineToken struct {
	Ordinal        int    `json:"ordinal"`
	Surface        string `json:"surface"`
	RawLemma       string `json:"rawLemma"`
	CanonicalLemma string `json:"canonicalLemma"`
	Headword       string `json:"headword"`
	Discriminator  string `json:"discriminator"`
	SearchKey      string `json:"searchKey"`
	UPOS           string `json:"upos"`
	Start          uint64 `json:"start"`
	End            uint64 `json:"end"`
}

type spanBaselineSentence struct {
	Ordinal   int                 `json:"ordinal"`
	Text      string              `json:"text"`
	Start     uint64              `json:"start"`
	End       uint64              `json:"end"`
	Citations []string            `json:"citations"`
	Tokens    []spanBaselineToken `json:"tokens"`
}

type spanBaselineUnit struct {
	FullText  string                 `json:"fullText"`
	Sentences []spanBaselineSentence `json:"sentences"`
}

// cutWholeFixture cuts every sentence of one testdata file as a single unit.
func cutWholeFixture(t *testing.T, path string) spanBaselineUnit {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	definition := definitionForBytes(data)
	definition.CitationStart = "0.0.0"
	definition.CitationEnd = "999.999.999"
	work, err := ParseOgaSource(data)
	require.NoError(t, err)
	unit, err := work.CutUnit(definition)
	require.NoError(t, err)
	out := spanBaselineUnit{FullText: unit.FullText}
	for sentenceOrdinal, sentence := range unit.Result.Sentences {
		built := spanBaselineSentence{Ordinal: sentenceOrdinal, Text: sentence.Text, Start: sentence.Location.StartOffset, End: sentence.Location.EndOffset, Citations: sentence.Citations}
		for tokenOrdinal, token := range sentence.Tokens {
			built.Tokens = append(built.Tokens, spanBaselineToken{
				Ordinal: tokenOrdinal, Surface: token.Surface, RawLemma: token.RawLemma, CanonicalLemma: token.CanonicalLemma,
				Headword: token.Headword, Discriminator: token.LexicalDiscriminator, SearchKey: token.SearchKey,
				UPOS: token.UPOS, Start: token.Location.StartOffset, End: token.Location.EndOffset,
			})
		}
		out.Sentences = append(out.Sentences, built)
	}
	return out
}

// TestInterpretationKeepsSpansAndLemmasOfEveryFixture is the A2 table test. The
// baseline is the output of the interpretation before native AGDT syntax
// (oga-agdt-upos/5), captured by running this same test file against commit
// 3391cc7b with MOUSEION_UPDATE_SPAN_BASELINE=1; sentence and token ordinals,
// offsets, surfaces, raw and canonical lemmas are compared. A new interpretation version must leave every printed
// ordinal, offset, surface and lemma exactly as it was. Set
// MOUSEION_UPDATE_SPAN_BASELINE=1 only to recapture it deliberately.
func TestInterpretationKeepsSpansAndLemmasOfEveryFixture(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "oga-src-*.conllu"))
	require.NoError(t, err)
	baselinePath := filepath.Join("testdata", "span-baseline.json")
	current := map[string]spanBaselineUnit{}
	for _, path := range paths {
		current[filepath.Base(path)] = cutWholeFixture(t, path)
	}
	if os.Getenv("MOUSEION_UPDATE_SPAN_BASELINE") == "1" {
		encoded, err := json.MarshalIndent(current, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(baselinePath, append(encoded, '\n'), 0o600))
	}
	raw, err := os.ReadFile(filepath.Clean(baselinePath))
	require.NoError(t, err)
	var baseline map[string]spanBaselineUnit
	require.NoError(t, json.Unmarshal(raw, &baseline))
	require.NotEmpty(t, baseline)
	for name, want := range baseline {
		t.Run(name, func(t *testing.T) {
			got, ok := current[name]
			require.True(t, ok, "fixture is missing")
			assert.Equal(t, want, got)
		})
	}
	assert.Len(t, current, len(baseline), "a fixture without a baseline")
}
