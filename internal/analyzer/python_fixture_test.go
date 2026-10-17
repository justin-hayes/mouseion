package analyzer_test

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestGoConsumesPythonProducedCorpus(t *testing.T) {
	encoded, err := os.ReadFile("testdata/stanza_python_corpus.pb.b64")
	require.NoError(t, err)
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	require.NoError(t, err, "decode Python fixture")
	corpus := new(mouseionv1.NormalizedCorpus)
	require.NoError(t, proto.Unmarshal(payload, corpus), "unmarshal Python fixture")
	assert.Equal(t, "1.0.0", corpus.GetSchemaVersion())
	assert.Equal(t, "de", corpus.GetLanguage())
	token := corpus.GetSentences()[0].GetTokens()[0]
	assert.Equal(t, "Goethe", token.GetSurface())
}

func TestGoConsumesItalianStanzaRegressionFixture(t *testing.T) {
	payload, err := os.ReadFile("../../nlp/tests/testdata/italian_stanza_expected.json")
	require.NoError(t, err)
	corpus := new(mouseionv1.NormalizedCorpus)
	require.NoError(t, protojson.Unmarshal(payload, corpus), "unmarshal Italian Python fixture")
	assert.Equal(t, "it", corpus.GetLanguage())
	require.Len(t, corpus.GetSentences(), 2)
	tokens := corpus.GetSentences()[0].GetTokens()
	assert.Equal(t, "L'", tokens[0].GetSurface())
	assert.Equal(t, "il", tokens[0].GetRawLemma())
	assert.Equal(t, "ragazza", tokens[4].GetCanonicalLemma())
	assert.Equal(t, "Plur", tokens[4].GetMorphology()["Number"])
	assert.Equal(t, "dell'", tokens[6].GetSurface())
	assert.Equal(t, "di", tokens[6].GetRawLemma())
	assert.Equal(t, "dell'", tokens[7].GetSurface())
	assert.Equal(t, "il", tokens[7].GetRawLemma())
	second := corpus.GetSentences()[1].GetTokens()
	assert.Equal(t, "porterà", second[5].GetSurface())
	assert.Equal(t, "portare", second[5].GetRawLemma())
	assert.Equal(t, "Yes", second[9].GetMorphology()["Clitic"])
}

func TestGoConsumesGreekStanzaRegressionFixture(t *testing.T) {
	payload, err := os.ReadFile("../../nlp/tests/testdata/greek_stanza_expected.json")
	require.NoError(t, err)
	corpus := new(mouseionv1.NormalizedCorpus)
	require.NoError(t, protojson.Unmarshal(payload, corpus), "unmarshal Greek Python fixture")
	assert.Equal(t, "el", corpus.GetLanguage())
	require.Len(t, corpus.GetSentences(), 2)
	first := corpus.GetSentences()[0].GetTokens()
	assert.Equal(t, "σε", first[0].GetSurface())
	assert.Equal(t, "ADP", first[0].GetPos())
	assert.Equal(t, "το", first[1].GetSurface())
	assert.Equal(t, "σπίτι", first[2].GetCanonicalLemma())
	assert.Equal(t, "Neut", first[2].GetMorphology()["Gender"])
	assert.Equal(t, uint32(5), first[5].GetHead())
	second := corpus.GetSentences()[1].GetTokens()
	assert.Equal(t, "σε", second[3].GetSurface())
	assert.Equal(t, "σε", second[3].GetRawLemma())
	assert.Equal(t, "την", second[4].GetSurface())
	assert.Equal(t, "κόσμοσ", second[9].GetCanonicalLemma())
	assert.Equal(t, "γελάω", second[10].GetRawLemma())
}

func TestGoConsumesFrenchStanzaRegressionFixture(t *testing.T) {
	payload, err := os.ReadFile("../../nlp/tests/testdata/french_stanza_expected.json")
	require.NoError(t, err)
	corpus := new(mouseionv1.NormalizedCorpus)
	require.NoError(t, protojson.Unmarshal(payload, corpus), "unmarshal French Python fixture")
	require.Equal(t, "fr", corpus.GetLanguage())
	require.Equal(t, "french", corpus.GetNormalizationProfile().GetName(), "the producer records the French profile")
	require.Equal(t, "1", corpus.GetNormalizationProfile().GetVersion())
	require.Len(t, corpus.GetSentences(), 6)
	sentences := corpus.GetSentences()

	t.Run("plain prose keeps its elision and contraction", func(t *testing.T) {
		first := sentences[0].GetTokens()
		require.Len(t, first, 9)
		require.Equal(t, "l'", first[2].GetSurface())
		require.Equal(t, "le", first[2].GetRawLemma())
		require.Equal(t, "de", first[5].GetSurface())
		require.Equal(t, "ADP", first[5].GetPos())
		require.Equal(t, "le", first[6].GetSurface())
		require.Equal(t, "DET", first[6].GetPos())
		require.Equal(t, "marché", first[7].GetCanonicalLemma())
		require.Equal(t, "obl:arg", first[7].GetDependency())
		second := sentences[1].GetTokens()
		require.Len(t, second, 9)
		require.Equal(t, "à", second[2].GetSurface())
		require.Equal(t, "ADP", second[2].GetPos())
		require.Equal(t, "sœur", second[7].GetCanonicalLemma())
		require.Equal(t, "Fem", second[7].GetMorphology()["Gender"])
	})

	t.Run("a closing guillemet stays whole", func(t *testing.T) {
		guillemet := sentences[2]
		require.Equal(t, "« Je suis sûr qu’il viendra ce soir.\u202f»", guillemet.GetText())
		require.Equal(t, uint64(62), guillemet.GetLocation().GetStartOffset())
		require.Equal(t, uint64(100), guillemet.GetLocation().GetEndOffset())
		tokens := guillemet.GetTokens()
		require.Len(t, tokens, 11)
		require.Equal(t, "»", tokens[10].GetSurface())
		require.Equal(t, uint64(99), tokens[10].GetLocation().GetStartOffset())
		require.Equal(t, uint64(100), tokens[10].GetLocation().GetEndOffset())
	})

	t.Run("text after a close is its own sentence", func(t *testing.T) {
		after := sentences[3]
		require.Equal(t, "Il faut partir.", after.GetText())
		require.Equal(t, uint64(102), after.GetLocation().GetStartOffset())
		require.Equal(t, uint64(117), after.GetLocation().GetEndOffset())
	})

	t.Run("a guillemet incise stays whole", func(t *testing.T) {
		incise := sentences[4]
		require.Equal(t, "« Tu viens ? » demanda-t-il.", incise.GetText())
		tokens := incise.GetTokens()
		require.Len(t, tokens, 8)
		require.Equal(t, "demanda", tokens[5].GetSurface())
		require.Equal(t, "parataxis:insert", tokens[5].GetDependency())
		require.Equal(t, "t-il", tokens[6].GetSurface())
		require.Equal(t, "lui", tokens[6].GetRawLemma())
	})

	t.Run("an em-dash incise stays whole", func(t *testing.T) {
		emDash := sentences[5]
		require.Equal(t, "— Tu viens ? demanda-t-il.", emDash.GetText())
		require.Equal(t, uint64(147), emDash.GetLocation().GetStartOffset())
		require.Equal(t, uint64(173), emDash.GetLocation().GetEndOffset())
		require.Equal(t, "—", emDash.GetTokens()[0].GetSurface())
	})
}
