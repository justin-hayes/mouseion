package fixtures

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureAnalysisJobsExposeTheSharedLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	analysis := Analysis{Store: store}

	jobs, err := store.ListBookPreparationJobs(ctx, OwnerID)
	require.NoError(t, err)
	require.NotEmpty(t, jobs)
	assert.Equal(t, domain.BookPreparationKindAnalysis, jobs[0].Kind)
	assert.Equal(t, domain.JobLifecycleCompleted, jobs[0].Lifecycle)
	assert.Equal(t, domain.BookPreparationStageFinalize, jobs[0].Stage)
	assert.Equal(t, domain.JobLifecycleRunning, jobs[len(jobs)-1].Lifecycle)

	failed := jobs[0]
	for _, job := range jobs {
		if job.Kind == domain.BookPreparationKindAnalysis && job.DisplayNumber == 2 {
			failed = job
		}
		assert.Equal(t, domain.BookPreparationKindAnalysis, job.Kind)
		assert.NotEmpty(t, job.BookID, "every fixture row links to its Book")
		assert.NotEmpty(t, job.BookTitle, "every fixture row names its Book")
		require.NotNil(t, job.Progress, "analysis rows report progress")
	}
	assert.Equal(t, "fixture-failed", failed.BookID)
	assert.Equal(t, "Fehlgeschlagene Analyse", failed.BookTitle)
	assert.Equal(t, domain.JobLifecycleFailed, failed.Lifecycle)
	assert.Equal(t, domain.BookPreparationStageAnalysis, failed.Stage)

	completed, err := analysis.Get(ctx, OwnerID, 42)
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleCompleted, completed.Lifecycle)
	assert.Equal(t, domain.BookPreparationStageFinalize, completed.Stage)

	failedStatus, err := analysis.Get(ctx, OwnerID, 43)
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleFailed, failedStatus.Lifecycle)
	assert.Equal(t, domain.BookPreparationStageAnalysis, failedStatus.Stage)

	cancelled, err := analysis.Cancel(ctx, OwnerID, fixtureCancellableJobID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleCancelled, cancelled.Lifecycle)
}

func TestFixtureCatalogueSyncJobsExposeTheSharedLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	sync := NewCatalogueSync(store)

	jobs, err := sync.List(ctx, OwnerID)
	require.NoError(t, err)
	lifecycles := make(map[string]domain.JobLifecycleState, len(jobs))
	for _, job := range jobs {
		lifecycles[job.ConnectionID] = job.Lifecycle
	}
	assert.Equal(t, domain.JobLifecycleCompleted, lifecycles["fixture-connection"])
	assert.Equal(t, domain.JobLifecycleFailed, lifecycles["fixture-failed-connection"])
	assert.Equal(t, domain.JobLifecycleRunning, lifecycles["fixture-syncing-connection"])

	unknown, err := sync.jobStatus(ctx, domain.CatalogueSyncStatus{OwnerID: OwnerID, ConnectionID: "fixture-syncing-connection", State: domain.CatalogueSyncState("unrecognised")})
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleRunning, unknown.Lifecycle)

	cancelled, err := sync.Cancel(ctx, OwnerID, fixtureCatalogueSyncJobIDs["fixture-syncing-connection"])
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleFailed, cancelled.Lifecycle)
}
