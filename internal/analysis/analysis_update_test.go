package analysis

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingCapabilities struct{}

func (failingCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return analyzer.Capabilities{}, errors.New("unreachable")
}

func TestCurrentTargetTreatsUnknownIdentityAsCurrent(t *testing.T) {
	ready := analyzer.LanguageCapability{Language: "fr", Ready: true, NormalizationProfile: "french", NormalizationVersion: "2"}
	target, ok := currentTarget(analyzer.Capabilities{Languages: []analyzer.LanguageCapability{ready}}, "fr")
	require.True(t, ok)
	assert.Equal(t, domain.AnalysisUpdateTarget{AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion, NormalizationProfile: "french", NormalizationVersion: "2"}, target)

	notReady := ready
	notReady.Ready = false
	unversioned := ready
	unversioned.NormalizationVersion = ""
	for name, value := range map[string]analyzer.Capabilities{
		"not reported": {},
		"not ready":    {Languages: []analyzer.LanguageCapability{notReady}},
		"no version":   {Languages: []analyzer.LanguageCapability{unversioned}},
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := currentTarget(value, "fr")
			assert.False(t, ok)
		})
	}
}

func TestCurrentTargetsNeedAReachableService(t *testing.T) {
	for _, tt := range []struct {
		name    string
		service *Service
		known   bool
	}{
		{"no provider", &Service{}, false},
		{"unreachable service", (&Service{}).WithCapabilities(failingCapabilities{}), false},
		{"cached answer while unreachable", (&Service{}).WithCapabilities(analyzertest.CapabilityProvider{Value: analyzer.Capabilities{Degraded: true}}), false},
		{"reachable service", (&Service{}).WithCapabilities(analyzertest.CapabilityProvider{}), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, known := tt.service.currentTargets(context.Background())
			assert.Equal(t, tt.known, known)
		})
	}
}

func TestPipelineOutdatedComparesEveryIdentityPart(t *testing.T) {
	target := domain.AnalysisUpdateTarget{AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion, NormalizationProfile: "french", NormalizationVersion: "2"}
	current := domain.AnalysisPipelineIdentity{AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion, ConfigIdentity: mainTextConfigIdentity, NormalizationProfile: "french", NormalizationVersion: "2"}
	assert.False(t, pipelineOutdated(current, target, mainTextConfigIdentity))
	defaultConfig := current
	defaultConfig.ConfigIdentity = snapshotConfigIdentity
	assert.False(t, pipelineOutdated(defaultConfig, target, snapshotConfigIdentity))
	gutenberg := defaultConfig
	gutenberg.ConfigIdentity = distributorConfigIdentity
	assert.True(t, pipelineOutdated(gutenberg, target, snapshotConfigIdentity), "a Gutenberg snapshot now selects Distributor text")

	for name, change := range map[string]func(*domain.AnalysisPipelineIdentity){
		"analyzer name":         func(i *domain.AnalysisPipelineIdentity) { i.AnalyzerName = "other" },
		"analyzer version":      func(i *domain.AnalysisPipelineIdentity) { i.AnalyzerVersion = "2" },
		"retired configuration": func(i *domain.AnalysisPipelineIdentity) { i.ConfigIdentity = "selection-default-v0" },
		"profile":               func(i *domain.AnalysisPipelineIdentity) { i.NormalizationProfile = "unicode-casefold" },
		"version":               func(i *domain.AnalysisPipelineIdentity) { i.NormalizationVersion = "1" },
	} {
		t.Run(name, func(t *testing.T) {
			published := current
			change(&published)
			assert.True(t, pipelineOutdated(published, target, mainTextConfigIdentity))
		})
	}
}

func TestAnalysisUpdateEntryPointsRequireTheirInputs(t *testing.T) {
	ctx := context.Background()
	offers, err := (&Service{}).AnalysisUpdateCandidates(ctx, "owner", []string{"source"})
	require.NoError(t, err)
	assert.Empty(t, offers)
	var nilService *Service
	offers, err = nilService.AnalysisUpdateCandidates(ctx, "owner", nil)
	require.NoError(t, err)
	assert.Empty(t, offers)
	_, err = (&Service{}).SubmitAnalysisUpdate(ctx, "owner", "book", "source", "run", "target")
	require.Error(t, err)
}
