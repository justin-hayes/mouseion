package prepareddeck

import (
	"os"
	"testing"
	"time"
)

func TestBatchConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		unsetenv(t, BatchMaxRequestsEnv)
		unsetenv(t, BatchPollIntervalEnv)
		got, err := BatchConfigFromEnv()
		if err != nil || got != (BatchConfig{MaxRequests: 5000, PollInterval: 30 * time.Second}) {
			t.Fatalf("BatchConfigFromEnv() = %+v, %v", got, err)
		}
	})

	t.Run("custom", func(t *testing.T) {
		t.Setenv(BatchMaxRequestsEnv, "1234")
		t.Setenv(BatchPollIntervalEnv, "45s")
		got, err := BatchConfigFromEnv()
		if err != nil || got != (BatchConfig{MaxRequests: 1234, PollInterval: 45 * time.Second}) {
			t.Fatalf("BatchConfigFromEnv() = %+v, %v", got, err)
		}
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
			if _, err := BatchConfigFromEnv(); err == nil {
				t.Fatalf("BatchConfigFromEnv() with %s=%q returned nil error", test.env, test.value)
			}
		})
	}
}

func unsetenv(t *testing.T, name string) {
	t.Helper()
	previous, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, previous)
		}
	})
}
