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

// bookPreparationFixtureRows is one row of each Book preparation kind in the
// states the /jobs page must tell apart: a failed Classical preparation with
// the stage and code My Books explains, a running EPUB acquisition, and a
// failed analysis with its code at the analysis stage.
func bookPreparationFixtureRows() []domain.BookPreparationJob {
	analysisProgress := 42
	return []domain.BookPreparationJob{
		{
			Kind:      domain.BookPreparationKindClassical,
			JobID:     11,
			BookID:    "book-classical",
			BookTitle: "Anabasis",
			Lifecycle: domain.JobLifecycleFailed,
			Stage:     domain.BookPreparationStageSource,
			Code:      domain.BookPreparationCodeSourcePinMismatch,
			CreatedAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
		},
		{
			Kind:      domain.BookPreparationKindEPUBAcquisition,
			JobID:     12,
			BookID:    "book-epub",
			BookTitle: "Der lange Weg nach Hause",
			Lifecycle: domain.JobLifecycleRunning,
			CreatedAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
		},
		{
			Kind:          domain.BookPreparationKindAnalysis,
			JobID:         13,
			DisplayNumber: 2,
			BookID:        "book-epub",
			BookTitle:     "Der lange Weg nach Hause",
			Lifecycle:     domain.JobLifecycleFailed,
			Stage:         domain.BookPreparationStageAnalysis,
			Code:          domain.BookPreparationCodeDependencyParsingUnavailable,
			Progress:      &analysisProgress,
			CreatedAt:     time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC),
		},
	}
}

// TestJobsPageShowsEveryPreparationKindInOneTableAndCatalogSyncApart is the
// page contract: one Book preparations table covers all three kinds with the
// same columns, and Catalog sync sits in its own table with the shared badge,
// its connection and its error text.
func TestJobsPageShowsEveryPreparationKindInOneTableAndCatalogSyncApart(t *testing.T) {
	var output bytes.Buffer
	rows := bookPreparationFixtureRows()
	syncJobs := []cataloguesync.Status{{
		ID: 31, ConnectionName: "Archive", Lifecycle: domain.JobLifecycleFailed,
		Error: "The catalog could not be reached.", CreatedAt: time.Date(2026, 11, 8, 9, 30, 0, 0, time.UTC),
	}}
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", rows, syncJobs, "", "").Render(context.Background(), &output))
	html := output.String()

	assert.Contains(t, html, `<body class="jobs-shell">`)
	assert.Contains(t, html, "<h2>Book preparations</h2>")
	assert.Contains(t, html, `aria-label="Book preparations"`)
	assert.NotContains(t, html, `aria-label="Analysis history"`, "the per-kind tables are gone")
	assert.NotContains(t, html, `aria-label="Classical preparation history"`, "the per-kind tables are gone")
	assert.NotContains(t, html, `aria-label="Catalog sync history"`)

	t.Run("every kind reads its own label in the one table", func(t *testing.T) {
		assert.Contains(t, html, "Classical preparation")
		assert.Contains(t, html, "EPUB acquisition")
		assert.Contains(t, html, `href="/jobs/13">Analysis #2</a>`, "an analysis row keeps its detail link")
	})

	t.Run("every Book preparation row links to its Book", func(t *testing.T) {
		assert.Contains(t, html, `href="/catalogs/books/book-classical/in-my-books">Anabasis</a>`)
		assert.Contains(t, html, `href="/catalogs/books/book-epub/in-my-books">Der lange Weg nach Hause</a>`)
	})

	t.Run("a failed row shows the stage and code My Books explains", func(t *testing.T) {
		classicalStart := strings.Index(html, "Anabasis")
		require.GreaterOrEqual(t, classicalStart, 0)
		row := html[classicalStart:]
		assert.Contains(t, row, ">source<")
		assert.Contains(t, row, "<code>source_pin_mismatch</code>")
	})

	t.Run("a failed analysis row shows its code at the analysis stage", func(t *testing.T) {
		analysisStart := strings.Index(html, `Analysis #2`)
		require.GreaterOrEqual(t, analysisStart, 0)
		row := html[analysisStart:]
		assert.Contains(t, row, ">analysis<")
		assert.Contains(t, row, "<code>dependency_parsing_unavailable</code>")
	})

	t.Run("only analysis rows report progress", func(t *testing.T) {
		assert.Contains(t, html, `aria-label="Analysis job 2 progress"`)
		acquisitionStart := strings.Index(html, "EPUB acquisition")
		require.GreaterOrEqual(t, acquisitionStart, 0)
		acquisitionRow := html[acquisitionStart : strings.Index(html[acquisitionStart:], "Analysis #2")+acquisitionStart]
		assert.NotContains(t, acquisitionRow, "<progress", "an acquisition row reports no progress")
	})

	t.Run("every row states its status with the shared badge", func(t *testing.T) {
		for _, label := range []string{"Failed", "Running"} {
			assert.Contains(t, html, label)
		}
		assert.NotContains(t, html, "Processing")
	})

	t.Run("every row shows its created time", func(t *testing.T) {
		assert.Contains(t, html, "2026-11-09 12:00:00 UTC")
		assert.Contains(t, html, "2026-11-10 12:00:00 UTC")
		assert.Contains(t, html, "2026-11-11 12:00:00 UTC")
	})

	t.Run("Catalog sync stays in its own table", func(t *testing.T) {
		assert.Contains(t, html, "<h2>Catalog sync</h2>")
		assert.Contains(t, html, `aria-label="Catalog sync"`)
		assert.Contains(t, html, `href="/jobs/31">Archive</a>`, "a sync row keeps its detail link")
		assert.Contains(t, html, "The catalog could not be reached.")
	})
}

