package analyzer

import (
	"context"
	"sync"
	"time"
)

// CapabilityProvider reports the analysis languages currently exposed by the
// NLP service.
type CapabilityProvider interface {
	GetCapabilities(context.Context) (Capabilities, error)
}

type LanguageCapability struct {
	Language          string
	DisplayName       string
	ModelVersion      string
	SupportedFeatures []string
	Ready             bool
}

type Capabilities struct {
	Languages []LanguageCapability
	Degraded  bool
}

// CachedCapabilityProvider bounds capability RPC traffic and returns the last
// successful response as degraded data during temporary NLP outages.
type CachedCapabilityProvider struct {
	provider CapabilityProvider
	ttl      time.Duration
	now      func() time.Time

	mu      sync.Mutex
	value   Capabilities
	fetched time.Time
}

func NewCachedCapabilityProvider(provider CapabilityProvider, ttl time.Duration) *CachedCapabilityProvider {
	return &CachedCapabilityProvider{provider: provider, ttl: ttl, now: time.Now}
}

func (c *CachedCapabilityProvider) GetCapabilities(ctx context.Context) (Capabilities, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if !c.fetched.IsZero() && now.Sub(c.fetched) < c.ttl {
		return cloneCapabilities(c.value), nil
	}
	value, err := c.provider.GetCapabilities(ctx)
	if err == nil {
		value.Degraded = false
		c.value = cloneCapabilities(value)
		c.fetched = now
		return cloneCapabilities(value), nil
	}
	if !c.fetched.IsZero() {
		value = cloneCapabilities(c.value)
		value.Degraded = true
		return value, nil
	}
	return Capabilities{}, err
}

func cloneCapabilities(value Capabilities) Capabilities {
	result := Capabilities{Degraded: value.Degraded, Languages: make([]LanguageCapability, len(value.Languages))}
	copy(result.Languages, value.Languages)
	for i := range result.Languages {
		result.Languages[i].SupportedFeatures = append([]string(nil), value.Languages[i].SupportedFeatures...)
	}
	return result
}
