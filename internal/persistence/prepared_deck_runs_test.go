package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunConfigMatchesFrozenExecutionIdentity(t *testing.T) {
	config, err := validatePreparedDeckRunConfig(PreparedDeckRunConfig{})
	require.NoError(t, err)
	run := domain.PreparedDeckRun{
		ExecutionMode:       domain.PreparedDeckExecutionBatch,
		TargetLanguage:      "en",
		RetryPolicyVersion:  config.RetryPolicyVersion,
		MaxProviderAttempts: config.MaxProviderAttempts,
		MaxBatchGenerations: config.MaxBatchGenerations,
		BatchMaxRequests:    config.BatchMaxRequests,
		BatchMaxBytes:       config.BatchMaxBytes,
	}
	assert.True(t, runConfigMatches(run, PreparedDeckRunConfig{}), "default batch/en config should match frozen identity")
	config.ExecutionMode = "standard"
	assert.False(t, runConfigMatches(run, config), "changed execution mode matched frozen identity")
	config.ExecutionMode = "batch"
	config.TargetLanguage = "fr"
	assert.False(t, runConfigMatches(run, config), "changed target language matched frozen identity")
}
