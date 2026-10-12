//go:build integration

package prepareddeck

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// The recovery paths open a real transaction before River is asked for work, so
// their refusals need the database. A client without live jobs stands in for River.
func TestRecoveryRefusesWorkRiverDidNotAccept(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store, err := persistence.Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	client := jobLessRiverClient{}

	require.EqualError(t, EnsureRecoveryJob(ctx, store, client), "prepareddeck: River did not return a live prepared-deck recovery job")

	worker := &RecoveryWorker{Store: store, Client: client}
	err = worker.repair(ctx, domain.PreparedDeckRecoveryWork{Kind: "finalizer", OwnerID: "owner", PreparationID: "preparation", RunID: "run"})
	require.EqualError(t, err, "prepareddeck: River did not return live durable recovery work")
}

func TestStandardTranslationGuardsRefuseWorkRiverDidNotAccept(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, _, _ := newStandardIntegrationRun(t, ctx, 1, 1)
	client := jobLessRiverClient{}

	recovery := &RecoveryWorker{Store: run.store, Client: client}
	err := recovery.repair(ctx, domain.PreparedDeckRecoveryWork{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0, Kind: "outcome"})
	require.EqualError(t, err, "prepareddeck: River did not return live standard translation recovery work")

	token := uuid.NewString()
	_, err = run.store.ClaimPreparedDeckTranslationOutcome(ctx, run.owner, run.prep.ID, run.run.ID, 0, 0, token, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	worker := &StandardTranslationWorker{Store: run.store, Client: client}
	args := StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}
	err = worker.retry(ctx, args, token, 1, "provider", "timeout")
	require.EqualError(t, err, "prepareddeck: River did not return a retry job")
}
