package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestRunConfigMatchesFrozenExecutionIdentity(t *testing.T) {
	config, err := validatePreparedDeckRunConfig(PreparedDeckRunConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run := domain.PreparedDeckRun{
		ExecutionMode:       domain.PreparedDeckExecutionBatch,
		TargetLanguage:      "en",
		RetryPolicyVersion:  config.RetryPolicyVersion,
		MaxProviderAttempts: config.MaxProviderAttempts,
		MaxBatchGenerations: config.MaxBatchGenerations,
		BatchMaxRequests:    config.BatchMaxRequests,
		BatchMaxBytes:       config.BatchMaxBytes,
	}
	if !runConfigMatches(run, PreparedDeckRunConfig{}) {
		t.Fatal("default batch/en config should match frozen identity")
	}
	config.ExecutionMode = "standard"
	if runConfigMatches(run, config) {
		t.Fatal("changed execution mode matched frozen identity")
	}
	config.ExecutionMode = "batch"
	config.TargetLanguage = "fr"
	if runConfigMatches(run, config) {
		t.Fatal("changed target language matched frozen identity")
	}
}
