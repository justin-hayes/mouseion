package analysis

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

func TestAggregateLemmasIsIdempotent(t *testing.T) {
	result := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{{CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Sing"}}, {CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Sing"}}, {CanonicalLemma: "5", UPOS: "NOUN"}}}}}
	lemmas := aggregateLemmas("sha256:x", result)
	if len(lemmas) != 1 || lemmas[0].Frequency != 2 || lemmas[0].ContentHash != "sha256:x" {
		t.Fatalf("lemmas = %+v", lemmas)
	}
}

func TestOffsetResultLocations(t *testing.T) {
	result := analyzer.Result{Sentences: []analyzer.Sentence{{
		Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 8},
		Tokens:   []analyzer.Token{{Location: analyzer.SourceLocation{StartOffset: 1, EndOffset: 6}}},
	}}}

	offsetResultLocations(&result, 100_000)

	if got := result.Sentences[0].Location; got.StartOffset != 100_000 || got.EndOffset != 100_008 {
		t.Fatalf("sentence location = %+v", got)
	}
	if got := result.Sentences[0].Tokens[0].Location; got.StartOffset != 100_001 || got.EndOffset != 100_006 {
		t.Fatalf("token location = %+v", got)
	}
}
