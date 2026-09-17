package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/riverqueue/river/rivertype"
)

func TestJobState(t *testing.T) {
	tests := []struct {
		name  string
		state rivertype.JobState
		want  string
	}{
		{name: "completed", state: rivertype.JobStateCompleted, want: "Succeeded"},
		{name: "discarded", state: rivertype.JobStateDiscarded, want: "Failed"},
		{name: "cancelled", state: rivertype.JobStateCancelled, want: "Failed"},
		{name: "running", state: rivertype.JobStateRunning, want: "Running"},
		{name: "available", state: rivertype.JobStateAvailable, want: "Available"},
		{name: "pending", state: rivertype.JobStatePending, want: "Pending"},
		{name: "retryable", state: rivertype.JobStateRetryable, want: "Retryable"},
		{name: "scheduled", state: rivertype.JobStateScheduled, want: "Scheduled"},
		{name: "unicode", state: rivertype.JobState("über"), want: "Über"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := jobState(analysis.Status{State: test.state}); got != test.want {
				t.Errorf("jobState(%q) = %q, want %q", test.state, got, test.want)
			}
		})
	}
}
