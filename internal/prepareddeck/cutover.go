package prepareddeck

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
)

type builder interface {
	PrepareCoverage(context.Context, string, string) (cardexport.Manifest, error)
	RenderManifest(context.Context, cardexport.Manifest, []cardexport.ExactEnrichment) (cardexport.Artifact, error)
}

type scopedBuilder interface {
	PrepareCoverageForAnalysis(context.Context, string, string) (cardexport.Manifest, error)
}

// buildManifest freezes selection and rendering inputs before the durable run
// is written. The durable persistence layer then stores this snapshot and all
// subsequent work uses it rather than re-running selection.
func buildManifest(ctx context.Context, b builder, owner, sourceID, analysisRunID string) (cardexport.Manifest, error) {
	if b == nil {
		return cardexport.Manifest{}, errors.New("prepareddeck: deck builder is unavailable")
	}
	if analysisRunID != "" {
		scoped, ok := b.(scopedBuilder)
		if !ok {
			return cardexport.Manifest{}, errors.New("prepareddeck: scoped analysis deck builder is unavailable")
		}
		return scoped.PrepareCoverageForAnalysis(ctx, owner, analysisRunID)
	}
	return b.PrepareCoverage(ctx, owner, sourceID)
}

// BatchPlanner turns the existing deterministic card-export manifest into a
// durable Batch run. It never invokes a provider; the provider boundary is
// crossed only by BatchSubmitWorker after the run and chunk identities commit.
type BatchPlanner struct {
	Builder         builder
	Codec           *enrichment.TranslationCodec
	ExternalEnabled bool
	BatchConfig     BatchConfig
}

func NewBatchPlanner(builder builder, codec *enrichment.TranslationCodec, externalEnabled bool, batchConfig BatchConfig) *BatchPlanner {
	return &BatchPlanner{Builder: builder, Codec: codec, ExternalEnabled: externalEnabled, BatchConfig: batchConfig}
}

func (p *BatchPlanner) PlanPreparedDeckRun(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, consent bool) (persistence.FreezePreparedDeckRunParams, error) {
	if p == nil || p.Builder == nil || strings.TrimSpace(preparation.OwnerID) == "" || strings.TrimSpace(preparation.ID) == "" {
		return persistence.FreezePreparedDeckRunParams{}, ErrInvalidInput
	}
	manifest, err := buildManifest(ctx, p.Builder, preparation.OwnerID, preparation.SourceMaterialID, preparation.AnalysisRunID)
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("prepare deck manifest: %w", err)
	}

	config := persistence.PreparedDeckRunConfig{
		ExternalTranslationConsent:    consent,
		ExternalTranslationConfigured: consent && p.ExternalEnabled,
		BatchMaxRequests:              p.BatchConfig.MaxRequests,
		BatchMaxBytes:                 persistence.DefaultBatchMaxBytes,
	}
	runID := uuid.NewString()
	if !config.ExternalTranslationConfigured {
		return persistence.FreezePreparedDeckRunParams{RunID: runID, Manifest: manifest.Snapshot(), Config: config}, nil
	}
	if p.Codec == nil {
		return persistence.FreezePreparedDeckRunParams{}, errors.New("prepareddeck: external translation requires an eligible OpenAI Batch endpoint")
	}
	config.ContextMode = string(enrichment.SentenceContext)
	config.Provider = p.Codec.ProviderName()
	config.ProviderVersion = p.Codec.ProviderVersion()
	config.Endpoint = enrichment.OpenAIChatCompletionsEndpoint
	config.Model = p.Codec.Model()

	candidates := manifest.EnrichmentCandidates()
	keys := make([]enrichment.CacheKey, len(candidates))
	for i, candidate := range candidates {
		keys[i] = enrichment.CacheKey{
			Language:        candidate.Language,
			CanonicalLemma:  candidate.CanonicalLemma,
			UPOS:            strings.ToUpper(candidate.UPOS),
			Provider:        config.Provider,
			ProviderVersion: config.ProviderVersion,
			SentenceHash:    enrichment.SentenceHash(candidate.ExampleSentence),
		}
	}
	manifest, err = manifest.BindCacheKeys(keys)
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("bind prepared deck cache identity: %w", err)
	}
	items, err := batchItems(ctx, tx, manifest.Snapshot())
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("find prepared deck cache misses: %w", err)
	}
	var chunks []persistence.PreparedDeckBatchChunkPlan
	if len(items) > 0 {
		chunks, err = PlanBatchChunks(p.Codec, runID, 1, config.Model, config.Endpoint, items, BatchChunkLimits{MaxRequests: config.BatchMaxRequests, MaxBytes: config.BatchMaxBytes})
		if err != nil {
			return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("plan prepared deck Batch chunks: %w", err)
		}
	}
	return persistence.FreezePreparedDeckRunParams{RunID: runID, Manifest: manifest.Snapshot(), Config: config, Chunks: chunks}, nil
}

