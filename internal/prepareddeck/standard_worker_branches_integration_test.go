//go:build integration

package prepareddeck

import (
	"bytes"
	"context"
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

// usageTranslationProvider reports token usage alongside each translation.
type usageTranslationProvider struct {
	*barrierTranslationProvider
	usage enrichment.TranslationUsage
}

func (p *usageTranslationProvider) TranslateWithUsage(ctx context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, enrichment.TranslationUsage, error) {
	response, err := p.Translate(ctx, request)
	return response, p.usage, err
}

func TestStandardWorkerReturnsUnknownRunErrorWithoutClaiming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.Error(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: uuid.NewString(), Ordinal: 0, Generation: 0}))

	t.Run("does not call the provider", func(t *testing.T) {
		calls, _ := provider.stats()
		require.Zero(t, calls)
	})
	t.Run("leaves the item pending", func(t *testing.T) {
		outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
		require.NoError(t, err)
		require.Len(t, outcomes, 1)
		require.Equal(t, domain.PreparedDeckOutcomePending, outcomes[0].State)
	})
}

func TestStandardWorkerRecordsProviderUsageAndLogsWarnings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, base := newStandardIntegrationRun(t, ctx, 1, 1)
	base.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		response := validTranslationResponse(request)
		response.Warnings = []string{"integration provider warning"}
		return response, nil
	}
	provider := &usageTranslationProvider{barrierTranslationProvider: base, usage: enrichment.TranslationUsage{PromptTokens: 11, CompletionTokens: 7}}
	metrics := &recordingBatchMetrics{}
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider, Metrics: metrics}

	var output bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))

	t.Run("logs provider warnings", func(t *testing.T) {
		require.Contains(t, output.String(), "prepared deck translation: integration provider warning")
	})
	t.Run("records provider token usage", func(t *testing.T) {
		require.Contains(t, metrics.metrics, BatchMetric{Mode: "standard", Name: MetricBatchUsageInputCount, Phase: "provider", State: "completed", Provider: "openai", Value: 11})
		require.Contains(t, metrics.metrics, BatchMetric{Mode: "standard", Name: MetricBatchUsageOutputCount, Phase: "provider", State: "completed", Provider: "openai", Value: 7})
	})
	t.Run("records the completed transition", func(t *testing.T) {
		require.Contains(t, metrics.metrics, BatchMetric{Mode: "standard", Name: MetricBatchProviderTransitions, Phase: "translating", State: "completed", Provider: "openai", Value: 1})
	})
	t.Run("completes the item", func(t *testing.T) {
		outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
		require.NoError(t, err)
		require.Len(t, outcomes, 1)
		require.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
	})
}

// A claim lost between the cache lookup and the provider call must leave the
// item to its new owner. The cache read is routed through a view whose
// evaluation steals the claim, in this test's own cloned database only.
func TestStandardWorkerDoesNotCallProviderAfterClaimLossOnCacheMiss(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	for _, statement := range []string{
		`ALTER TABLE enrichment_cache RENAME TO enrichment_cache_base`,
		`CREATE FUNCTION steal_translation_claim() RETURNS boolean LANGUAGE sql VOLATILE AS $$ UPDATE deck_preparation_translation_outcomes SET claim_token=gen_random_uuid() WHERE state='running'; SELECT true $$`,
		`CREATE VIEW enrichment_cache AS SELECT base.* FROM (SELECT steal_translation_claim() AS stolen) steal, enrichment_cache_base base`,
	} {
		_, err := run.store.Pool().Exec(ctx, statement)
		require.NoError(t, err, statement)
	}
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))

	t.Run("does not call the provider", func(t *testing.T) {
		calls, _ := provider.stats()
		require.Zero(t, calls)
	})
	t.Run("leaves the item to the new claim", func(t *testing.T) {
		outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
		require.NoError(t, err)
		require.Len(t, outcomes, 1)
		require.Equal(t, domain.PreparedDeckOutcomeRunning, outcomes[0].State)
	})
}

// A cache-write failure is recorded as a terminal failure with the cache error
// class, so the item does not keep its running claim.
func TestStandardWorkerRecordsCacheWriteFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if _, err := run.store.Pool().Exec(ctx, `ALTER TABLE enrichment_cache RENAME TO enrichment_cache_unavailable`); err != nil {
			return enrichment.TranslationResponse{}, err
		}
		return validTranslationResponse(request), nil
	}
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))

	t.Run("calls the provider once", func(t *testing.T) {
		calls, _ := provider.stats()
		require.Equal(t, 1, calls)
	})
	t.Run("records the cache-write failure", func(t *testing.T) {
		outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
		require.NoError(t, err)
		require.Len(t, outcomes, 1)
		require.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[0].State)
		require.Equal(t, "cache", outcomes[0].ErrorClass)
		require.Equal(t, "cache_write", outcomes[0].ErrorCode)
	})
}
