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
	Surface        string
	RawLemma       string
	CanonicalLemma string
	Headword       string
	Discriminator  string
	SearchKey      string
	UPOS           string
	Start, End     uint64
}

type spanBaselineSentence struct {
	Text       string
	Start, End uint64
	Citations  []string
	Tokens     []spanBaselineToken
}

type spanBaselineUnit struct {
	FullText  string
	Sentences []spanBaselineSentence
}

// cutWholeFixture cuts every sentence of one testdata file as a single unit.
func cutWholeFixture(t *testing.T, path string) spanBaselineUnit {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	definition := definitionForBytes(data)
	definition.CitationStart = "0.0.0"
	definition.CitationEnd = "999.999.999"
	work, err := ParseOgaSource(data)
	require.NoError(t, err)
	unit, err := work.CutUnit(definition)
	require.NoError(t, err)
	out := spanBaselineUnit{FullText: unit.FullText}
	for _, sentence := range unit.Result.Sentences {
		built := spanBaselineSentence{Text: sentence.Text, Start: sentence.Location.StartOffset, End: sentence.Location.EndOffset, Citations: sentence.Citations}
		for _, token := range sentence.Tokens {
			built.Tokens = append(built.Tokens, spanBaselineToken{
				Surface: token.Surface, RawLemma: token.RawLemma, CanonicalLemma: token.CanonicalLemma,
				Headword: token.Headword, Discriminator: token.LexicalDiscriminator, SearchKey: token.SearchKey,
				UPOS: token.UPOS, Start: token.Location.StartOffset, End: token.Location.EndOffset,
			})
		}
		out.Sentences = append(out.Sentences, built)
	}
	return out
}

// TestInterpretationKeepsSpansAndLemmasOfEveryFixture is the A2 table test. The
// baseline was captured from the interpretation before native AGDT syntax
// (oga-agdt-upos/5); a new interpretation version must leave every printed
// ordinal, offset, surface and lemma exactly as it was. Set
// MOUSEION_UPDATE_SPAN_BASELINE=1 only to recapture it deliberately.
func TestInterpretationKeepsSpansAndLemmasOfEveryFixture(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "oga-src-*.conllu"))
	require.NoError(t, err)
	baselinePath := filepath.Join("testdata", "span-baseline.json")
	current := map[string]spanBaselineUnit{}
	for _, path := range paths {
		if filepath.Base(path) == "oga-src-syntax.conllu" {
			continue // added with native syntax; it has no earlier version
		}
		current[filepath.Base(path)] = cutWholeFixture(t, path)
	}
	if os.Getenv("MOUSEION_UPDATE_SPAN_BASELINE") == "1" {
		encoded, err := json.MarshalIndent(current, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(baselinePath, append(encoded, '\n'), 0o600))
	}
	raw, err := os.ReadFile(baselinePath)
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
