package prepareddeck

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		unsetenv(t, BatchMaxRequestsEnv)
		unsetenv(t, BatchPollIntervalEnv)
		got, err := BatchConfigFromEnv()
		require.NoError(t, err)
		assert.Equal(t, BatchConfig{MaxRequests: 5000, PollInterval: 30 * time.Second}, got)
	})

	t.Run("custom", func(t *testing.T) {
		t.Setenv(BatchMaxRequestsEnv, "1234")
		t.Setenv(BatchPollIntervalEnv, "45s")
		got, err := BatchConfigFromEnv()
		require.NoError(t, err)
		assert.Equal(t, BatchConfig{MaxRequests: 1234, PollInterval: 45 * time.Second}, got)
	})

	for _, test := range []struct {
		name, env, value string
	}{
		{"empty request count", BatchMaxRequestsEnv, ""},
		{"non-numeric request count", BatchMaxRequestsEnv, "many"},
		{"zero request count", BatchMaxRequestsEnv, "0"},
		{"provider request limit", BatchMaxRequestsEnv, "50001"},
		{"empty poll interval", BatchPollIntervalEnv, ""},
		{"unitless poll interval", BatchPollIntervalEnv, "30"},
		{"zero poll interval", BatchPollIntervalEnv, "0s"},
		{"negative poll interval", BatchPollIntervalEnv, "-1s"},
		{"excessive poll interval", BatchPollIntervalEnv, "24h1s"},
	} {
		t.Run(test.name, func(t *testing.T) {
			unsetenv(t, BatchMaxRequestsEnv)
			unsetenv(t, BatchPollIntervalEnv)
			t.Setenv(test.env, test.value)
			_, err := BatchConfigFromEnv()
			assert.Error(t, err, "BatchConfigFromEnv() with %s=%q", test.env, test.value)
		})
	}
}

func TestPreparedDeckConfigFromEnv(t *testing.T) {
	for _, name := range []string{TranslationModeEnv, StandardMaxConcurrencyEnv, StandardMaxAttemptsEnv, StandardRetryBaseDelayEnv, StandardRetryMaxDelayEnv} {
		unsetenv(t, name)
	}
	got, err := PreparedDeckConfigFromEnv()
	require.NoError(t, err)
	want := PreparedDeckConfig{TranslationMode: "standard", StandardMaxConcurrency: 4, StandardMaxAttempts: 3, StandardRetryBaseDelay: time.Second, StandardRetryMaxDelay: 30 * time.Second}
	assert.Equal(t, want, got)
	t.Setenv(TranslationModeEnv, "batch")
	t.Setenv(StandardMaxConcurrencyEnv, "8")
	t.Setenv(StandardMaxAttemptsEnv, "7")
	t.Setenv(StandardRetryBaseDelayEnv, "2s")
	t.Setenv(StandardRetryMaxDelayEnv, "1m")
	got, err = PreparedDeckConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "batch", got.TranslationMode)
	assert.Equal(t, 8, got.StandardMaxConcurrency)
	assert.Equal(t, 7, got.StandardMaxAttempts)
	assert.Equal(t, 2*time.Second, got.StandardRetryBaseDelay)
	assert.Equal(t, time.Minute, got.StandardRetryMaxDelay)
	for _, test := range []struct{ env, value string }{
		{TranslationModeEnv, "other"}, {StandardMaxConcurrencyEnv, "0"}, {StandardMaxConcurrencyEnv, "65"},
		{StandardMaxAttemptsEnv, "0"}, {StandardMaxAttemptsEnv, "21"}, {StandardRetryBaseDelayEnv, "0s"},
		{StandardRetryMaxDelayEnv, "500ms"},
	} {
		t.Run(test.env+"="+test.value, func(t *testing.T) {
			t.Setenv(test.env, test.value)
			_, err := PreparedDeckConfigFromEnv()
			assert.Error(t, err, "accepted invalid %s=%q", test.env, test.value)
		})
	}
}

func unsetenv(t *testing.T, name string) {
	t.Helper()
	previous, existed := os.LookupEnv(name)
	require.NoError(t, os.Unsetenv(name))
	t.Cleanup(func() {
		if existed {
			require.NoError(t, os.Setenv(name, previous))
		}
	})
}
