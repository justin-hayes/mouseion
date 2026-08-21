package enrichmentjob

import (
	"os"
	"testing"
	"time"
)

func TestConfiguredJobTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		previous, existed := os.LookupEnv(jobTimeoutEnv)
		if err := os.Unsetenv(jobTimeoutEnv); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(jobTimeoutEnv, previous)
			}
		})
		timeout, err := configuredJobTimeout()
		if err != nil || timeout != defaultJobTimeout {
			t.Fatalf("configuredJobTimeout() = %v, %v; want %v, nil", timeout, err, defaultJobTimeout)
		}
	})

	t.Run("custom", func(t *testing.T) {
		t.Setenv(jobTimeoutEnv, "45m")
		timeout, err := configuredJobTimeout()
		if err != nil || timeout != 45*time.Minute {
			t.Fatalf("configuredJobTimeout() = %v, %v; want 45m, nil", timeout, err)
		}
	})

	for _, value := range []string{"", "invalid", "0s", "-1s"} {
		t.Run("invalid_"+value, func(t *testing.T) {
			t.Setenv(jobTimeoutEnv, value)
			if _, err := configuredJobTimeout(); err == nil {
				t.Fatalf("configuredJobTimeout() with %q returned nil error", value)
			}
		})
	}
}
