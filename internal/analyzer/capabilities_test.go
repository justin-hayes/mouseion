package analyzer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

func TestCachedCapabilityProviderSharesInFlightLookup(t *testing.T) {
	var calls atomic.Int32
	lookupStarted := make(chan struct{})
	releaseLookup := make(chan struct{})
	provider := NewCachedCapabilityProvider(capabilityProviderFunc(func(context.Context) (Capabilities, error) {
		if calls.Add(1) == 1 {
			close(lookupStarted)
			<-releaseLookup
		}
		return Capabilities{Languages: []LanguageCapability{{Language: "de", Ready: true}}}, nil
	}), time.Minute)

	const callers = 8
	start := make(chan struct{})
	ready := sync.WaitGroup{}
	ready.Add(callers)
	var lookups sync.WaitGroup
	lookups.Add(callers)
	results := make(chan Capabilities, callers)
	errors := make(chan error, callers)
	for range callers {
		go func() {
			defer lookups.Done()
			ready.Done()
			<-start
			value, err := provider.GetCapabilities(context.Background())
			results <- value
			errors <- err
		}()
	}
	ready.Wait()
	close(start)
	<-lookupStarted

	lockAcquired := make(chan struct{})
	go func() {
		provider.mu.Lock()
		close(lockAcquired)
		provider.mu.Unlock()
	}()
	select {
	case <-lockAcquired:
	case <-time.After(time.Second):
		t.Fatal("cache mutex is held during capability RPC")
	}

	close(releaseLookup)
	lookups.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("capability RPC calls = %d, want 1", got)
	}
	for range callers {
		if err := <-errors; err != nil {
			t.Fatalf("capability lookup error = %v", err)
		}
		value := <-results
		if len(value.Languages) != 1 || value.Languages[0].Language != "de" || value.Degraded {
			t.Fatalf("capability lookup = %+v", value)
		}
	}
}
