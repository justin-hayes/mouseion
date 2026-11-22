package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJobLifecyclePredicates(t *testing.T) {
	t.Parallel()
	assert.True(t, JobLifecycleQueued.Active())
	assert.True(t, JobLifecycleRunning.Active())
	assert.False(t, JobLifecycleCompleted.Active())
	assert.False(t, JobLifecycleFailed.Active())
	assert.False(t, JobLifecycleCancelled.Active())

	assert.False(t, JobLifecycleQueued.Retryable())
	assert.False(t, JobLifecycleRunning.Retryable())
	assert.False(t, JobLifecycleCompleted.Retryable())
	assert.True(t, JobLifecycleFailed.Retryable())
	assert.True(t, JobLifecycleCancelled.Retryable())
}

func TestJobLifecycleForRiverStateMapsEveryStateToOneLifecycle(t *testing.T) {
	t.Parallel()
	tests := map[string]JobLifecycleState{
		"available": JobLifecycleQueued,
		"pending":   JobLifecycleQueued,
		"retryable": JobLifecycleQueued,
		"scheduled": JobLifecycleQueued,
		"queued":    JobLifecycleQueued,
		"running":   JobLifecycleRunning,
		"completed": JobLifecycleCompleted,
		"discarded": JobLifecycleFailed,
		"failed":    JobLifecycleFailed,
		"cancelled": JobLifecycleCancelled,
		"unknown":   JobLifecycleQueued,
		"":          JobLifecycleQueued,
	}

	for state, want := range tests {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, JobLifecycleForRiverState(state))
		})
	}
}
