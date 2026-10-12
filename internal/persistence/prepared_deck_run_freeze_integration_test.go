//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type preparedDeckFreezeFixture struct {
	ownerID     string
	preparation domain.DeckPreparation
	projection  cardexport.StorageProjection
}

// newPreparedDeckFreezeFixture builds one owner, source and preparation whose
// frozen manifest is produced through the public presentation lifecycle.
func newPreparedDeckFreezeFixture(t *testing.T, ctx context.Context, store *PostgresStore, provider testutil.PresentationProvider, entries []cardexport.Entry) preparedDeckFreezeFixture {
	t.Helper()
	owner, err := store.CreateUser(ctx, "freeze-"+uuid.NewString(), false)
	require.NoError(t, err)
	title := "Freeze Fixture"
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: title, MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("Haus"), FullText: "Haus"})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(title), DeckName: title, ContentHash: source.ContentHash})
	require.NoError(t, err)
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, title, entries, provider)
	require.NoError(t, err)
	return preparedDeckFreezeFixture{ownerID: owner.ID, preparation: preparation, projection: deck.StorageProjection()}
}

func preparedDeckFreezeEntry() cardexport.Entry {
	return cardexport.Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: "Freeze Fixture", FirstEncounter: 1}
}

func preparedDeckExternalConfig() PreparedDeckRunConfig {
	return PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test"}
}

// preparedDeckSingleChunkPlan is a well-formed Batch plan covering ordinal 0.
func preparedDeckSingleChunkPlan() PreparedDeckBatchChunkPlan {
	return PreparedDeckBatchChunkPlan{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("a", 64), InputBytes: 64, Ordinals: []int{0}}
}

func beginPreparedDeckFreezeTx(t *testing.T, ctx context.Context, store *PostgresStore) pgx.Tx {
	t.Helper()
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	t.Cleanup(func() { rollbackIntegrationTx(t, ctx, tx) })
	return tx
}

// attemptPreparedDeckFreeze drives one freeze attempt that is expected to fail,
// rolling its transaction back so the fixture stays reusable.
func attemptPreparedDeckFreeze(t *testing.T, ctx context.Context, store *PostgresStore, params FreezePreparedDeckRunParams) (FreezePreparedDeckRunResult, error) {
	t.Helper()
	tx := beginPreparedDeckFreezeTx(t, ctx, store)
	result, freezeErr := store.FreezePreparedDeckRunTx(ctx, tx, params)
	rollbackIntegrationTx(t, ctx, tx)
	return result, freezeErr
}

// freezePreparedDeckRun freezes a run, optionally attaching the first chunk's
// submission job, and commits it.
func freezePreparedDeckRun(t *testing.T, ctx context.Context, store *PostgresStore, params FreezePreparedDeckRunParams, submissionJobID int64) FreezePreparedDeckRunResult {
	t.Helper()
	tx := beginPreparedDeckFreezeTx(t, ctx, store)
	result, err := store.FreezePreparedDeckRunTx(ctx, tx, params)
	if err != nil {
		rollbackIntegrationTx(t, ctx, tx)
		require.NoError(t, err)
	}
	if submissionJobID != 0 {
		chunk := result.Chunks[0]
		require.NoError(t, store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, params.OwnerID, params.PreparationID, result.Run.ID, chunk.ID, chunk.Generation, submissionJobID))
	}
	require.NoError(t, tx.Commit(ctx))
	return result
}