// TestJobsPageRendersRowsInListOrder keeps the page honest about the one
// list's newest-first contract: the page renders the rows exactly as the list
// returns them.
func TestJobsPageRendersRowsInListOrder(t *testing.T) {
	var output bytes.Buffer
	rows := []domain.BookPreparationJob{
		{Kind: domain.BookPreparationKindAnalysis, JobID: 1, DisplayNumber: 1, BookID: "b1", BookTitle: "Newest book", Lifecycle: domain.JobLifecycleQueued, Stage: domain.BookPreparationStageAnalysis},
		{Kind: domain.BookPreparationKindEPUBAcquisition, JobID: 2, BookID: "b2", BookTitle: "Older book", Lifecycle: domain.JobLifecycleCompleted},
	}
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", rows, nil, "", "").Render(context.Background(), &output))
	html := output.String()
	assert.Less(t, strings.Index(html, "Newest book"), strings.Index(html, "Older book"), "the page keeps the list's newest-first order")
}

// TestJobsPageShowsTheSharedEmptyState keeps one empty state for both tables.
func TestJobsPageShowsTheSharedEmptyState(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", nil, nil, "", "").Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "No jobs yet")
	assert.Contains(t, html, "Book preparation and catalog sync work will appear here when submitted.")
	assert.NotContains(t, html, `aria-label="Book preparations"`)
	assert.NotContains(t, html, `aria-label="Catalog sync"`)
}

func TestJobsViewsRenderFinalizationLifecycleAtFinalize(t *testing.T) {
	progress := 60
	var output bytes.Buffer
	jobs := []domain.BookPreparationJob{
		{Kind: domain.BookPreparationKindAnalysis, JobID: 1, DisplayNumber: 1, Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageFinalize, Progress: &progress},
		{Kind: domain.BookPreparationKindAnalysis, JobID: 2, DisplayNumber: 2, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageFinalize, Code: domain.BookPreparationCodeAnalysisFailed},
		{Kind: domain.BookPreparationKindAnalysis, JobID: 3, DisplayNumber: 3, Lifecycle: domain.JobLifecycleCancelled, Stage: domain.BookPreparationStageFinalize},
	}
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", jobs, nil, "", "").Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "Running")
	assert.Contains(t, html, "Failed")
	assert.Contains(t, html, "Cancelled")
	assert.NotContains(t, html, "Processing")
}

