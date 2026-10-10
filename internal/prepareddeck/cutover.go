package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/riverqueue/river"
)

type inputAssembler interface {
	AssemblePreparedDeckInputs(context.Context, pgx.Tx, domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error)
}

type preparedDeckFactStore interface {
	LoadPreparedDeckInputFactsTx(context.Context, pgx.Tx, domain.DeckPreparation) (persistence.PreparedDeckInputFacts, error)
	LoadPreparedDeckCandidateFactsTx(context.Context, pgx.Tx, domain.DeckPreparation, []domain.SelectionCandidate) ([]persistence.PreparedDeckCandidateFacts, error)
}

// InputAssembler applies recurring-vocabulary selection to transaction-scoped
// persistence facts and returns the projections consumed by Presentation.
type InputAssembler struct {
	Store preparedDeckFactStore
}

func NewInputAssembler(store preparedDeckFactStore) *InputAssembler {
	return &InputAssembler{Store: store}
}

func (a *InputAssembler) AssemblePreparedDeckInputs(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	if a == nil || a.Store == nil {
		return nil, "", errors.New("prepareddeck: input fact store is unavailable")
	}
	if preparation.SnapshotID == "" && tx != nil {
		var bookID *string
		if err := tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, preparation.OwnerID, preparation.SourceMaterialID).Scan(&bookID); err != nil {
			return nil, "", fmt.Errorf("resolve direct-deck Book identity: %w", err)
		}
		if bookID != nil {
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, preparation.OwnerID+":"+*bookID); err != nil {
				return nil, "", fmt.Errorf("lock direct-deck vocabulary decisions: %w", err)
			}
		}
	}
	facts, err := a.Store.LoadPreparedDeckInputFactsTx(ctx, tx, preparation)
	if err != nil {
		return nil, "", fmt.Errorf("load prepared deck input facts: %w", err)
	}
	if !facts.CurrentReadingSnapshotActive && facts.Analysis != nil {
		facts.Candidates, err = projectPreparedDeckCandidates(preparation, facts.CorpusID, *facts.Analysis, facts.Corrections)
		if err != nil {
			return nil, "", err
		}
	}
	selected := make([]domain.SelectionCandidate, 0, len(facts.Candidates))
	if preparation.SnapshotID != "" && !facts.CurrentReadingSnapshotActive {
		return nil, "", fmt.Errorf("prepareddeck: Current reading snapshot %q is unavailable", preparation.SnapshotID)
	}
	if facts.CurrentReadingSnapshotActive {
		selected = append(selected, facts.CurrentReadingSnapshot...)
	} else {
		eligibility := selection.NewEligibility(facts.Known, facts.Reserved)
		for _, candidate := range facts.Candidates {
			if !eligibility.Allows(candidate, selection.DefaultRecurringMinOccurrences) {
				continue
			}
			selected = append(selected, candidate)
		}
	}
	selectedFacts, err := a.Store.LoadPreparedDeckCandidateFactsTx(ctx, tx, preparation, selected)
	if err != nil {
		return nil, "", fmt.Errorf("load selected prepared deck facts: %w", err)
	}

	projections := make([]cardexport.CandidateProjection, 0, len(selectedFacts))
	for _, fact := range selectedFacts {
		projections = append(projections, cardexport.CandidateProjection{OwnerID: preparation.OwnerID, DeckName: facts.DeckName, Candidate: fact.Candidate, Entry: fact.Entry, Sentences: fact.Sentences, RequireContextualGloss: false})
	}
	return projections, facts.DeckName, nil
}

func projectPreparedDeckCandidates(preparation domain.DeckPreparation, corpusID string, corpus analyzer.Result, corrections []domain.OccurrenceLemmaCorrection) ([]domain.SelectionCandidate, error) {
	decisions := make([]selection.OccurrenceDecision, 0, len(corrections))
	for _, correction := range corrections {
		if correction.StartOffset < 0 || correction.EndOffset < 0 {
			return nil, errors.New("prepareddeck: correction has invalid source offsets")
		}
		decisions = append(decisions, selection.OccurrenceDecision{
			Occurrence: selection.OccurrenceIdentity{SourceDocumentID: correction.SourceDocumentID, StartOffset: uint64(correction.StartOffset), EndOffset: uint64(correction.EndOffset)},
			Lemma:      correction.CanonicalLemma,
			Excluded:   correction.Excluded,
		})
	}
	projected, err := selection.Project(corpus, selection.DefaultConfig(corpusID), decisions)
	if err != nil {
		return nil, fmt.Errorf("project effective prepared-deck vocabulary: %w", err)
	}
	result := make([]domain.SelectionCandidate, 0, len(projected))
	for _, candidate := range projected {
		forms, marshalErr := json.Marshal(candidate.ObservedForms)
		if marshalErr != nil {
			return nil, marshalErr
		}
		references, marshalErr := json.Marshal(candidate.SentenceReferences)
		if marshalErr != nil {
			return nil, marshalErr
		}
		provenance, marshalErr := json.Marshal(candidate.Provenance)
		if marshalErr != nil {
			return nil, marshalErr
		}
		firstEncounter := int64(^uint64(0) >> 1)
		for _, reference := range candidate.SentenceReferences {
			startOffset, convertErr := checked.Int64FromUint64(reference.Location.StartOffset)
			if convertErr != nil {
				return nil, fmt.Errorf("prepareddeck: source offset exceeds PostgreSQL range: %w", convertErr)
			}
			if startOffset < firstEncounter {
				firstEncounter = startOffset
			}
		}
		result = append(result, domain.SelectionCandidate{
			OwnerID: preparation.OwnerID, CorpusID: corpusID,
			Language: candidate.Identity.Language, CanonicalLemma: candidate.Identity.CanonicalLemma, UPOS: candidate.Identity.UPOS,
			OccurrenceCount: candidate.OccurrenceCount, FirstEncounter: firstEncounter,
			ObservedForms: forms, SentenceReferences: references, Provenance: provenance,
		})
	}
	return result, nil
}