// TestFreezePreparedDeckRunValidatesBeforeWriting pins every rejection rule that
// runs before a run is written. Each case rolls its transaction back, so one
// fixture can drive them all.
func TestFreezePreparedDeckRunValidatesBeforeWriting(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))

	local := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{}, []cardexport.Entry{preparedDeckFreezeEntry()})
	external := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"}, []cardexport.Entry{preparedDeckFreezeEntry()})

	blocked := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{}, []cardexport.Entry{preparedDeckFreezeEntry()})
	_, err := store.Pool().Exec(ctx, `UPDATE deck_preparations SET state='failed' WHERE owner_id=$1 AND id=$2`, blocked.ownerID, blocked.preparation.ID)
	require.NoError(t, err)

	tests := []struct {
		name   string
		params FreezePreparedDeckRunParams
		want   error
	}{
		{
			name:   "missing owner identity",
			params: FreezePreparedDeckRunParams{PreparationID: local.preparation.ID, Projection: local.projection},
			want:   ErrInvalidTransition,
		},
		{
			name:   "unknown preparation",
			params: FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: uuid.NewString(), Projection: local.projection},
			want:   ErrNotFound,
		},
		{
			name:   "declared manifest digest mismatch",
			params: FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, ExpectedManifestDigest: strings.Repeat("f", 64), Projection: local.projection},
			want:   ErrImmutable,
		},
		{
			name:   "invalid durable run identity",
			params: FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, RunID: "not-a-uuid", Projection: local.projection},
			want:   ErrImmutable,
		},
		{
			name:   "preparation is not accepting a run",
			params: FreezePreparedDeckRunParams{OwnerID: blocked.ownerID, PreparationID: blocked.preparation.ID, Projection: blocked.projection},
			want:   ErrInvalidTransition,
		},
		{
			name: "invalid frozen execution identity",
			params: FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, Projection: local.projection,
				Config: PreparedDeckRunConfig{ExecutionMode: "streaming"}},
			want: ErrInvalidTransition,
		},
		{
			name: "disabled external translation has provider identity",
			params: FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, Projection: local.projection,
				Config: PreparedDeckRunConfig{Provider: "openai"}},
			want: ErrInvalidTransition,
		},
		{
			name: "incomplete external translation identity",
			params: FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, Projection: local.projection,
				Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions"}},
			want: ErrInvalidTransition,
		},
		{
			name: "invalid durable run limits",
			params: FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, Projection: local.projection,
				Config: PreparedDeckRunConfig{MaxBatchGenerations: 99}},
			want: ErrInvalidTransition,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, test.params)
			require.ErrorIs(t, freezeErr, test.want)
		})
	}

	externalChunks := func(chunks []PreparedDeckBatchChunkPlan) FreezePreparedDeckRunParams {
		return FreezePreparedDeckRunParams{OwnerID: external.ownerID, PreparationID: external.preparation.ID, Projection: external.projection, Config: preparedDeckExternalConfig(), Chunks: chunks}
	}
	t.Run("manifest owner contradicts preparation", func(t *testing.T) {
		projection := local.projection
		projection.Owner = "00000000-0000-0000-0000-000000000009"
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, Projection: projection})
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("manifest schema is not current", func(t *testing.T) {
		projection := local.projection
		projection.SchemaVersion = cardexport.LegacyManifestSchemaVersion
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, Projection: projection})
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("manifest filename contradicts preparation", func(t *testing.T) {
		projection := local.projection
		projection.DeckName = "Other Deck"
		projection.Filename = cardexport.DownloadFilename("Other Deck")
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, Projection: projection})
		require.ErrorIs(t, freezeErr, ErrPreparedDeckIdentity)
	})
	t.Run("cache identity contradicts disabled translation", func(t *testing.T) {
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, FreezePreparedDeckRunParams{OwnerID: external.ownerID, PreparationID: external.preparation.ID, Projection: external.projection})
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("cache identity contradicts the provider snapshot", func(t *testing.T) {
		config := preparedDeckExternalConfig()
		config.Provider = "other"
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, FreezePreparedDeckRunParams{OwnerID: external.ownerID, PreparationID: external.preparation.ID, Projection: external.projection, Config: config})
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("cache identity contradicts the context mode", func(t *testing.T) {
		config := preparedDeckExternalConfig()
		config.ContextMode = "lemma_only"
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, FreezePreparedDeckRunParams{OwnerID: external.ownerID, PreparationID: external.preparation.ID, Projection: external.projection, Config: config})
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("standard run cannot contain Batch chunks", func(t *testing.T) {
		params := FreezePreparedDeckRunParams{OwnerID: local.ownerID, PreparationID: local.preparation.ID, Projection: local.projection,
			Config: PreparedDeckRunConfig{ExecutionMode: "standard"}, Chunks: []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()}}
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, params)
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("Batch plans do not cover pending items", func(t *testing.T) {
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, externalChunks(nil))
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("Batch plan is malformed", func(t *testing.T) {
		chunks := []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()}
		chunks[0].Generation = 0
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, externalChunks(chunks))
		require.ErrorIs(t, freezeErr, ErrInvalidTransition)
	})
	t.Run("Batch plan split reason is unknown", func(t *testing.T) {
		chunks := []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()}
		chunks[0].SplitReason = "unknown"
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, externalChunks(chunks))
		require.ErrorIs(t, freezeErr, ErrInvalidTransition)
	})
	t.Run("Batch chunk ordinals are not ordered", func(t *testing.T) {
		chunks := []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()}
		chunks[0].Ordinals = []int{0, 0}
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, externalChunks(chunks))
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("Batch chunk contains a non-pending item", func(t *testing.T) {
		chunks := []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()}
		chunks[0].Ordinals = []int{5}
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, externalChunks(chunks))
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("Batch item appears in multiple chunks", func(t *testing.T) {
		first, second := preparedDeckSingleChunkPlan(), preparedDeckSingleChunkPlan()
		second.ChunkIndex = 1
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, externalChunks([]PreparedDeckBatchChunkPlan{first, second}))
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
	t.Run("Batch chunk identity is duplicated", func(t *testing.T) {
		first, second := preparedDeckSingleChunkPlan(), preparedDeckSingleChunkPlan()
		second.Ordinals = []int{1}
		_, freezeErr := attemptPreparedDeckFreeze(t, ctx, store, externalChunks([]PreparedDeckBatchChunkPlan{first, second}))
		require.ErrorIs(t, freezeErr, ErrImmutable)
	})
}

