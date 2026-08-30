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

func TestPreparedDeckConfigFromEnv(t *testing.T) {
	for _, name := range []string{TranslationModeEnv, StandardMaxConcurrencyEnv, StandardMaxAttemptsEnv, StandardRetryBaseDelayEnv, StandardRetryMaxDelayEnv} {
		unsetenv(t, name)
	}
	got, err := PreparedDeckConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	want := PreparedDeckConfig{TranslationMode: "standard", StandardMaxConcurrency: 4, StandardMaxAttempts: 3, StandardRetryBaseDelay: time.Second, StandardRetryMaxDelay: 30 * time.Second}
	if got != want {
		t.Fatalf("defaults=%+v want=%+v", got, want)
	}
	t.Setenv(TranslationModeEnv, "batch")
	t.Setenv(StandardMaxConcurrencyEnv, "8")
	t.Setenv(StandardMaxAttemptsEnv, "7")
	t.Setenv(StandardRetryBaseDelayEnv, "2s")
	t.Setenv(StandardRetryMaxDelayEnv, "1m")
	got, err = PreparedDeckConfigFromEnv()
	if err != nil || got.TranslationMode != "batch" || got.StandardMaxConcurrency != 8 || got.StandardMaxAttempts != 7 || got.StandardRetryBaseDelay != 2*time.Second || got.StandardRetryMaxDelay != time.Minute {
		t.Fatalf("custom=%+v err=%v", got, err)
	}
	for _, test := range []struct{ env, value string }{
		{TranslationModeEnv, "other"}, {StandardMaxConcurrencyEnv, "0"}, {StandardMaxConcurrencyEnv, "65"},
		{StandardMaxAttemptsEnv, "0"}, {StandardMaxAttemptsEnv, "21"}, {StandardRetryBaseDelayEnv, "0s"},
		{StandardRetryMaxDelayEnv, "500ms"},
	} {
		t.Run(test.env+"="+test.value, func(t *testing.T) {
			t.Setenv(test.env, test.value)
			if _, err := PreparedDeckConfigFromEnv(); err == nil { t.Fatalf("accepted invalid %s=%q", test.env, test.value) }
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
