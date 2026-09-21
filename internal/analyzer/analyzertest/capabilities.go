package analyzertest

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

type CapabilityProvider struct {
	Value analyzer.Capabilities
	Err   error
}

func (p CapabilityProvider) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return p.Value, p.Err
}

func ReadyDepparseCapabilityProvider() CapabilityProvider {
	return CapabilityProvider{Value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", SupportedFeatures: []string{analyzer.FeatureDepparse}, Ready: true},
		{Language: "it", SupportedFeatures: []string{analyzer.FeatureDepparse}, Ready: true},
		{Language: "el", DisplayName: "Greek", SupportedFeatures: []string{analyzer.FeatureDepparse}, Ready: true},
	}}}
}

var _ analyzer.CapabilityProvider = CapabilityProvider{}