func TestJobsViewsUseOneLifecycleBadgeForEveryKind(t *testing.T) {
	var output bytes.Buffer
	preparations := []domain.BookPreparationJob{
		{Kind: domain.BookPreparationKindAnalysis, JobID: 1, DisplayNumber: 1, BookID: "book-1", BookTitle: "Analysis book", Lifecycle: domain.JobLifecycleRunning, Stage: domain.BookPreparationStageAnalysis},
		{Kind: domain.BookPreparationKindClassical, JobID: 2, BookID: "book-2", BookTitle: "Anabasis", Lifecycle: domain.JobLifecycleRunning},
		{Kind: domain.BookPreparationKindEPUBAcquisition, JobID: 3, BookID: "book-3", BookTitle: "EPUB book", Lifecycle: domain.JobLifecycleRunning},
	}
	syncJobs := []cataloguesync.Status{{ID: 4, ConnectionName: "Archive", Lifecycle: domain.JobLifecycleRunning}}
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", preparations, syncJobs, "", "").Render(context.Background(), &output))
	html := output.String()
	assert.GreaterOrEqual(t, strings.Count(html, "status-badge--info"), 4, "every running row uses the shared info badge")
	assert.Equal(t, 4, strings.Count(html, ">Running</span>"), "every running row reads Running")
}

func TestJobsViewsKeepTheAnalysisDetailPageContract(t *testing.T) {
	var detail bytes.Buffer
	status := analysis.Status{ID: 2, DisplayNumber: 2, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageAnalysis, State: "failed", SourceMaterialID: "book-1", FailureCode: domain.BookPreparationCodeDependencyParsingUnavailable}
	require.NoError(t, JobPage(domain.User{Username: "learner"}, "csrf", status, "", "Der lange Weg nach Hause").Render(context.Background(), &detail))
	detailHTML := detail.String()
	assert.Contains(t, detailHTML, `<body class="jobs-shell">`)
	assert.Contains(t, detailHTML, "Retry analysis")
	assert.Contains(t, detailHTML, "Failed")
	assert.Contains(t, detailHTML, "Der lange Weg nach Hause")
	assert.Contains(t, detailHTML, "Unavailable")
	assert.Contains(t, detailHTML, "Analysis failed because dependency parsing is unavailable. Retry the analysis when you are ready.")
	assert.NotContains(t, detailHTML, "0001-01-01")
	assert.Contains(t, detailHTML, `role="status"`)

	var legacyDetail bytes.Buffer
	legacyStatus := analysis.Status{ID: 2, DisplayNumber: 2, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageAnalysis, State: "failed", SourceMaterialID: "book-1"}
	require.NoError(t, JobPage(domain.User{Username: "learner"}, "csrf", legacyStatus, "", "Der lange Weg nach Hause").Render(context.Background(), &legacyDetail))
	assert.Contains(t, legacyDetail.String(), "Analysis failed. Retry the analysis when you are ready.")

	var interruptedDetail bytes.Buffer
	interruptedStatus := analysis.Status{ID: 2, DisplayNumber: 2, Lifecycle: domain.JobLifecycleFailed, Stage: domain.BookPreparationStageAnalysis, State: "failed", SourceMaterialID: "book-1", FailureCode: domain.BookPreparationCodeAnalysisInterrupted}
	require.NoError(t, JobPage(domain.User{Username: "learner"}, "csrf", interruptedStatus, "", "Der lange Weg nach Hause").Render(context.Background(), &interruptedDetail))
	assert.Contains(t, interruptedDetail.String(), "The analysis was interrupted. Retry the analysis when you are ready.")
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

func TestOperationalShellStylesExcludePico(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, ShellLayout("Jobs", &domain.User{}, "csrf", NavigationLibrary, shellStyleJobs).Render(context.Background(), &output))
	assert.Contains(t, output.String(), `<body class="jobs-shell">`)
	assert.NotContains(t, output.String(), "pico-2.1.1.min.css")
}
