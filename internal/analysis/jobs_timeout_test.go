package analysis

import (
	"os"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredJobTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		previous, existed := os.LookupEnv(jobTimeoutEnv)
		require.NoError(t, os.Unsetenv(jobTimeoutEnv))
		if existed {
			testutil.Cleanup(t, "job timeout environment", func() error { return os.Setenv(jobTimeoutEnv, previous) })
		}
		timeout, err := configuredJobTimeout()
		require.NoError(t, err)
		assert.Equal(t, defaultJobTimeout, timeout)
	})

	t.Run("custom", func(t *testing.T) {
		t.Setenv(jobTimeoutEnv, "45m")
		timeout, err := configuredJobTimeout()
		require.NoError(t, err)
		assert.Equal(t, 45*time.Minute, timeout)
	})

	for _, value := range []string{"", "invalid", "0s", "-1s"} {
		t.Run("invalid_"+value, func(t *testing.T) {
			t.Setenv(jobTimeoutEnv, value)
			_, err := configuredJobTimeout()
			assert.Error(t, err, "configuredJobTimeout() with %q", value)
		})
	}
}
