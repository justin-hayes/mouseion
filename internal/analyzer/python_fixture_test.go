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