// BatchPlanner freezes transaction-scoped input facts into a durable run. It
// never invokes an external provider; that boundary is crossed only after the
// run and its chunk identities commit.
type BatchPlanner struct {
	Assembler       inputAssembler
	Presentation    *cardexport.Presentation
	Codec           *enrichment.TranslationCodec
	ExternalEnabled bool
	BatchConfig     BatchConfig
}

// StandardPlanner freezes the same manifest and cache identity as BatchPlanner
// but leaves provider work as one durable scalar job per miss.
type StandardPlanner struct {
	*BatchPlanner
	StandardConfig PreparedDeckConfig
}

func NewStandardPlanner(assembler inputAssembler, presentation *cardexport.Presentation, codec *enrichment.TranslationCodec, externalEnabled bool, batchConfig BatchConfig, standardConfig PreparedDeckConfig) *StandardPlanner {
	if standardConfig.StandardMaxAttempts == 0 {
		standardConfig.StandardMaxAttempts = DefaultStandardMaxAttempts
	}
	return &StandardPlanner{BatchPlanner: NewBatchPlanner(assembler, presentation, codec, externalEnabled, batchConfig), StandardConfig: standardConfig}
}

// PreparedDeckPlanner selects the executor once, when a new run is frozen.
// DurableCoordinator intentionally bypasses it for an existing run.
func NewPreparedDeckPlanner(assembler inputAssembler, presentation *cardexport.Presentation, codec *enrichment.TranslationCodec, externalEnabled bool, batchConfig BatchConfig, config PreparedDeckConfig) DurableRunPlanner {
	if config.TranslationMode == "batch" {
		return NewBatchPlanner(assembler, presentation, codec, externalEnabled, batchConfig)
	}
	return NewStandardPlanner(assembler, presentation, codec, externalEnabled, batchConfig, config)
}

func NewBatchPlanner(assembler inputAssembler, presentation *cardexport.Presentation, codec *enrichment.TranslationCodec, externalEnabled bool, batchConfig BatchConfig) *BatchPlanner {
	return &BatchPlanner{Assembler: assembler, Presentation: presentation, Codec: codec, ExternalEnabled: externalEnabled, BatchConfig: batchConfig}
}

func (p *BatchPlanner) PlanPreparedDeckRun(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) (persistence.FreezePreparedDeckRunParams, error) {
	return p.planPreparedDeckRun(ctx, tx, preparation, domain.PreparedDeckExecutionBatch, 0)
}

func (p *StandardPlanner) PlanPreparedDeckRun(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) (persistence.FreezePreparedDeckRunParams, error) {
	if p == nil || p.BatchPlanner == nil {
		return persistence.FreezePreparedDeckRunParams{}, ErrInvalidInput
	}
	return p.BatchPlanner.planPreparedDeckRun(ctx, tx, preparation, domain.PreparedDeckExecutionStandard, p.StandardConfig.StandardMaxAttempts)
}

