package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobsViewsUseOwnedShellAndExposeStatusText(t *testing.T) {
	var history bytes.Buffer
	jobs := []domain.AnalysisJob{
		{ID: 1, DisplayNumber: 1, SourceMaterialID: "book-1", Lifecycle: domain.JobLifecycleCompleted, Stage: domain.BookPreparationStageFinalize, Progress: 100},
		{ID: 2, DisplayNumber: 2, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageAnalysis, Progress: 42, Error: "Analyzer stopped."},
		{ID: 3, DisplayNumber: 3, Lifecycle: domain.JobLifecycleQueued, Stage: domain.BookPreparationStageAnalysis},
		{ID: 4, DisplayNumber: 4, Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageAnalysis},
		{ID: 5, DisplayNumber: 5, Lifecycle: domain.JobLifecycleCancelled, Stage: domain.BookPreparationStageAnalysis},
	}
	syncJobs := []cataloguesync.Status{{ID: 3, Lifecycle: domain.JobLifecycleCancelled, ConnectionName: "Archive"}}
	bookContexts := map[string]jobBookContext{"book-1": {Title: "Der lange Weg nach Hause", ReadingURL: "/reading#book-1"}}
	require.NoError(t, JobsPageWithCatalogueSync(domain.User{Username: "learner"}, "csrf", jobs, syncJobs, nil, "", "", bookContexts).Render(context.Background(), &history))
	html := history.String()
	assert.Contains(t, html, `<body class="jobs-shell">`)
	assert.NotContains(t, html, "pico-2.1.1.min.css")
	assert.Contains(t, html, `aria-label="Analysis history"`)
	assert.Contains(t, html, "Completed")
	assert.Contains(t, html, "Failed")
	assert.Contains(t, html, "Queued")
	assert.Contains(t, html, "Running")
	assert.NotContains(t, html, "Processing")
	assert.Contains(t, html, "Cancelled")
	assert.Contains(t, html, `aria-label="Analysis job 1 progress"`)
	assert.Contains(t, html, "Der lange Weg nach Hause")
	var analysisHistory bytes.Buffer
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", jobs, "", bookContexts).Render(context.Background(), &analysisHistory))
	assert.Contains(t, analysisHistory.String(), "Status")
	assert.Contains(t, analysisHistory.String(), "Failed")
	assert.Contains(t, analysisHistory.String(), "Unavailable")
	assert.NotContains(t, analysisHistory.String(), "0001-01-01")

	var detail bytes.Buffer
	status := analysis.Status{ID: 2, DisplayNumber: 2, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageAnalysis, State: "failed", SourceMaterialID: "book-1", Error: "Analyzer stopped."}
	require.NoError(t, JobPage(domain.User{Username: "learner"}, "csrf", status, "", "Der lange Weg nach Hause").Render(context.Background(), &detail))
	detailHTML := detail.String()
	assert.Contains(t, detailHTML, `<body class="jobs-shell">`)
	assert.NotContains(t, detailHTML, "pico-2.1.1.min.css")
	assert.Contains(t, detailHTML, "Retry analysis")
	assert.Contains(t, detailHTML, "Failed")
	assert.Contains(t, detailHTML, "Der lange Weg nach Hause")
	assert.Contains(t, detailHTML, "Unavailable")
	assert.NotContains(t, detailHTML, "0001-01-01")
	assert.Contains(t, detailHTML, `role="status"`)
}

func TestJobsViewsShowClassicalPreparationBookAttemptAndState(t *testing.T) {
	var output bytes.Buffer
	jobs := []domain.ClassicalPreparationJob{{
		ID: 9, BookID: "book-9", BookTitle: "Anabasis", Attempt: 1, State: "running",
		CreatedAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
	}}
	require.NoError(t, JobsPageWithCatalogueSync(domain.User{Username: "learner"}, "csrf", nil, nil, jobs, "", "", nil).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `aria-label="Classical preparation history"`)
	assert.Contains(t, html, "Classical preparation")
	assert.Contains(t, html, "Anabasis")
	assert.Contains(t, html, "Running")
	assert.Contains(t, html, "2026-11-09 12:00")
}

