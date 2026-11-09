package webapp

import (
	"bytes"
	"context"
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
		{ID: 1, DisplayNumber: 1, SourceMaterialID: "book-1", AnalysisState: "completed", Progress: 100},
		{ID: 2, DisplayNumber: 2, AnalysisState: "failed", Progress: 42, Error: "Analyzer stopped."},
		{ID: 3, DisplayNumber: 3, AnalysisState: "queued"},
		{ID: 4, DisplayNumber: 4, AnalysisState: "running"},
		{ID: 5, DisplayNumber: 5, AnalysisState: "cancelled"},
	}
	syncJobs := []cataloguesync.Status{{ID: 3, LogicalState: "cancelled", ConnectionName: "Archive"}}
	bookContexts := map[string]jobBookContext{"book-1": {Title: "Der lange Weg nach Hause", ReadingURL: "/reading#book-1"}}
	require.NoError(t, JobsPageWithCatalogueSync(domain.User{Username: "learner"}, "csrf", jobs, syncJobs, nil, "", bookContexts).Render(context.Background(), &history))
	html := history.String()
	assert.Contains(t, html, `<body class="jobs-shell">`)
	assert.NotContains(t, html, "pico-2.1.1.min.css")
	assert.Contains(t, html, `aria-label="Analysis history"`)
	assert.Contains(t, html, "Completed")
	assert.Contains(t, html, "Failed")
	assert.Contains(t, html, "Queued")
	assert.Contains(t, html, "Processing")
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
	status := analysis.Status{ID: 2, DisplayNumber: 2, LogicalState: "failed", State: "failed", SourceMaterialID: "book-1", Error: "Analyzer stopped."}
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
	require.NoError(t, JobsPageWithCatalogueSync(domain.User{Username: "learner"}, "csrf", nil, nil, jobs, "", nil).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `aria-label="Classical preparation history"`)
	assert.Contains(t, html, "Classical preparation")
	assert.Contains(t, html, "Anabasis")
	assert.Contains(t, html, "Running")
	assert.Contains(t, html, "2026-11-09 12:00")
}

func TestClassicalPreparationJobStateMapping(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"completed": "Completed", "discarded": "Failed", "cancelled": "Failed",
		"running": "Running", "available": "Queued", "pending": "Queued",
		"scheduled": "Queued", "retryable": "Queued", "unknown": "Queued",
	}
	for state, want := range cases {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, classicalPreparationJobState(state))
			assert.NotEmpty(t, classicalPreparationJobTone(state))
		})
	}
}

func TestOperationalShellStylesExcludePico(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, ShellLayout("Jobs", &domain.User{}, "csrf", NavigationLibrary, shellStyleJobs).Render(context.Background(), &output))
	assert.Contains(t, output.String(), `<body class="jobs-shell">`)
	assert.NotContains(t, output.String(), "pico-2.1.1.min.css")
}
