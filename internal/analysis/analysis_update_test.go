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
		_, ok := currentTarget(value, "fr")
		assert.False(t, ok, name)
	}
}

func TestCurrentTargetsNeedAReachableService(t *testing.T) {
	ctx := context.Background()
	_, known := (&Service{}).currentTargets(ctx)
	assert.False(t, known, "no provider")
	_, known = (&Service{}).WithCapabilities(failingCapabilities{}).currentTargets(ctx)
	assert.False(t, known, "unreachable service")
	_, known = (&Service{}).WithCapabilities(analyzertest.CapabilityProvider{Value: analyzer.Capabilities{Degraded: true}}).currentTargets(ctx)
	assert.False(t, known, "a cached answer served while the service is unreachable")
	_, known = (&Service{}).WithCapabilities(analyzertest.CapabilityProvider{}).currentTargets(ctx)
	assert.True(t, known)
}

func TestPipelineOutdatedComparesEveryIdentityPart(t *testing.T) {
	target := domain.AnalysisUpdateTarget{AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion, NormalizationProfile: "french", NormalizationVersion: "2"}
	current := domain.AnalysisPipelineIdentity{AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion, ConfigIdentity: mainTextConfigIdentity, NormalizationProfile: "french", NormalizationVersion: "2"}
	assert.False(t, pipelineOutdated(current, target))
	defaultConfig := current
	defaultConfig.ConfigIdentity = snapshotConfigIdentity
	assert.False(t, pipelineOutdated(defaultConfig, target))

	for name, change := range map[string]func(*domain.AnalysisPipelineIdentity){
		"analyzer name":         func(i *domain.AnalysisPipelineIdentity) { i.AnalyzerName = "other" },
		"analyzer version":      func(i *domain.AnalysisPipelineIdentity) { i.AnalyzerVersion = "2" },
		"retired configuration": func(i *domain.AnalysisPipelineIdentity) { i.ConfigIdentity = "selection-default-v0" },
		"profile":               func(i *domain.AnalysisPipelineIdentity) { i.NormalizationProfile = "unicode-casefold" },
		"version":               func(i *domain.AnalysisPipelineIdentity) { i.NormalizationVersion = "1" },
	} {
		published := current
		change(&published)
		assert.True(t, pipelineOutdated(published, target), name)
	}
}

func TestAnalysisUpdateEntryPointsRequireTheirInputs(t *testing.T) {
	ctx := context.Background()
	offers, err := (&Service{}).AnalysisUpdateOffers(ctx, "owner", []string{"source"})
	require.NoError(t, err)
	assert.Empty(t, offers)
	var nilService *Service
	offers, err = nilService.AnalysisUpdateOffers(ctx, "owner", nil)
	require.NoError(t, err)
	assert.Empty(t, offers)
	_, err = (&Service{}).SubmitAnalysisUpdate(ctx, "owner", "book", "source", "run", "target")
	require.Error(t, err)
}
