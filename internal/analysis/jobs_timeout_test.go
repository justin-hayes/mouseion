package analysis

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredJobTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		// t.Setenv records the prior value for restoration; the Unsetenv that
		// follows is what leaves the variable unset for this subtest.
		t.Setenv(jobTimeoutEnv, "")
		require.NoError(t, os.Unsetenv(jobTimeoutEnv))
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
