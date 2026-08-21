package analysis

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

func TestAggregateLemmasIsIdempotent(t *testing.T) {
	result := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{{CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Sing"}}, {CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Sing"}}}}}}
	lemmas := aggregateLemmas("sha256:x", result)
	if len(lemmas) != 1 || lemmas[0].Frequency != 2 || lemmas[0].ContentHash != "sha256:x" {
		t.Fatalf("lemmas = %+v", lemmas)
	}
}