// TestPreparedDeckRunProgressReportsIdentityAndAge pins the public progress
// projection: an unknown run is not found, a fresh run reports its counts, and a
// submitted chunk reports an age.
func TestPreparedDeckRunProgressReportsIdentityAndAge(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	fixture := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"}, []cardexport.Entry{preparedDeckFreezeEntry()})
	params := FreezePreparedDeckRunParams{OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, Projection: fixture.projection, Config: preparedDeckExternalConfig(),
		Chunks: []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()}}
	frozen := freezePreparedDeckRun(t, ctx, store, params, 9401)

	t.Run("unknown run is not found", func(t *testing.T) {
		_, progressErr := store.PreparedDeckRunProgress(ctx, fixture.ownerID, fixture.preparation.ID, uuid.NewString())
		require.ErrorIs(t, progressErr, ErrNotFound)
	})

	// The submitted probe needs the pre-submission state, so these two steps stay
	// one sequence.
	progress, err := store.PreparedDeckRunProgress(ctx, fixture.ownerID, fixture.preparation.ID, frozen.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, progress.CandidateCount)
	assert.Equal(t, 1, progress.PendingCount)
	assert.Zero(t, progress.BatchAge)

	submissionToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckBatchSubmission(ctx, fixture.ownerID, fixture.preparation.ID, frozen.Run.ID, frozen.Chunks[0].ID, 1, submissionToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.RecordPreparedDeckBatchSubmitted(ctx, fixture.ownerID, fixture.preparation.ID, frozen.Run.ID, frozen.Chunks[0].ID, 1, submissionToken, "progress-input", "progress-batch", time.Now().UTC())
	require.NoError(t, err)
	progress, err = store.PreparedDeckRunProgress(ctx, fixture.ownerID, fixture.preparation.ID, frozen.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, progress.BatchSubmittedChunks)
	assert.Positive(t, progress.BatchAge)
}

// TestLoadPreparedDeckFinalizationRejectsUnfinalizableRun pins the guard that
// only a finalizing or completed run with completed translation may load.
func TestLoadPreparedDeckFinalizationRejectsUnfinalizableRun(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	fixture := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"}, []cardexport.Entry{preparedDeckFreezeEntry()})
	params := FreezePreparedDeckRunParams{OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, Projection: fixture.projection, Config: preparedDeckExternalConfig(),
		Chunks: []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()}}
	frozen := freezePreparedDeckRun(t, ctx, store, params, 0)

	t.Run("unknown run is not found", func(t *testing.T) {
		_, _, loadErr := store.LoadPreparedDeckFinalization(ctx, fixture.ownerID, fixture.preparation.ID, uuid.NewString())
		require.ErrorIs(t, loadErr, ErrNotFound)
	})
	t.Run("translating run cannot finalize", func(t *testing.T) {
		_, _, loadErr := store.LoadPreparedDeckFinalization(ctx, fixture.ownerID, fixture.preparation.ID, frozen.Run.ID)
		require.ErrorIs(t, loadErr, ErrInvalidTransition)
	})
}

