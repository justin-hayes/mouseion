package analyzer_test

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
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
