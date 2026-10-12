//go:build integration

package prepareddeck

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A translation job whose stored manifest fails its identity check must not
// call the provider. The immutability trigger is disabled in this test's own
// cloned database only, to simulate corruption that the trigger would block.
func TestStandardWorkerDoesNotTranslateWhenStoredManifestDigestIsTampered(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	_, err := run.store.Pool().Exec(ctx, `ALTER TABLE deck_preparation_manifests DISABLE TRIGGER deck_preparation_manifests_immutable`)
	require.NoError(t, err)
	_, err = run.store.Pool().Exec(ctx, `UPDATE deck_preparation_manifests SET manifest_digest=$4 WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`, run.owner, run.prep.ID, run.run.ID, strings.Repeat("0", 64))
	require.NoError(t, err)
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))

	calls, _ := provider.stats()
	assert.Zero(t, calls, "provider must not be called for a manifest that fails its identity check")
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[0].State)
	assert.Equal(t, "manifest", outcomes[0].ErrorCode)
}

// A job whose manifest item has gone missing must not reach the provider.
func TestStandardWorkerDoesNotTranslateWhenManifestItemIsMissing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	_, err := run.store.Pool().Exec(ctx, `DELETE FROM deck_preparation_manifest_items WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.Error(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))

	calls, _ := provider.stats()
	assert.Zero(t, calls, "provider must not be called for an unknown manifest item")
}

// A failing immutable cache lookup must fail the item without calling the provider.
func TestStandardWorkerDoesNotTranslateWhenCacheLookupFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	_, err := run.store.Pool().Exec(ctx, `ALTER TABLE enrichment_cache RENAME TO enrichment_cache_unavailable`)
	require.NoError(t, err)
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))

	t.Run("does not call the provider", func(t *testing.T) {
		calls, _ := provider.stats()
		require.Zero(t, calls, "provider must not be called when the cache lookup fails")
	})
	t.Run("records the cache-lookup failure", func(t *testing.T) {
		outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
		require.NoError(t, err)
		require.Len(t, outcomes, 1)
		require.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[0].State)
		require.Equal(t, "cache", outcomes[0].ErrorClass)
		require.Equal(t, "cache_lookup", outcomes[0].ErrorCode)
	})
}