// TestCancelCurrentPreparedDeckRunOutcomes pins cancellation as idempotent and
// invalid for a preparation that is not queued or preparing.
func TestCancelCurrentPreparedDeckRunOutcomes(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))

	t.Run("cancellation is idempotent", func(t *testing.T) {
		fixture := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{}, []cardexport.Entry{preparedDeckFreezeEntry()})
		cancelled, err := store.CancelCurrentPreparedDeckRun(ctx, fixture.ownerID, fixture.preparation.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.DeckPreparationCancelled, cancelled.State)
		again, err := store.CancelCurrentPreparedDeckRun(ctx, fixture.ownerID, fixture.preparation.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.DeckPreparationCancelled, again.State)
	})
	t.Run("a settled preparation cannot be cancelled", func(t *testing.T) {
		fixture := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{}, []cardexport.Entry{preparedDeckFreezeEntry()})
		_, err := store.Pool().Exec(ctx, `UPDATE deck_preparations SET state='failed' WHERE owner_id=$1 AND id=$2`, fixture.ownerID, fixture.preparation.ID)
		require.NoError(t, err)
		_, err = store.CancelCurrentPreparedDeckRun(ctx, fixture.ownerID, fixture.preparation.ID)
		require.ErrorIs(t, err, ErrInvalidTransition)
	})
	t.Run("an unknown preparation is not found", func(t *testing.T) {
		fixture := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{}, []cardexport.Entry{preparedDeckFreezeEntry()})
		_, err := store.CancelCurrentPreparedDeckRun(ctx, fixture.ownerID, uuid.NewString())
		require.ErrorIs(t, err, ErrNotFound)
	})
}

// TestReconcilePreparedDeckBatchRejectsInvalidItems pins the per-item rules that
// run once a chunk is fenced for reconciliation. The reconciliation rolls back on
// each rejection, so one claimed chunk drives every case.
func TestReconcilePreparedDeckBatchRejectsInvalidItems(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	fixture := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"}, []cardexport.Entry{preparedDeckFreezeEntry()})
	config := preparedDeckExternalConfig()
	config.MaxBatchGenerations = 1
	config.MaxProviderAttempts = 1
	params := FreezePreparedDeckRunParams{OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, Projection: fixture.projection, Config: config,
		Chunks: []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()}}
	frozen := freezePreparedDeckRun(t, ctx, store, params, 9501)
	chunk := frozen.Chunks[0]

	submissionToken := uuid.NewString()
	_, err := store.ClaimPreparedDeckBatchSubmission(ctx, fixture.ownerID, fixture.preparation.ID, frozen.Run.ID, chunk.ID, chunk.Generation, submissionToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.RecordPreparedDeckBatchSubmitted(ctx, fixture.ownerID, fixture.preparation.ID, frozen.Run.ID, chunk.ID, chunk.Generation, submissionToken, "item-input", "item-batch", time.Now().UTC())
	require.NoError(t, err)
	reconcileToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckBatchReconciliation(ctx, fixture.ownerID, fixture.preparation.ID, frozen.Run.ID, chunk.ID, 0, reconcileToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)

	base := func() PreparedDeckBatchReconcileParams {
		return PreparedDeckBatchReconcileParams{
			OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, RunID: frozen.Run.ID, ChunkID: chunk.ID, ClaimToken: reconcileToken,
			Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed", CompletedCount: 1},
			Items: []PreparedDeckBatchItemReconciliation{{Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted}},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*PreparedDeckBatchReconcileParams)
		wantErr error
	}{
		{name: "item state is not reconcilable", mutate: func(p *PreparedDeckBatchReconcileParams) { p.Items[0].State = domain.PreparedDeckOutcomeRunning }, wantErr: ErrInvalidTransition},
		{name: "failed item has no error class", mutate: func(p *PreparedDeckBatchReconcileParams) {
			p.Items[0].State = domain.PreparedDeckOutcomeFailed
			p.Chunk.CompletedCount = 0
			p.Chunk.FailedCount = 1
		}, wantErr: ErrInvalidTransition},
		{name: "omission carries an error class", mutate: func(p *PreparedDeckBatchReconcileParams) {
			p.Items[0].OmissionReason, p.Items[0].ErrorClass = "too short", "validation"
		}, wantErr: ErrInvalidTransition},
		{name: "ordinal is not a chunk member", mutate: func(p *PreparedDeckBatchReconcileParams) { p.Items[0].Ordinal = 99 }, wantErr: ErrPreparedDeckIdentity},
		{name: "pending item exceeds the generation budget", mutate: func(p *PreparedDeckBatchReconcileParams) {
			p.Items[0].State = domain.PreparedDeckOutcomePending
			p.Chunk.CompletedCount = 0
		}, wantErr: ErrInvalidTransition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reconcile := base()
			test.mutate(&reconcile)
			_, err := store.ReconcilePreparedDeckBatch(ctx, reconcile, nil, nil)
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}
