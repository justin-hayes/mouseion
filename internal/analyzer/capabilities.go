package analyzer

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"golang.org/x/sync/singleflight"
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

const FeatureDepparse = "depparse"

func (c LanguageCapability) Supports(feature string) bool {
	return slices.Contains(c.SupportedFeatures, feature)
}

type Capabilities struct {
	Languages []LanguageCapability
	Degraded  bool
}

func ReadySupportedLanguages(value Capabilities) []domain.SupportedLanguage {
	languages := make([]domain.SupportedLanguage, 0, len(value.Languages))
	for _, capability := range value.Languages {
		if capability.Ready {
			languages = append(languages, domain.SupportedLanguage{Language: canonicalization.NormalizeLanguage(capability.Language), DisplayName: capability.DisplayName})
		}
	}
	return languages
}

// CachedCapabilityProvider bounds capability RPC traffic and returns the last
// successful response as degraded data during temporary NLP outages.
type CachedCapabilityProvider struct {
	provider CapabilityProvider
	ttl      time.Duration
	now      func() time.Time
	lookup   singleflight.Group

	mu      sync.Mutex
	value   Capabilities
	fetched time.Time
}

func NewCachedCapabilityProvider(provider CapabilityProvider, ttl time.Duration) *CachedCapabilityProvider {
	return &CachedCapabilityProvider{provider: provider, ttl: ttl, now: time.Now}
}

func (c *CachedCapabilityProvider) GetCapabilities(ctx context.Context) (Capabilities, error) {
	c.mu.Lock()
	now := c.now()
	if c.cacheFresh(now) {
		value := cloneCapabilities(c.value)
		c.mu.Unlock()
		return value, nil
	}
	c.mu.Unlock()

	resultCh := c.lookup.DoChan("capabilities", func() (any, error) {
		c.mu.Lock()
		now := c.now()
		if c.cacheFresh(now) {
			value := cloneCapabilities(c.value)
			c.mu.Unlock()
			return value, nil
		}
		c.mu.Unlock()

		// Keep a shared refresh alive for other waiters, but retain the elected caller's deadline.
		lookupContext := context.WithoutCancel(ctx)
		if deadline, ok := ctx.Deadline(); ok {
			var cancel context.CancelFunc
			lookupContext, cancel = context.WithDeadline(lookupContext, deadline)
			defer cancel()
		}
		value, err := c.provider.GetCapabilities(lookupContext)
		if err == nil {
			value.Degraded = false
			c.mu.Lock()
			c.value = cloneCapabilities(value)
			c.fetched = now
			result := cloneCapabilities(value)
			c.mu.Unlock()
			return result, nil
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.fetched.IsZero() {
			value = cloneCapabilities(c.value)
			value.Degraded = true
			return value, nil
		}
		return Capabilities{}, err
	})
	select {
	case result := <-resultCh:
		if result.Err != nil {
			return Capabilities{}, result.Err
		}
		return cloneCapabilities(result.Val.(Capabilities)), nil
	case <-ctx.Done():
		return Capabilities{}, ctx.Err()
	}
}

func (c *CachedCapabilityProvider) cacheFresh(now time.Time) bool {
	return !c.fetched.IsZero() && now.Sub(c.fetched) < c.ttl
}

func cloneCapabilities(value Capabilities) Capabilities {
	result := Capabilities{Degraded: value.Degraded, Languages: make([]LanguageCapability, len(value.Languages))}
	copy(result.Languages, value.Languages)
	for i := range result.Languages {
		result.Languages[i].SupportedFeatures = append([]string(nil), value.Languages[i].SupportedFeatures...)
	}
	return result
}
