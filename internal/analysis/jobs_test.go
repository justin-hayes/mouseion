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

func TestNormalizedArtifactHashIncludesAnalysisIdentity(t *testing.T) {
	result := analyzer.Result{
		Language:      "de",
		SchemaVersion: "1.0.0",
		NormalizationProfile: analyzer.NormalizationProfile{
			Name: "german-standard-post-1996", Version: "4",
		},
		Analysis: analyzer.AnalysisProvenance{AnalyzerName: "stanza", AnalyzerVersion: "1.14.0"},
	}
	base := normalizedArtifactHash("sha256:source", "scope", "scope-identity", "mouseion-scoped-analyzer", "3", "selection-default-v1", result)

	changedProfile := result
	changedProfile.NormalizationProfile.Version = "3"
	if got := normalizedArtifactHash("sha256:source", "scope", "scope-identity", "mouseion-scoped-analyzer", "3", "selection-default-v1", changedProfile); got == base {
		t.Fatal("normalization profile version did not change artifact identity")
	}
	if got := normalizedArtifactHash("sha256:source", "scope", "scope-identity", "mouseion-scoped-analyzer", "2", "selection-default-v1", result); got == base {
		t.Fatal("analyzer version did not change artifact identity")
	}
	if got := normalizedArtifactHash("sha256:source", "scope", "scope-identity", "mouseion-scoped-analyzer", "3", "selection-default-v2", result); got == base {
		t.Fatal("configuration identity did not change artifact identity")
	}
	if got := normalizedArtifactHash("sha256:source", "scope", "old-scope-identity", "mouseion-scoped-analyzer", "3", "selection-default-v1", result); got == base {
		t.Fatal("analysis identity did not change artifact identity")
	}
}

func TestOrdinaryAnalysisIdentityDoesNotReuseLegacyContentHash(t *testing.T) {
	contentHash := "sha256:source"
	legacyIdentity := contentHash // The pre-contract-change duplicate lookup key.
	currentIdentity := ordinaryAnalysisIdentity(contentHash)
	if currentIdentity == legacyIdentity {
		t.Fatalf("current ordinary identity reused legacy key %q", legacyIdentity)
	}
	if currentIdentity != ordinaryAnalysisIdentity(contentHash) {
		t.Fatal("ordinary analysis identity is not deterministic")
	}
}
