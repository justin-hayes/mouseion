package analyzer

import (
	"context"
	"errors"
	"testing"
	"time"
)

type capabilityProviderFunc func(context.Context) (Capabilities, error)

func (f capabilityProviderFunc) GetCapabilities(ctx context.Context) (Capabilities, error) {
	return f(ctx)
}

func TestCachedCapabilityProviderCachesAndReturnsStaleDataOnFailure(t *testing.T) {
	calls := 0
	provider := NewCachedCapabilityProvider(capabilityProviderFunc(func(context.Context) (Capabilities, error) {
		calls++
		if calls > 1 {
			return Capabilities{}, errors.New("unavailable")
		}
		return Capabilities{Languages: []LanguageCapability{{Language: "de", Ready: true}}}, nil
	}), time.Minute)
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	provider.now = func() time.Time { return now }

	first, err := provider.GetCapabilities(context.Background())
	if err != nil || first.Degraded || calls != 1 {
		t.Fatalf("first = %+v, calls = %d, err = %v", first, calls, err)
	}
	if _, err = provider.GetCapabilities(context.Background()); err != nil || calls != 1 {
		t.Fatalf("cached calls = %d, err = %v", calls, err)
	}
	now = now.Add(2 * time.Minute)
	stale, err := provider.GetCapabilities(context.Background())
	if err != nil || !stale.Degraded || stale.Languages[0].Language != "de" || calls != 2 {
		t.Fatalf("stale = %+v, calls = %d, err = %v", stale, calls, err)
	}
}

func TestCachedCapabilityProviderReturnsInitialFailure(t *testing.T) {
	want := errors.New("unavailable")
	provider := NewCachedCapabilityProvider(capabilityProviderFunc(func(context.Context) (Capabilities, error) {
		return Capabilities{}, want
	}), time.Minute)
	if _, err := provider.GetCapabilities(context.Background()); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
