package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
