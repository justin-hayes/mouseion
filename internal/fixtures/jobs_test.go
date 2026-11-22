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

	jobs, err := store.ListAnalysisJobs(ctx, OwnerID)
	require.NoError(t, err)
	require.NotEmpty(t, jobs)
	assert.Equal(t, domain.JobLifecycleCompleted, jobs[0].Lifecycle)
	assert.Equal(t, domain.BookPreparationStageFinalize, jobs[0].Stage)
	assert.Equal(t, domain.JobLifecycleRunning, jobs[len(jobs)-1].Lifecycle)

	completed, err := analysis.Get(ctx, OwnerID, 42)
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleCompleted, completed.Lifecycle)
	assert.Equal(t, domain.BookPreparationStageFinalize, completed.Stage)

	failed, err := analysis.Get(ctx, OwnerID, 43)
	require.NoError(t, err)
	assert.Equal(t, domain.JobLifecycleFailed, failed.Lifecycle)
	assert.Equal(t, domain.BookPreparationStageAnalysis, failed.Stage)

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
