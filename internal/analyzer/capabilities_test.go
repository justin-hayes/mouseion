package analyzer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	assert.False(t, first.Degraded)
	assert.Equal(t, 1, calls)
	_, err = provider.GetCapabilities(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "cached calls = %d", calls)
	now = now.Add(2 * time.Minute)
	stale, err := provider.GetCapabilities(context.Background())
	require.NoError(t, err)
	assert.True(t, stale.Degraded)
	assert.Equal(t, "de", stale.Languages[0].Language)
	assert.Equal(t, 2, calls)
}

func TestCachedCapabilityProviderReturnsInitialFailure(t *testing.T) {
	want := errors.New("unavailable")
	provider := NewCachedCapabilityProvider(capabilityProviderFunc(func(context.Context) (Capabilities, error) {
		return Capabilities{}, want
	}), time.Minute)
	_, err := provider.GetCapabilities(context.Background())
	assert.ErrorIs(t, err, want)
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
		require.FailNow(t, "cache mutex is held during capability RPC")
	}

	close(releaseLookup)
	lookups.Wait()
	assert.Equal(t, int32(1), calls.Load(), "capability RPC calls = %d, want 1", calls.Load())
	for range callers {
		err := <-errors
		require.NoError(t, err, "capability lookup error = %v", err)
		value := <-results
		require.Len(t, value.Languages, 1)
		assert.Equal(t, "de", value.Languages[0].Language)
		assert.False(t, value.Degraded)
	}
}

func TestCachedCapabilityProviderDoesNotCancelSharedLookup(t *testing.T) {
	var calls atomic.Int32
	lookupStarted := make(chan struct{})
	releaseLookup := make(chan struct{})
	provider := NewCachedCapabilityProvider(capabilityProviderFunc(func(ctx context.Context) (Capabilities, error) {
		if ctx.Err() != nil {
			return Capabilities{}, ctx.Err()
		}
		if calls.Add(1) == 1 {
			close(lookupStarted)
			<-releaseLookup
		}
		return Capabilities{Languages: []LanguageCapability{{Language: "de", Ready: true}}}, nil
	}), time.Minute)

	leaderContext, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, err := provider.GetCapabilities(leaderContext)
		leaderResult <- err
	}()
	<-lookupStarted
	cancelLeader()
	leaderErr := <-leaderResult
	assert.ErrorIs(t, leaderErr, context.Canceled, "leader error = %v, want context canceled", leaderErr) //nolint:testifylint // Leader cancellation and waiter recovery are independent outcomes.

	waiterResult := make(chan Capabilities, 1)
	waiterError := make(chan error, 1)
	go func() {
		value, err := provider.GetCapabilities(context.Background())
		waiterResult <- value
		waiterError <- err
	}()
	close(releaseLookup)

	waiterErr := <-waiterError
	require.NoError(t, waiterErr, "waiter error = %v", waiterErr)
	value := <-waiterResult
	require.Len(t, value.Languages, 1)
	assert.Equal(t, "de", value.Languages[0].Language)
	assert.False(t, value.Degraded)
	assert.Equal(t, int32(1), calls.Load(), "capability RPC calls = %d, want 1", calls.Load())
}
