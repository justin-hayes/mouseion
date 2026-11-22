package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestJobLifecycleViewLabelsAndTones(t *testing.T) {
	tests := []struct {
		state domain.JobLifecycleState
		label string
		tone  StatusTone
	}{
		{domain.JobLifecycleQueued, "Queued", StatusInfo},
		{domain.JobLifecycleRunning, "Running", StatusInfo},
		{domain.JobLifecycleCompleted, "Completed", StatusSuccess},
		{domain.JobLifecycleFailed, "Failed", StatusDanger},
		{domain.JobLifecycleCancelled, "Cancelled", StatusDanger},
	}

	for _, test := range tests {
		t.Run(string(test.state), func(t *testing.T) {
			view := jobLifecycleViewFor(test.state)
			assert.Equal(t, test.label, view.Label)
			assert.Equal(t, test.tone, view.Tone)
		})
	}
}

func TestJobLifecycleViewUnknownStateDefaultsToQueued(t *testing.T) {
	view := jobLifecycleViewFor(domain.JobLifecycleState("unrecognised"))
	assert.Equal(t, "Queued", view.Label)
	assert.Equal(t, StatusInfo, view.Tone)
}

func TestAnalysisStatusSummaryCoversEveryLifecycle(t *testing.T) {
	tests := map[string]struct {
		status analysis.Status
		want   string
	}{
		"completed at finalize": {
			status: analysis.Status{Lifecycle: domain.JobLifecycleCompleted, Stage: domain.BookPreparationStageFinalize},
			want:   analysisCompleteSummary,
		},
		"completed at analysis": {
			status: analysis.Status{Lifecycle: domain.JobLifecycleCompleted, Stage: domain.BookPreparationStageAnalysis},
			want:   analysisCompleteSummary,
		},
		"running at finalize": {
			status: analysis.Status{Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageFinalize},
			want:   "The analysis corpus is complete; selection and publication are still in progress.",
		},
		"running at analysis": {
			status: analysis.Status{Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageAnalysis, Progress: 42, Attempt: 1},
			want:   "42% complete · attempt 1",
		},
		"queued at analysis": {
			status: analysis.Status{Lifecycle: domain.JobLifecycleQueued, Stage: domain.BookPreparationStageAnalysis},
			want:   "0% complete · attempt 1",
		},
		"failed at analysis": {
			status: analysis.Status{Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageAnalysis},
			want:   "Analysis failed. Review the message and retry the EPUB snapshot when you are ready.",
		},
		"cancelled at analysis": {
			status: analysis.Status{Lifecycle: domain.JobLifecycleCancelled, Stage: domain.BookPreparationStageAnalysis},
			want:   "Analysis cancelled. Retry the EPUB snapshot when you are ready.",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, analysisStatusSummary(test.status))
		})
	}
}

func TestCatalogueSyncJobSummaryUsesLifecycle(t *testing.T) {
	assert.Equal(t, "Catalog metadata sync complete. No EPUB content was downloaded.", catalogueSyncJobSummary(cataloguesync.Status{Lifecycle: domain.JobLifecycleCompleted}))
	assert.Equal(t, "Authentication failed.", catalogueSyncJobSummary(cataloguesync.Status{Lifecycle: domain.JobLifecycleFailed, Error: "Authentication failed."}))
	assert.Equal(t, "35% complete · attempt 2", catalogueSyncJobSummary(cataloguesync.Status{Lifecycle: domain.JobLifecycleRunning, Progress: 35, Attempt: 2}))
}

func TestCatalogueSyncStatusRunningUsesLifecycle(t *testing.T) {
	assert.True(t, catalogueSyncStatusRunning(cataloguesync.Status{Lifecycle: domain.JobLifecycleQueued}))
	assert.True(t, catalogueSyncStatusRunning(cataloguesync.Status{Lifecycle: domain.JobLifecycleRunning}))
	assert.False(t, catalogueSyncStatusRunning(cataloguesync.Status{Lifecycle: domain.JobLifecycleCompleted}))
	assert.False(t, catalogueSyncStatusRunning(cataloguesync.Status{Lifecycle: domain.JobLifecycleFailed}))
	assert.False(t, catalogueSyncStatusRunning(cataloguesync.Status{Lifecycle: domain.JobLifecycleCancelled}))
}

func TestAnalysisFinalizationFailureCanBeRetried(t *testing.T) {
	status := analysis.Status{Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageFinalize}
	if !jobRetryable(status) {
		t.Fatal("completed corpus with failed publication should be retryable")
	}
	if got := analysisStatusSummary(status); got != "The analysis corpus is complete, but finalization failed. Its corpus was retained; retry to finish selection and publication without rerunning NLP." {
		t.Fatalf("analysisStatusSummary() = %q", got)
	}
}

func TestCancelledAnalysisFinalizationCanBeRetried(t *testing.T) {
	status := analysis.Status{Lifecycle: domain.JobLifecycleCancelled, Stage: domain.BookPreparationStageFinalize}
	if !jobRetryable(status) {
		t.Fatal("cancelled finalization should be retryable")
	}
	if got := analysisStatusSummary(status); got != "Analysis finalization was cancelled. The completed corpus was retained; retry when you are ready." {
		t.Fatalf("analysisStatusSummary() = %q", got)
	}
}
