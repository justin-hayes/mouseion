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

func TestBookPreparationKindLabelNamesEveryKind(t *testing.T) {
	assert.Equal(t, "Classical preparation", bookPreparationKindLabel(domain.BookPreparationKindClassical))
	assert.Equal(t, "EPUB acquisition", bookPreparationKindLabel(domain.BookPreparationKindEPUBAcquisition))
	assert.Equal(t, "Analysis", bookPreparationKindLabel(domain.BookPreparationKindAnalysis))
	assert.Equal(t, "Book preparation", bookPreparationKindLabel(domain.BookPreparationKind("unrecognised")))
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

// TestBookPreparationFailureReasonReusesMyBooksReasons pins the learner
// reason a failed row adds under its stable code to the wording Reading
// already explains for the same failure, for every kind and stage.
func TestBookPreparationFailureReasonReusesMyBooksReasons(t *testing.T) {
	tests := []struct {
		name string
		job  domain.BookPreparationJob
		want string
	}{
		{
			name: "classical fetch failure",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindClassical, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed},
			want: classicalAssessmentUnavailable,
		},
		{
			name: "classical source refusal",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindClassical, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageSource, Code: domain.BookPreparationCodeSourcePinMismatch},
			want: classicalSourceMismatchLabel,
		},
		{
			name: "classical import failure",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindClassical, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageImport, Code: domain.BookPreparationCodeClassicalFailed},
			want: classicalAnalysisFailedLabel,
		},
		{
			name: "classical withdrawal",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindClassical, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageWithdrawn, Code: domain.BookPreparationCodeClassicalWithdrawn},
			want: classicalWithdrawnLabel,
		},
		{
			name: "classical catalog precondition",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindClassical, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageCatalog, Code: domain.BookPreparationCodeClassicalCatalogLegacyRows},
			want: "Classical catalog unavailable",
		},
		{
			name: "acquisition entry missing",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindEPUBAcquisition, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageEntry, Code: domain.BookPreparationCodeCatalogEntryMissing},
			want: epubAcquisitionEntryMissingLabel,
		},
		{
			name: "acquisition download failure",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindEPUBAcquisition, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed},
			want: epubAcquisitionDownloadFailedLabel,
		},
		{
			name: "acquisition oversize",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindEPUBAcquisition, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageSource, Code: domain.BookPreparationCodeSourceOversize},
			want: epubAcquisitionOversizeLabel,
		},
		{
			name: "acquisition unusable file",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindEPUBAcquisition, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageSource, Code: domain.BookPreparationCodeSourceFormatUnknown},
			want: epubAcquisitionFormatLabel,
		},
		{
			name: "acquisition import failure",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindEPUBAcquisition, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageImport, Code: domain.BookPreparationCodeEPUBImportFailed},
			want: epubAcquisitionImportFailedLabel,
		},
		{
			name: "analysis dependency parsing unavailable",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindAnalysis, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageAnalysis, Code: domain.BookPreparationCodeDependencyParsingUnavailable},
			want: "Analysis failed because dependency parsing is unavailable.",
		},
		{
			name: "analysis interrupted",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindAnalysis, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageAnalysis, Code: domain.BookPreparationCodeAnalysisInterrupted},
			want: "The analysis was interrupted.",
		},
		{
			name: "generic analysis failure",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindAnalysis, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageFinalize, Code: domain.BookPreparationCodeAnalysisFailed},
			want: "Analysis failed.",
		},
		{
			name: "a failed row with no stage adds no reason",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindClassical, Lifecycle: domain.JobLifecycleFailed, Code: domain.BookPreparationCodeClassicalFailed},
			want: "",
		},
		{
			name: "a running row adds no reason",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindClassical, Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageSource, Code: domain.BookPreparationCodeSourcePinMismatch},
			want: "",
		},
		{
			name: "a completed row adds no reason",
			job:  domain.BookPreparationJob{Kind: domain.BookPreparationKindAnalysis, Lifecycle: domain.JobLifecycleCompleted, Stage: domain.BookPreparationStageFinalize, Code: domain.BookPreparationCodeAnalysisFailed},
			want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, bookPreparationFailureReason(test.job))
		})
	}
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
