package ancientgreek

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cutSyntaxFixture(t *testing.T) PreparedUnit {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-syntax.conllu"))
	require.NoError(t, err)
	definition := definitionForBytes(data)
	definition.CitationStart = "1.1.1"
	definition.CitationEnd = "1.1.3"
	work, err := ParseOgaSource(data)
	require.NoError(t, err)
	unit, err := work.CutUnit(definition)
	require.NoError(t, err)
	return unit
}

func TestCutUnitStoresNativeAGDTSyntax(t *testing.T) {
	unit := cutSyntaxFixture(t)

	t.Run("run identity", func(t *testing.T) {
		assert.Equal(t, SyntaxStateNativeAGDT, unit.SyntaxState)
		assert.Contains(t, unit.ConfigIdentity, string(SyntaxStateNativeAGDT))
		assert.NotContains(t, unit.ConfigIdentity, string(SyntaxStateAbsent))
		assert.Equal(t, FeatsMappingVersion, "oga-agdt-feats/2")
		assert.Equal(t, "oga-agdt-syntax/1", InterpretationVersionNativeAGDT)
	})

	require.Len(t, unit.Result.Sentences, 3)

	t.Run("a token headed by an unprinted node is self-headed and names the node", func(t *testing.T) {
		sentence := unit.Result.Sentences[0]
		require.Len(t, sentence.Tokens, 3)
		subject := sentence.Tokens[0]
		assert.Equal(t, "SBJ", subject.Dependency)
		assert.Equal(t, uint32(0), subject.Head)
		assert.Equal(t, "2", subject.HeadUnprintedNode)
		dependentOfNode := sentence.Tokens[1]
		assert.Equal(t, "ATR", dependentOfNode.Dependency)
		assert.Equal(t, uint32(1), dependentOfNode.Head)
		assert.Equal(t, "3", dependentOfNode.HeadUnprintedNode)
		assert.Equal(t, "AuxK", sentence.Tokens[2].Dependency)
		assert.Equal(t, uint32(2), sentence.Tokens[2].Head)
		assert.Equal(t, "2", sentence.Tokens[2].HeadUnprintedNode)
	})

	t.Run("unprinted nodes keep relation and head", func(t *testing.T) {
		assert.Equal(t, []analyzer.UnprintedSyntaxNode{
			{SourceID: "2", Dependency: "PRED"},
			{SourceID: "3", Dependency: "OBJ", HeadUnprintedNode: "2"},
		}, unit.Result.Sentences[0].UnprintedNodes)
	})

	t.Run("a comma root is self-headed and its dependent points at it", func(t *testing.T) {
		sentence := unit.Result.Sentences[1]
		require.Len(t, sentence.Tokens, 2)
		assert.Equal(t, "AuxK", sentence.Tokens[0].Dependency)
		assert.Equal(t, uint32(0), sentence.Tokens[0].Head)
		assert.Empty(t, sentence.Tokens[0].HeadUnprintedNode)
		assert.Equal(t, "PRED", sentence.Tokens[1].Dependency)
		assert.Equal(t, uint32(0), sentence.Tokens[1].Head)
		assert.Empty(t, sentence.Tokens[1].HeadUnprintedNode)
		assert.Empty(t, sentence.UnprintedNodes)
	})

	t.Run("a printed head resolves to its printed ordinal and the Zeta defect is repaired", func(t *testing.T) {
		sentence := unit.Result.Sentences[2]
		require.Len(t, sentence.Tokens, 7)
		assert.Equal(t, "PRED", sentence.Tokens[0].Dependency)
		assert.Equal(t, uint32(0), sentence.Tokens[0].Head, "an AGDT root is self-headed")
		assert.Equal(t, uint32(0), sentence.Tokens[3].Head)
		assert.Equal(t, "AuxZ", sentence.Tokens[6].Dependency)
		assert.Equal(t, []byte("AuxZ"), []byte(sentence.Tokens[6].Dependency))
	})
}

func TestMapFeaturesAddsVerbFormAspectPerfectAndCommonGender(t *testing.T) {
	cases := []struct {
		name     string
		postag   string
		want     map[string]string
		unmapped []FeatureValueCount
	}{
		{"perfect", "v3sria---", map[string]string{"Person": "3", "Number": "Sing", "Tense": "Past", "Aspect": "Perf", "Mood": "Ind", "VerbForm": "Fin", "Voice": "Act"}, nil},
		{"future perfect", "v3stia---", map[string]string{"Person": "3", "Number": "Sing", "Tense": "Fut", "Aspect": "Perf", "Mood": "Ind", "VerbForm": "Fin", "Voice": "Act"}, nil},
		{"imperfect", "v3siia---", map[string]string{"Person": "3", "Number": "Sing", "Tense": "Imp", "Aspect": "Imp", "Mood": "Ind", "VerbForm": "Fin", "Voice": "Act"}, nil},
		{"pluperfect has no aspect", "v3slia---", map[string]string{"Person": "3", "Number": "Sing", "Tense": "Pqp", "Mood": "Ind", "VerbForm": "Fin", "Voice": "Act"}, nil},
		{"aorist stays past", "v3saia---", map[string]string{"Person": "3", "Number": "Sing", "Tense": "Past", "Mood": "Ind", "VerbForm": "Fin", "Voice": "Act"}, nil},
		{"infinitive has no mood", "v--pna---", map[string]string{"Tense": "Pres", "VerbForm": "Inf", "Voice": "Act"}, nil},
		{"participle has no mood", "v-sppamn-", map[string]string{"Number": "Sing", "Tense": "Pres", "VerbForm": "Part", "Voice": "Act", "Gender": "Masc", "Case": "Nom"}, nil},
		{"subjunctive and imperative are finite", "v3spsa---", map[string]string{"Person": "3", "Number": "Sing", "Tense": "Pres", "Mood": "Sub", "VerbForm": "Fin", "Voice": "Act"}, nil},
		{"common gender", "n-s---cn-", map[string]string{"Number": "Sing", "Gender": "Fem,Masc", "Case": "Nom"}, nil},
		{"unknown tense is reported", "v3sxia---", map[string]string{"Person": "3", "Number": "Sing", "Mood": "Ind", "VerbForm": "Fin", "Voice": "Act"}, []FeatureValueCount{{Feature: "Tense", Value: "x"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, unmapped := mapFeatures(tc.postag)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.unmapped, unmapped)
		})
	}
}
