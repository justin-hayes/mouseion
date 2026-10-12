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
	"github.com/stretchr/testify/assert"
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

	calls, _ := provider.stats()
	assert.Zero(t, calls)
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomePending, outcomes[0].State)
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

	assert.Contains(t, output.String(), "prepared deck translation: integration provider warning")
	assert.Contains(t, metrics.metrics, BatchMetric{Mode: "standard", Name: MetricBatchUsageInputCount, Phase: "provider", State: "completed", Provider: "openai", Value: 11})
	assert.Contains(t, metrics.metrics, BatchMetric{Mode: "standard", Name: MetricBatchUsageOutputCount, Phase: "provider", State: "completed", Provider: "openai", Value: 7})
	assert.Contains(t, metrics.metrics, BatchMetric{Mode: "standard", Name: MetricBatchProviderTransitions, Phase: "translating", State: "completed", Provider: "openai", Value: 1})
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
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

	calls, _ := provider.stats()
	assert.Zero(t, calls, "provider must not be called after the claim is lost")
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeRunning, outcomes[0].State)
}

// "persistence" is not an accepted outcome error class, so recording the
// cache-write failure is itself rejected: the job returns an error for River
// to retry and the item keeps its running claim.
func TestStandardWorkerReturnsErrorWhenCacheWriteFails(t *testing.T) {
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

	err := worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0})
	require.ErrorContains(t, err, "deck_preparation_translation_outcomes_error_class_check")

	calls, _ := provider.stats()
	assert.Equal(t, 1, calls)
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeRunning, outcomes[0].State)
}