func (p *BatchPlanner) planPreparedDeckRun(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, mode domain.PreparedDeckExecutionMode, standardAttempts int) (persistence.FreezePreparedDeckRunParams, error) {
	if p == nil || p.Assembler == nil || p.Presentation == nil || strings.TrimSpace(preparation.OwnerID) == "" || strings.TrimSpace(preparation.ID) == "" {
		return persistence.FreezePreparedDeckRunParams{}, ErrInvalidInput
	}
	if !p.ExternalEnabled || p.Codec == nil {
		return persistence.FreezePreparedDeckRunParams{}, errors.New("prepareddeck: a configured translation provider is required for contextual Glosses")
	}
	projections, deckName, err := p.Assembler.AssemblePreparedDeckInputs(ctx, tx, preparation)
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("assemble prepared deck inputs: %w", err)
	}

	config := persistence.PreparedDeckRunConfig{
		ExternalTranslationConsent:    true,
		ExternalTranslationConfigured: true,
		ExecutionMode:                 string(mode),
		TargetLanguage:                "en",
		BatchMaxRequests:              p.BatchConfig.MaxRequests,
		BatchMaxBytes:                 persistence.DefaultBatchMaxBytes,
	}
	if mode == domain.PreparedDeckExecutionStandard && standardAttempts > 0 {
		config.MaxProviderAttempts = standardAttempts
	}
	runID := uuid.NewString()
	config.ContextMode = string(enrichment.SentenceContext)
	config.Provider = p.Codec.ProviderName()
	config.ProviderVersion = p.Codec.ContextualGlossProviderVersion()
	config.Endpoint = enrichment.OpenAIChatCompletionsEndpoint
	config.Model = p.Codec.Model()
	for i := range projections {
		projections[i].Provider = config.Provider
		projections[i].ProviderVersion = config.ProviderVersion
		projections[i].TargetLanguage = config.TargetLanguage
		projections[i].RequireContextualGloss = true
	}
	deck, freezeDiagnostics, err := p.Presentation.Freeze(ctx, preparation.OwnerID, deckName, projections)
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("freeze prepared deck presentation: %w", err)
	}
	logGlossCoverage(freezeDiagnostics.GlossCoverage)
	work := deck.WorkProjection()
	work, err = pendingBatchWork(ctx, tx, work)
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("find prepared deck cache misses: %w", err)
	}
	var chunks []persistence.PreparedDeckBatchChunkPlan
	if mode == domain.PreparedDeckExecutionBatch && len(work) > 0 {
		chunks, err = PlanBatchChunks(p.Codec, runID, 1, config.Model, config.Endpoint, work, BatchChunkLimits{MaxRequests: config.BatchMaxRequests, MaxBytes: config.BatchMaxBytes})
		if err != nil {
			return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("plan prepared deck Batch chunks: %w", err)
		}
	}
	return persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: deck.StorageProjection(), Config: config, Chunks: chunks}, nil
}

type glossCoverageEvent struct {
	Event  string                     `json:"event"`
	Groups []cardexport.GlossCoverage `json:"groups"`
}

func logGlossCoverage(groups []cardexport.GlossCoverage) {
	payload, err := json.Marshal(glossCoverageEvent{Event: "gloss_coverage", Groups: groups})
	if err == nil {
		log.Printf("gloss_coverage %s", payload)
	}
}

func pendingBatchWork(ctx context.Context, tx pgx.Tx, work []cardexport.WorkItem) ([]cardexport.WorkItem, error) {
	pending := make([]cardexport.WorkItem, 0, len(work))
	for _, item := range work {
		if tx != nil {
			var found bool
			key := item.CacheKey
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM enrichment_cache WHERE language=$1 AND target_language=$2 AND canonical_lemma=$3 AND upos=$4 AND provider=$5 AND provider_version=$6 AND sentence_hash=$7 AND dictionary_provider_version=$8 AND meaning_evidence_hash=$9 AND translation <> '' AND (sentence_hash = '' OR sentence_translation <> ''))`, key.Language, key.TargetLanguage, key.CanonicalLemma, key.UPOS, key.Provider, key.ProviderVersion, key.SentenceHash, key.DictionaryProviderVersion, key.MeaningEvidenceHash).Scan(&found); err != nil {
				return nil, err
			}
			if found {
				continue
			}
		}
		pending = append(pending, item)
	}
	return pending, nil
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
	_, err = w.Coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: a.OwnerID, PreparationID: a.PreparationID})
	if err == nil || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	return w.fail(ctx, a, err)
}

func (w *Worker) fail(ctx context.Context, args JobArgs, cause error) error {
	_, failErr := w.Store.FailDeckPreparation(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, cause.Error())
	if failErr != nil && !errors.Is(failErr, persistence.ErrInvalidTransition) {
		return fmt.Errorf("prepare durable run: %w; mark preparation failed: %w", cause, failErr)
	}
	return nil
}

func AddPreparedDeckWorker(workers *river.Workers, store *persistence.PostgresStore, presentation *cardexport.Presentation, client riverClient, codec *enrichment.TranslationCodec, batchConfig BatchConfig, preparedConfig PreparedDeckConfig, externalEnabled bool) {
	planner := NewPreparedDeckPlanner(NewInputAssembler(store), presentation, codec, externalEnabled, batchConfig, preparedConfig)
	river.AddWorker(workers, &Worker{Coordinator: NewDurableCoordinator(store, client, planner), Store: store})
}