func batchItems(ctx context.Context, tx pgx.Tx, snapshot cardexport.ManifestSnapshot) ([]enrichment.BatchTranslationItem, error) {
	items := make([]enrichment.BatchTranslationItem, 0)
	for _, item := range snapshot.Items {
		if item.Disposition != cardexport.ManifestAccepted || item.CacheKey == nil {
			continue
		}
		if tx != nil {
			var found bool
			key := item.CacheKey
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM enrichment_cache WHERE language=$1 AND canonical_lemma=$2 AND upos=$3 AND provider=$4 AND provider_version=$5 AND sentence_hash=$6)`, key.Language, key.CanonicalLemma, key.UPOS, key.Provider, key.ProviderVersion, key.SentenceHash).Scan(&found); err != nil {
				return nil, err
			}
			if found {
				continue
			}
		}
		items = append(items, enrichment.BatchTranslationItem{Ordinal: item.Ordinal, Request: enrichment.TranslationRequest{
			Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS,
			TargetWord: item.Entry.TargetWord, ExampleSentence: item.Entry.Sentence,
		}})
	}
	return items, nil
}

// Worker owns only the short freeze operation. Provider calls are performed
// later by the durable Batch workers.
type Worker struct {
	river.WorkerDefaults[JobArgs]
	Coordinator *DurableCoordinator
	Store       *persistence.PostgresStore
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if w == nil || w.Coordinator == nil || w.Store == nil || job == nil {
		return ErrInvalidInput
	}
	a := job.Args
	preparation, err := w.Store.ClaimDeckPreparation(ctx, a.OwnerID, a.PreparationID)
	if errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.AnalysisRunID == "" {
		source, sourceErr := w.Store.GetSourceMaterial(ctx, a.OwnerID, a.SourceMaterialID)
		if sourceErr != nil {
			return w.fail(ctx, a, fmt.Errorf("load source material: %w", sourceErr))
		}
		if source.ContentHash != a.ContentHash {
			return w.fail(ctx, a, errors.New("source material identity changed"))
		}
	}
	if preparation.SourceMaterialID != a.SourceMaterialID || preparation.ContentHash != a.ContentHash || preparation.AnalysisRunID != a.AnalysisRunID {
		return w.fail(ctx, a, errors.New("source material identity changed"))
	}
	_, err = w.Coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: a.OwnerID, PreparationID: a.PreparationID, ExternalTranslationConsent: a.ExternalTranslationConsent})
	if err == nil || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	return w.fail(ctx, a, err)
}

func (w *Worker) fail(ctx context.Context, args JobArgs, cause error) error {
	_, failErr := w.Store.FailDeckPreparation(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, cause.Error())
	if failErr != nil && !errors.Is(failErr, persistence.ErrInvalidTransition) {
		return fmt.Errorf("prepare durable run: %v; mark preparation failed: %w", cause, failErr)
	}
	return nil
}

func AddBatchWorker(workers *river.Workers, store *persistence.PostgresStore, export *cardexport.Service, client riverClient, codec *enrichment.TranslationCodec, batchConfig BatchConfig, externalEnabled bool) {
	planner := NewBatchPlanner(export, codec, externalEnabled, batchConfig)
	river.AddWorker(workers, &Worker{Coordinator: NewDurableCoordinator(store, client, planner), Store: store})
}
