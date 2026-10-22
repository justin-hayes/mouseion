package analysis

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireDependencyParsing(t *testing.T) {
	provider := analyzertest.CapabilityProvider{Value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{
		Language: "de-DE", SupportedFeatures: []string{"tokenize", analyzer.FeatureDepparse}, Ready: true,
	}}}}

	t.Run("available dependency parsing is accepted", func(t *testing.T) {
		require.NoError(t, requireDependencyParsing(context.Background(), provider, "de"))
	})

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
	t.Run("degraded provider state still permits dependency parsing", func(t *testing.T) {
		require.NoError(t, requireDependencyParsing(context.Background(), degraded, "de"))
	})

	outage := analyzertest.CapabilityProvider{Err: errors.New("NLP unavailable")}
	err = requireDependencyParsing(context.Background(), outage, "de")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrDependencyParsingUnavailable)
	assert.Equal(t, "Analysis could not be completed. Retry the analysis or review the current source.", safeAnalysisError(err))
}

func TestSnapshotAnalysisExcludesSpeakerLabelsFromStatisticsButKeepsEvidence(t *testing.T) {
	result := analyzer.Result{
		Language: "de",
		Sentences: []analyzer.Sentence{{
			Text: "Valentin fällt. Das Buch.",
			Tokens: []analyzer.Token{
				{Surface: "Valentin", CanonicalLemma: "Valentin", UPOS: "PROPN", Dependency: "nsubj", Location: analyzer.SourceLocation{SourceDocumentID: "unit", EndOffset: 8}},
				{Surface: "fällt", CanonicalLemma: "fallen", UPOS: "VERB", Dependency: "root", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 9, EndOffset: 15}},
				{Surface: "Buch", CanonicalLemma: "Buch", UPOS: "NOUN", Dependency: "obl", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 21, EndOffset: 25}},
			},
			Location: analyzer.SourceLocation{SourceDocumentID: "unit", EndOffset: 26},
		}},
		Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "test", AnalyzerVersion: "1"},
		NormalizationProfile: analyzer.NormalizationProfile{Name: "test", Version: "1"},
	}
	merge := snapshotChunkMerge{result: result, completed: 1}
	structure := domain.SnapshotStructure{Recorded: true, RoleSpans: []domain.UnitRoleSpan{{UnitID: "unit", Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 15}}}

	analyzed, err := merge.analysis(JobArgs{ContentHash: "hash", AnalyzerName: "test", AnalyzerVersion: "1", ConfigIdentity: "config"}, structure)
	require.NoError(t, err)
	assert.Equal(t, int64(1), analyzed.statistics.AnalyzableTokenCount, "the speaker label is not an analyzable token")
	assert.Equal(t, int64(1), analyzed.statistics.DistinctLemmaCount)
	assert.Len(t, analyzed.result.Sentences[0].Tokens, 3, "all analyzer evidence stays in the corpus")
	assert.Equal(t, "fallen", analyzed.result.Sentences[0].Tokens[1].CanonicalLemma)
}

func TestAggregateLemmasIsIdempotent(t *testing.T) {
	result := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{{CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Sing"}}, {CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Sing"}}, {CanonicalLemma: "5", UPOS: "NOUN"}}}}}
	lemmas, err := aggregateLemmas("sha256:x", result)
	require.NoError(t, err)
	require.Len(t, lemmas, 1)
	assert.Equal(t, int64(2), lemmas[0].Frequency)
	assert.Equal(t, "sha256:x", lemmas[0].ContentHash)
}

func TestAggregateLemmasExcludesSeparableParticles(t *testing.T) {
	result := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{
		{CanonicalLemma: "aufstehen", UPOS: "VERB", Dependency: "root"},
		{CanonicalLemma: "auf", UPOS: "ADV", Dependency: "compound:prt"},
	}}}}

	lemmas, err := aggregateLemmas("sha256:x", result)
	require.NoError(t, err)

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

func TestSnapshotAnalysisUsesDeclaredMainTextSelection(t *testing.T) {
	units := []snapshotUnit{
		{UnitID: "front", Order: 0, LandmarkTypes: []string{"titlepage"}},
		{UnitID: "main", Order: 1, LandmarkTypes: []string{"BODYMATTER"}},
		{UnitID: "back", Order: 2, LandmarkTypes: []string{"bibliography"}},
	}

	decision := identifyMainText(units)
	assert.Equal(t, mainTextConfigIdentity, snapshotConfigIdentityFor(decision))
	assert.Equal(t, []string{"main"}, decision.SelectedUnitIDs)
	assert.Equal(t, []string{"front", "back"}, decision.ExcludedUnitIDs)
	assert.Equal(t, []snapshotUnit{units[1]}, selectedSnapshotUnits(units, decision.SelectedUnitIDs))

	noOp := identifyMainText([]snapshotUnit{
		{UnitID: "first", Order: 0, LandmarkTypes: []string{"bodymatter"}},
		{UnitID: "second", Order: 1},
	})
	assert.Equal(t, snapshotConfigIdentity, snapshotConfigIdentityFor(noOp))
	assert.False(t, noOp.Applies)
}

func TestCleanupCancelledAnalysisTreatsMissingLiveJobAsSuccess(t *testing.T) {
	cancelled := false
	err := cleanupCancelledAnalysis(context.Background(), "owner", "run", func(context.Context, string, string) (int64, error) {
		return 0, pgx.ErrNoRows
	}, func(context.Context, int64) (*rivertype.JobRow, error) {
		cancelled = true
		return &rivertype.JobRow{}, nil
	})

	require.NoError(t, err)
	assert.False(t, cancelled)
}

func TestCleanupCancelledAnalysisReportsPostCommitFailures(t *testing.T) {
	lookupErr := errors.New("River lookup unavailable")
	err := cleanupCancelledAnalysis(context.Background(), "owner", "run", func(context.Context, string, string) (int64, error) {
		return 0, lookupErr
	}, func(context.Context, int64) (*rivertype.JobRow, error) {
		return nil, errors.New("should not cancel")
	})
	require.ErrorIs(t, err, lookupErr)

	cancelErr := errors.New("River cancellation unavailable")
	err = cleanupCancelledAnalysis(context.Background(), "owner", "run", func(context.Context, string, string) (int64, error) {
		return 42, nil
	}, func(context.Context, int64) (*rivertype.JobRow, error) {
		return nil, cancelErr
	})
	require.ErrorIs(t, err, cancelErr)
}

func TestSelectionConfigWithoutSnapshotHasNoStructure(t *testing.T) {
	config, err := (&Worker{}).selectionConfig(context.Background(), JobArgs{}, "corpus")
	require.NoError(t, err)
	assert.Equal(t, "corpus", config.CorpusID)
	assert.False(t, config.Structure.Recorded, "a run without a snapshot supplies no structure")
}