func TestJobsViewsRenderFinalizationLifecycleAtFinalize(t *testing.T) {
	var output bytes.Buffer
	jobs := []domain.AnalysisJob{
		{ID: 1, DisplayNumber: 1, Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageFinalize},
		{ID: 2, DisplayNumber: 2, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageFinalize},
		{ID: 3, DisplayNumber: 3, Lifecycle: domain.JobLifecycleCancelled, Stage: domain.BookPreparationStageFinalize},
	}
	require.NoError(t, JobsPageWithCatalogueSync(domain.User{Username: "learner"}, "csrf", jobs, nil, nil, "", "", nil).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "Running")
	assert.Contains(t, html, "Failed")
	assert.Contains(t, html, "Cancelled")
	assert.NotContains(t, html, "Processing")
}

func TestAnalysisFinalizationDetailOffersRetryAndRendersLifecycle(t *testing.T) {
	t.Run("failed finalization", func(t *testing.T) {
		var output bytes.Buffer
		status := analysis.Status{ID: 7, DisplayNumber: 7, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageFinalize, State: "discarded"}
		require.NoError(t, JobPage(domain.User{Username: "learner"}, "csrf", status, "", "").Render(context.Background(), &output))
		html := output.String()
		assert.Contains(t, html, "Failed")
		assert.Contains(t, html, "Retry analysis")
		assert.NotContains(t, html, "Cancel analysis")
	})

	t.Run("cancelled finalization", func(t *testing.T) {
		var output bytes.Buffer
		status := analysis.Status{ID: 8, DisplayNumber: 8, Lifecycle: domain.JobLifecycleCancelled, Stage: domain.BookPreparationStageFinalize, State: "cancelled"}
		require.NoError(t, JobPage(domain.User{Username: "learner"}, "csrf", status, "", "").Render(context.Background(), &output))
		html := output.String()
		assert.Contains(t, html, "Cancelled")
		assert.Contains(t, html, "Retry analysis")
	})

	t.Run("running finalization", func(t *testing.T) {
		var output bytes.Buffer
		status := analysis.Status{ID: 9, DisplayNumber: 9, Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageFinalize, State: "running"}
		require.NoError(t, JobPage(domain.User{Username: "learner"}, "csrf", status, "", "").Render(context.Background(), &output))
		html := output.String()
		assert.Contains(t, html, "Running")
		assert.NotContains(t, html, "Cancel analysis", "finalization keeps today's actions and is left to settle")
		assert.NotContains(t, html, "Retry analysis")
	})
}

func TestJobsViewsUseOneLifecycleBadgeForEveryKind(t *testing.T) {
	var output bytes.Buffer
	analysisJobs := []domain.AnalysisJob{{ID: 1, DisplayNumber: 1, Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageAnalysis}}
	preparationJobs := []domain.ClassicalPreparationJob{{ID: 2, BookID: "book-2", BookTitle: "Anabasis", State: "running", CreatedAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}}
	syncJobs := []cataloguesync.Status{{ID: 3, ConnectionName: "Archive", Lifecycle: domain.JobLifecycleRunning}}
	require.NoError(t, JobsPageWithCatalogueSync(domain.User{Username: "learner"}, "csrf", analysisJobs, syncJobs, preparationJobs, "", "", nil).Render(context.Background(), &output))
	html := output.String()
	assert.GreaterOrEqual(t, strings.Count(html, "status-badge--info"), 3, "every running row uses the shared info badge")
	assert.Equal(t, 3, strings.Count(html, ">Running</span>"), "every running row reads Running")
}

func TestOperationalShellStylesExcludePico(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, ShellLayout("Jobs", &domain.User{}, "csrf", NavigationLibrary, shellStyleJobs).Render(context.Background(), &output))
	assert.Contains(t, output.String(), `<body class="jobs-shell">`)
	assert.NotContains(t, output.String(), "pico-2.1.1.min.css")
}
