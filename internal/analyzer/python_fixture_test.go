package analyzer_test

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestGoConsumesPythonProducedCorpus(t *testing.T) {
	encoded, err := os.ReadFile("testdata/stanza_python_corpus.pb.b64")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatalf("decode Python fixture: %v", err)
	}
	corpus := new(mouseionv1.NormalizedCorpus)
	if err := proto.Unmarshal(payload, corpus); err != nil {
		t.Fatalf("unmarshal Python fixture: %v", err)
	}
	if corpus.GetSchemaVersion() != "1.0.0" || corpus.GetLanguage() != "de" {
		t.Fatalf("unexpected corpus header: %v", corpus)
	}
	token := corpus.GetSentences()[0].GetTokens()[0]
	if token.GetSurface() != "Goethe" || token.GetNamedEntity() != "B-PER" {
		t.Fatalf("unexpected Python-produced token: %v", token)
	}
}

func TestGoConsumesItalianStanzaRegressionFixture(t *testing.T) {
	payload, err := os.ReadFile("../../nlp/tests/testdata/italian_stanza_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	corpus := new(mouseionv1.NormalizedCorpus)
	if err = protojson.Unmarshal(payload, corpus); err != nil {
		t.Fatalf("unmarshal Italian Python fixture: %v", err)
	}
	if corpus.GetLanguage() != "it" || len(corpus.GetSentences()) != 2 {
		t.Fatalf("unexpected Italian corpus header: %v", corpus)
	}
	tokens := corpus.GetSentences()[0].GetTokens()
	if tokens[0].GetSurface() != "L'" || tokens[0].GetRawLemma() != "il" || tokens[4].GetCanonicalLemma() != "ragazza" || tokens[4].GetMorphology()["Number"] != "Plur" {
		t.Fatalf("unexpected article/gender/number normalization: %v", tokens)
	}
	if tokens[6].GetSurface() != "dell'" || tokens[6].GetRawLemma() != "di" || tokens[7].GetSurface() != "dell'" || tokens[7].GetRawLemma() != "il" {
		t.Fatalf("unexpected contraction expansion: %v", tokens[6:8])
	}
	second := corpus.GetSentences()[1].GetTokens()
	if second[0].GetNamedEntity() != "S-PER" || second[5].GetSurface() != "porterà" || second[5].GetRawLemma() != "portare" || second[9].GetMorphology()["Clitic"] != "Yes" {
		t.Fatalf("unexpected name/accent/clitic normalization: %v", second)
	}
}
