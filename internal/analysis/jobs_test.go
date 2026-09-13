package analysis

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireDependencyParsing(t *testing.T) {
	provider := analyzertest.CapabilityProvider{Value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{
		Language: "de-DE", SupportedFeatures: []string{"tokenize", analyzer.FeatureDepparse}, Ready: true,
	}}}}

	assert.NoError(t, requireDependencyParsing(context.Background(), provider, "de"))

	missing := analyzertest.CapabilityProvider{Value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{
		Language: "de", SupportedFeatures: []string{"tokenize", "pos", "lemma"}, Ready: true,
	}}}}
	err := requireDependencyParsing(context.Background(), missing, "de")
	require.ErrorIs(t, err, ErrDependencyParsingUnavailable)
	assert.Contains(t, err.Error(), `language "de"`)

	degraded := analyzertest.CapabilityProvider{Value: analyzer.Capabilities{
		Degraded:  true,
		Languages: []analyzer.LanguageCapability{{Language: "de", SupportedFeatures: []string{analyzer.FeatureDepparse}}},
	}}
	assert.NoError(t, requireDependencyParsing(context.Background(), degraded, "de"))

	outage := analyzertest.CapabilityProvider{Err: errors.New("NLP unavailable")}
	err = requireDependencyParsing(context.Background(), outage, "de")
	assert.NotErrorIs(t, err, ErrDependencyParsingUnavailable)
	assert.Equal(t, "Analysis could not be completed. Retry the analysis or review the current source.", safeAnalysisError(err))
}

func TestAggregateLemmasIsIdempotent(t *testing.T) {
	result := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{{CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Sing"}}, {CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Sing"}}, {CanonicalLemma: "5", UPOS: "NOUN"}}}}}
	lemmas := aggregateLemmas("sha256:x", result)
	require.Len(t, lemmas, 1)
	assert.Equal(t, int64(2), lemmas[0].Frequency)
	assert.Equal(t, "sha256:x", lemmas[0].ContentHash)
}

func TestAggregateLemmasExcludesSeparableParticles(t *testing.T) {
	result := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{
		{CanonicalLemma: "aufstehen", UPOS: "VERB", Dependency: "root"},
		{CanonicalLemma: "auf", UPOS: "ADV", Dependency: "compound:prt"},
	}}}}

	lemmas := aggregateLemmas("sha256:x", result)

	require.Len(t, lemmas, 1)
	assert.Equal(t, "aufstehen", lemmas[0].CanonicalLemma)
}

func TestOffsetResultLocations(t *testing.T) {
	result := analyzer.Result{Sentences: []analyzer.Sentence{{
		Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 8},
		Tokens:   []analyzer.Token{{Location: analyzer.SourceLocation{StartOffset: 1, EndOffset: 6}}},
	}}}

	offsetResultLocations(&result, 100_000)

	assert.Equal(t, analyzer.SourceLocation{StartOffset: 100_000, EndOffset: 100_008}, result.Sentences[0].Location)
	assert.Equal(t, analyzer.SourceLocation{StartOffset: 100_001, EndOffset: 100_006}, result.Sentences[0].Tokens[0].Location)
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
	assert.NotEqual(t, base, normalizedArtifactHash("sha256:source", "scope", "scope-identity", "mouseion-scoped-analyzer", "3", "selection-default-v1", changedProfile), "normalization profile version did not change artifact identity")
	assert.NotEqual(t, base, normalizedArtifactHash("sha256:source", "scope", "scope-identity", "mouseion-scoped-analyzer", "2", "selection-default-v1", result), "analyzer version did not change artifact identity")
	assert.NotEqual(t, base, normalizedArtifactHash("sha256:source", "scope", "scope-identity", "mouseion-scoped-analyzer", "3", "selection-default-v2", result), "configuration identity did not change artifact identity")
	assert.NotEqual(t, base, normalizedArtifactHash("sha256:source", "scope", "old-scope-identity", "mouseion-scoped-analyzer", "3", "selection-default-v1", result), "analysis identity did not change artifact identity")
}

func TestOrdinaryAnalysisIdentityDoesNotReuseLegacyContentHash(t *testing.T) {
	contentHash := "sha256:source"
	legacyIdentity := contentHash // The pre-contract-change duplicate lookup key.
	currentIdentity := ordinaryAnalysisIdentity(contentHash)
	assert.NotEqual(t, legacyIdentity, currentIdentity, "current ordinary identity reused legacy key %q", legacyIdentity)
	assert.Equal(t, currentIdentity, ordinaryAnalysisIdentity(contentHash), "ordinary analysis identity is not deterministic")
}
