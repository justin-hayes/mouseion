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

func TestAnalysisFinalizationFailureCanBeRetried(t *testing.T) {
	status := analysis.Status{LogicalState: "finalization-failed"}
	if !jobRetryable(status) {
		t.Fatal("completed corpus with failed publication should be retryable")
	}
	if got := analysisStatusSummary(status); got != "The analysis corpus is complete, but finalization failed. Its corpus was retained; retry to finish selection and publication without rerunning NLP." {
		t.Fatalf("analysisStatusSummary() = %q", got)
	}
}

func TestCancelledAnalysisFinalizationCanBeRetried(t *testing.T) {
	status := analysis.Status{LogicalState: "finalization-cancelled"}
	if !jobRetryable(status) {
		t.Fatal("cancelled finalization should be retryable")
	}
	if got := analysisStatusSummary(status); got != "Analysis finalization was cancelled. The completed corpus was retained; retry when you are ready." {
		t.Fatalf("analysisStatusSummary() = %q", got)
	}
}
