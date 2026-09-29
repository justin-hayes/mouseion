package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

type PreparedDeckBatchItemReconciliation struct {
	Ordinal        int
	State          domain.PreparedDeckOutcomeState
	CacheEntry     *enrichment.CacheEntry
	ErrorClass     string
	ErrorCode      string
	OmissionReason string
}

type PreparedDeckBatchReconcileParams struct {
	OwnerID, PreparationID, RunID, ChunkID string
	ReconciliationGeneration               int
	ClaimToken                             string
	Chunk                                  PreparedDeckBatchReconciliationUpdate
	Items                                  []PreparedDeckBatchItemReconciliation
	RetryChunks                            []PreparedDeckBatchChunkPlan
}

type PreparedDeckBatchReconcileResult struct {
	Chunk       domain.PreparedDeckBatchChunk
	Run         domain.PreparedDeckRun
	RetryChunks []domain.PreparedDeckBatchChunk
}

// PreparedDeckBatchSubmissionJobInserter inserts one retry-generation job in
// the same transaction that persists the retry selection.
type PreparedDeckBatchSubmissionJobInserter func(context.Context, pgx.Tx, domain.PreparedDeckBatchChunk) (int64, error)

type preparedDeckBatchMember struct {
	Ordinal int
	Key     enrichment.CacheKey
}

// ReconcilePreparedDeckBatch atomically applies one fully validated terminal
// provider result. Exact cache rows, fenced item outcomes, retry chunks, the
// terminal chunk, and a possible finalizer job either all commit or all roll
// back together.
func (s *PostgresStore) ReconcilePreparedDeckBatch(ctx context.Context, params PreparedDeckBatchReconcileParams, insertSubmission PreparedDeckBatchSubmissionJobInserter, insertFinalizer PreparedDeckRunJobInserter) (result PreparedDeckBatchReconcileResult, err error) {
	if err := validatePreparedDeckBatchReconcileParams(s, params); err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	run, chunk, err := lockPreparedDeckBatchForReconciliation(ctx, tx, params)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	retryOrdinals, err := reconcilePreparedDeckBatchOutcomes(ctx, tx, params, run, chunk)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}

	retryChunks, err := insertPreparedDeckRetryChunks(ctx, tx, params, run, chunk.Generation+1, retryOrdinals, insertSubmission)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	chunk, err = completePreparedDeckBatchChunk(ctx, tx, params)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}

	run, err = advancePreparedDeckRunTx(ctx, tx, run, insertFinalizer)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	return PreparedDeckBatchReconcileResult{Chunk: chunk, Run: run, RetryChunks: retryChunks}, nil
}

func validatePreparedDeckBatchReconcileParams(s *PostgresStore, params PreparedDeckBatchReconcileParams) error {
	if s == nil || strings.TrimSpace(params.OwnerID) == "" || strings.TrimSpace(params.PreparationID) == "" || strings.TrimSpace(params.RunID) == "" || strings.TrimSpace(params.ChunkID) == "" || strings.TrimSpace(params.ClaimToken) == "" {
		return ErrInvalidTransition
	}
	if params.Chunk.State != domain.PreparedDeckBatchCompleted || !terminalProviderStatus(params.Chunk.ProviderStatus) {
		return ErrInvalidTransition
	}
	if params.Chunk.CompletedCount < 0 || params.Chunk.FailedCount < 0 || params.Chunk.ExpiredCount < 0 || params.Chunk.InputTokens < 0 || params.Chunk.OutputTokens < 0 {
		return ErrInvalidTransition
	}
	return validateBoundedError(params.Chunk.ErrorClass, params.Chunk.ErrorCode)
}

func lockPreparedDeckBatchForReconciliation(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams) (domain.PreparedDeckRun, domain.PreparedDeckBatchChunk, error) {
	runModel, err := sqlcgen.New(tx).GetPreparedDeckRunForUpdate(ctx, sqlcgen.GetPreparedDeckRunForUpdateParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, ID: params.RunID})
	if err != nil {
		return domain.PreparedDeckRun{}, domain.PreparedDeckBatchChunk{}, missing(err)
	}
	run := preparedDeckRunFromModel(runModel)
	if run.State != domain.PreparedDeckRunTranslating || (run.TranslationState != domain.PreparedDeckTranslationPending && run.TranslationState != domain.PreparedDeckTranslationRunning) {
		return domain.PreparedDeckRun{}, domain.PreparedDeckBatchChunk{}, ErrPreparedDeckClaimLost
	}
	chunkModel, err := sqlcgen.New(tx).GetPreparedDeckBatchChunkForUpdate(ctx, sqlcgen.GetPreparedDeckBatchChunkForUpdateParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, ID: params.ChunkID})
	if err != nil {
		return domain.PreparedDeckRun{}, domain.PreparedDeckBatchChunk{}, missing(err)
	}
	chunk := preparedDeckBatchChunkFromModel(chunkModel)
	if chunk.State != domain.PreparedDeckBatchReconciling || chunk.ReconciliationGeneration != params.ReconciliationGeneration || chunk.ReconciliationClaimToken != params.ClaimToken {
		return domain.PreparedDeckRun{}, domain.PreparedDeckBatchChunk{}, ErrPreparedDeckClaimLost
	}
	return run, chunk, nil
}

func reconcilePreparedDeckBatchOutcomes(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams, run domain.PreparedDeckRun, chunk domain.PreparedDeckBatchChunk) (map[int]struct{}, error) {
	members, err := loadPreparedDeckBatchMembers(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	if len(members) != chunk.RequestCount || len(params.Items) != len(members) || params.Chunk.CompletedCount+params.Chunk.FailedCount+params.Chunk.ExpiredCount > chunk.RequestCount {
		return nil, ErrPreparedDeckIdentity
	}
	memberByOrdinal := make(map[int]preparedDeckBatchMember, len(members))
	for _, member := range members {
		memberByOrdinal[member.Ordinal] = member
	}
	seen := make(map[int]struct{}, len(params.Items))
	retryOrdinals := make(map[int]struct{})
	for _, item := range params.Items {
		member, ok := memberByOrdinal[item.Ordinal]
		if !ok {
			return nil, ErrPreparedDeckIdentity
		}
		if _, duplicate := seen[item.Ordinal]; duplicate {
			return nil, ErrPreparedDeckIdentity
		}
		seen[item.Ordinal] = struct{}{}
		retry, err := reconcilePreparedDeckBatchItem(ctx, tx, params, run, chunk, member, item)
		if err != nil {
			return nil, err
		}
		if retry {
			retryOrdinals[item.Ordinal] = struct{}{}
		}
	}
	if len(seen) != len(members) {
		return nil, ErrPreparedDeckIdentity
	}
	return retryOrdinals, nil
}

func reconcilePreparedDeckBatchItem(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams, run domain.PreparedDeckRun, chunk domain.PreparedDeckBatchChunk, member preparedDeckBatchMember, item PreparedDeckBatchItemReconciliation) (bool, error) {
	if item.State != domain.PreparedDeckOutcomeCompleted && item.State != domain.PreparedDeckOutcomePending && item.State != domain.PreparedDeckOutcomeFailed {
		return false, ErrInvalidTransition
	}
	if item.OmissionReason != "" && (item.State != domain.PreparedDeckOutcomeCompleted || strings.TrimSpace(item.OmissionReason) == "" || len([]rune(item.OmissionReason)) > 200 || strings.ContainsAny(item.OmissionReason, "<>") || item.ErrorClass != "" || item.ErrorCode != "" || item.CacheEntry != nil) {
		return false, ErrInvalidTransition
	}
	if item.State == domain.PreparedDeckOutcomeFailed && strings.TrimSpace(item.ErrorClass) == "" {
		return false, ErrInvalidTransition
	}
	if err := validateBoundedError(item.ErrorClass, item.ErrorCode); err != nil {
		return false, err
	}
	outcomeModel, err := sqlcgen.New(tx).GetPreparedDeckTranslationOutcomeForUpdate(ctx, sqlcgen.GetPreparedDeckTranslationOutcomeForUpdateParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, Ordinal: item.Ordinal})
	if err != nil {
		return false, missing(err)
	}
	outcome, err := preparedDeckOutcomeFromModel(outcomeModel)
	if err != nil {
		return false, err
	}
	if outcome.State == domain.PreparedDeckOutcomeCompleted || outcome.State == domain.PreparedDeckOutcomeFailed || outcome.State == domain.PreparedDeckOutcomeCancelled {
		return false, nil
	}
	if outcome.State != domain.PreparedDeckOutcomePending && outcome.State != domain.PreparedDeckOutcomeRunning {
		return false, ErrPreparedDeckIdentity
	}
	if item.State == domain.PreparedDeckOutcomePending && chunk.Generation >= run.MaxBatchGenerations {
		return false, ErrInvalidTransition
	}
	if err := persistPreparedDeckBatchCache(ctx, tx, member, item); err != nil {
		return false, err
	}
	var terminalAt *time.Time
	if item.State != domain.PreparedDeckOutcomePending {
		terminal := time.Now().UTC()
		terminalAt = &terminal
	}
	if err := updatePreparedDeckBatchOutcome(ctx, tx, params, chunk, item, terminalAt); err != nil {
		return false, err
	}
	return item.State == domain.PreparedDeckOutcomePending, nil
}

func persistPreparedDeckBatchCache(ctx context.Context, tx pgx.Tx, member preparedDeckBatchMember, item PreparedDeckBatchItemReconciliation) error {
	if item.State != domain.PreparedDeckOutcomeCompleted {
		if item.CacheEntry != nil {
			return ErrPreparedDeckIdentity
		}
		return nil
	}
	if item.OmissionReason != "" {
		return nil
	}
	if item.CacheEntry == nil || item.CacheEntry.CacheKey != member.Key || item.CacheEntry.CachedAt.IsZero() {
		return ErrPreparedDeckIdentity
	}
	entry := item.CacheEntry
	selection, err := marshalSenseSelection(entry.SenseSelection)
	if err != nil {
		return err
	}
	targets := append([]string{}, entry.SentenceTranslationTargets...)
	if err = sqlcgen.New(tx).UpsertEnrichmentCache(ctx, sqlcgen.UpsertEnrichmentCacheParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash, DictionaryProviderVersion: entry.DictionaryProviderVersion, MeaningEvidenceHash: entry.MeaningEvidenceHash, Translation: entry.Translation, FallbackGloss: entry.FallbackGloss, SenseSelection: selection, SentenceTranslation: entry.SentenceTranslation, SentenceTranslationTarget: entry.SentenceTranslationTarget, SentenceTranslationTargets: targets, CachedAt: entry.CachedAt}); err != nil {
		return err
	}
	// The cache is immutable and first-writer-wins. Read the committed row so
	// the finalizer consumes the trusted value when another run won the race.
	_, err = sqlcgen.New(tx).EnrichmentCacheLookup(ctx, sqlcgen.EnrichmentCacheLookupParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash, DictionaryProviderVersion: entry.DictionaryProviderVersion, MeaningEvidenceHash: entry.MeaningEvidenceHash})
	return err
}

func updatePreparedDeckBatchOutcome(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams, chunk domain.PreparedDeckBatchChunk, item PreparedDeckBatchItemReconciliation, terminalAt *time.Time) error {
	return sqlcgen.New(tx).UpdatePreparedDeckOutcomeFromBatch(ctx, sqlcgen.UpdatePreparedDeckOutcomeFromBatchParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, Ordinal: item.Ordinal, State: string(item.State), ProviderAttemptCount: chunk.Generation, TerminalAt: pgTimeArgPtr(terminalAt), ErrorClass: item.ErrorClass, ErrorCode: item.ErrorCode, OmissionReason: item.OmissionReason})
}

func completePreparedDeckBatchChunk(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams) (domain.PreparedDeckBatchChunk, error) {
	model, err := sqlcgen.New(tx).CompletePreparedDeckBatchChunk(ctx, sqlcgen.CompletePreparedDeckBatchChunkParams{Owner: params.OwnerID, Preparation: params.PreparationID, Run: params.RunID, ID: params.ChunkID, ProviderStatus: textArg(params.Chunk.ProviderStatus), OutputFileID: params.Chunk.OutputFileID, ErrorFileID: params.Chunk.ErrorFileID, CompletedCount: params.Chunk.CompletedCount, FailedCount: params.Chunk.FailedCount, ExpiredCount: params.Chunk.ExpiredCount, InputTokens: params.Chunk.InputTokens, OutputTokens: params.Chunk.OutputTokens, ErrorClass: params.Chunk.ErrorClass, ErrorCode: params.Chunk.ErrorCode, ProviderCompletedAt: pgTimeArgPtr(params.Chunk.ProviderCompletedAt)})
	if err != nil {
		return domain.PreparedDeckBatchChunk{}, missing(err)
	}
	return preparedDeckBatchChunkFromModel(model), nil
}

func loadPreparedDeckBatchMembers(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams) ([]preparedDeckBatchMember, error) {
	rows, err := sqlcgen.New(tx).ListPreparedDeckBatchChunkMembers(ctx, sqlcgen.ListPreparedDeckBatchChunkMembersParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, ChunkID: params.ChunkID})
	if err != nil {
		return nil, err
	}
	var members []preparedDeckBatchMember
	for _, row := range rows {
		member := preparedDeckBatchMember{Ordinal: row.Ordinal, Key: enrichment.CacheKey{Language: row.Language, TargetLanguage: row.TargetLanguage, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, Provider: row.Provider, ProviderVersion: row.ProviderVersion, DictionaryProviderVersion: row.DictionaryProviderVersion, SentenceHash: row.SentenceHash, MeaningEvidenceHash: row.MeaningEvidenceHash}}
		if member.Key.Provider == "" || member.Key.ProviderVersion == "" || member.Key.TargetLanguage == "" {
			return nil, ErrPreparedDeckIdentity
		}
		members = append(members, member)
	}
	return members, nil
}

func insertPreparedDeckRetryChunks(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams, run domain.PreparedDeckRun, generation int, retryOrdinals map[int]struct{}, insertSubmission PreparedDeckBatchSubmissionJobInserter) ([]domain.PreparedDeckBatchChunk, error) {
	planned := make(map[int]struct{}, len(retryOrdinals))
	for _, plan := range params.RetryChunks {
		if plan.Generation != generation || generation > run.MaxBatchGenerations || plan.Model != run.Model || plan.Endpoint != run.Endpoint || plan.SplitReason != "retry" || len(plan.Ordinals) == 0 || len(plan.Ordinals) > run.BatchMaxRequests || plan.InputBytes < 1 || plan.InputBytes > run.BatchMaxBytes {
			return nil, ErrInvalidTransition
		}
		for _, ordinal := range plan.Ordinals {
			if _, ok := retryOrdinals[ordinal]; !ok {
				return nil, ErrPreparedDeckIdentity
			}
			if _, duplicate := planned[ordinal]; duplicate {
				return nil, ErrPreparedDeckIdentity
			}
			planned[ordinal] = struct{}{}
		}
	}
	if len(planned) != len(retryOrdinals) || (len(retryOrdinals) > 0 && insertSubmission == nil) {
		return nil, ErrInvalidTransition
	}
	nextIndex32, err := sqlcgen.New(tx).NextPreparedDeckBatchChunkIndex(ctx, sqlcgen.NextPreparedDeckBatchChunkIndexParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, Generation: generation})
	if err != nil {
		return nil, err
	}
	nextIndex := int(nextIndex32)
	chunks := make([]domain.PreparedDeckBatchChunk, 0, len(params.RetryChunks))
	for _, plan := range params.RetryChunks {
		chunkID := plan.ID
		if chunkID == "" {
			chunkID = uuid.NewString()
		}
		first, last := plan.Ordinals[0], plan.Ordinals[len(plan.Ordinals)-1]
		chunkIndex := nextIndex + len(chunks)
		if err := sqlcgen.New(tx).InsertPreparedDeckBatchChunk(ctx, sqlcgen.InsertPreparedDeckBatchChunkParams{ID: chunkID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, ChunkIndex: chunkIndex, Generation: generation, Model: plan.Model, Endpoint: plan.Endpoint, SplitReason: "retry", FirstOrdinal: first, LastOrdinal: last, InputDigest: plan.InputDigest, RequestCount: len(plan.Ordinals), InputBytes: plan.InputBytes, EstimatedPromptTokens: plan.EstimatedPromptTokens}); err != nil {
			return nil, err
		}
		for position, ordinal := range plan.Ordinals {
			if err := sqlcgen.New(tx).InsertPreparedDeckBatchChunkItemFromManifest(ctx, sqlcgen.InsertPreparedDeckBatchChunkItemFromManifestParams{Owner: params.OwnerID, Preparation: params.PreparationID, Run: params.RunID, Chunk: chunkID, Generation: generation, Position: position, Ordinal: ordinal}); err != nil {
				return nil, err
			}
		}
		chunk := domain.PreparedDeckBatchChunk{ID: chunkID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, ChunkIndex: chunkIndex, Generation: generation, State: domain.PreparedDeckBatchPending, Model: plan.Model, Endpoint: plan.Endpoint, SplitReason: "retry", FirstOrdinal: first, LastOrdinal: last, InputDigest: plan.InputDigest, RequestCount: len(plan.Ordinals), InputBytes: plan.InputBytes, EstimatedPromptTokens: plan.EstimatedPromptTokens, Ordinals: append([]int(nil), plan.Ordinals...)}
		jobID, err := insertSubmission(ctx, tx, chunk)
		if err != nil || jobID < 1 {
			if err == nil {
				err = ErrInvalidTransition
			}
			return nil, err
		}
		if err = setPreparedDeckBatchSubmissionJobTx(ctx, tx, chunk, jobID); err != nil {
			return nil, err
		}
		chunk.SubmissionJobID, chunk.SubmissionGeneration = jobID, generation
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func setPreparedDeckBatchSubmissionJobTx(ctx context.Context, tx pgx.Tx, chunk domain.PreparedDeckBatchChunk, jobID int64) error {
	rows, err := sqlcgen.New(tx).SetPreparedDeckBatchSubmissionJobPending(ctx, sqlcgen.SetPreparedDeckBatchSubmissionJobPendingParams{OwnerID: chunk.OwnerID, PreparationID: chunk.PreparationID, RunID: chunk.RunID, ID: chunk.ID, SubmissionGeneration: chunk.Generation, SubmissionJobID: pgtype.Int8{Int64: jobID, Valid: true}})
	if err == nil && rows != 1 {
		return ErrPreparedDeckClaimLost
	}
	return err
}

func advancePreparedDeckRunTx(ctx context.Context, tx pgx.Tx, run domain.PreparedDeckRun, insertFinalizer PreparedDeckRunJobInserter) (domain.PreparedDeckRun, error) {
	counts, err := sqlcgen.New(tx).CountPreparedDeckRunOutcomeStates(ctx, sqlcgen.CountPreparedDeckRunOutcomeStatesParams{OwnerID: run.OwnerID, PreparationID: run.PreparationID, RunID: run.ID})
	if err != nil {
		return run, err
	}
	completed, conversionErr := checked.IntFromInt64(counts.CompletedCount)
	if conversionErr != nil {
		return run, fmt.Errorf("invalid completed translation count: %w", conversionErr)
	}
	failed, conversionErr := checked.IntFromInt64(counts.FailedCount)
	if conversionErr != nil {
		return run, fmt.Errorf("invalid failed translation count: %w", conversionErr)
	}
	nonterminal, conversionErr := checked.IntFromInt64(counts.NonterminalCount)
	if conversionErr != nil {
		return run, fmt.Errorf("invalid nonterminal translation count: %w", conversionErr)
	}
	if nonterminal > 0 {
		model, err := sqlcgen.New(tx).UpdatePreparedDeckRunTranslationRunning(ctx, sqlcgen.UpdatePreparedDeckRunTranslationRunningParams{OwnerID: run.OwnerID, PreparationID: run.PreparationID, ID: run.ID, CompletedCount: completed, FailedCount: failed})
		return preparedDeckRunFromModel(model), err
	}
	if run.State != domain.PreparedDeckRunTranslating {
		return run, nil
	}
	if run.ExecutionMode == domain.PreparedDeckExecutionStandard && run.ExternalTranslationConsent && run.ExternalTranslationConfigured && failed > 0 {
		model, err := sqlcgen.New(tx).FailPreparedDeckRunIncomplete(ctx, sqlcgen.FailPreparedDeckRunIncompleteParams{OwnerID: run.OwnerID, PreparationID: run.PreparationID, ID: run.ID, CompletedCount: completed, FailedCount: failed})
		run = preparedDeckRunFromModel(model)
		if err != nil {
			return run, err
		}
		if _, err = sqlcgen.New(tx).FailDeckPreparationTranslation(ctx, sqlcgen.FailDeckPreparationTranslationParams{OwnerID: run.OwnerID, ID: run.PreparationID, CurrentRunID: uuidArg(run.ID), Error: "prepared-deck translation was incomplete"}); err != nil {
			return run, err
		}
		return run, nil
	}
	model, err := sqlcgen.New(tx).FinalizePreparedDeckRun(ctx, sqlcgen.FinalizePreparedDeckRunParams{OwnerID: run.OwnerID, PreparationID: run.PreparationID, ID: run.ID, CompletedCount: completed, FailedCount: failed})
	run = preparedDeckRunFromModel(model)
	if err != nil {
		return run, err
	}
	if insertFinalizer == nil {
		return run, ErrInvalidTransition
	}
	if err = insertFinalizer(ctx, tx, run); err != nil {
		return domain.PreparedDeckRun{}, err
	}
	return run, nil
}

func (s *PostgresStore) AdvancePreparedDeckRunIfTerminal(ctx context.Context, owner, preparationID, runID string, insertFinalizer PreparedDeckRunJobInserter) (result domain.PreparedDeckRun, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PreparedDeckRun{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	model, err := sqlcgen.New(tx).GetPreparedDeckRunForUpdate(ctx, sqlcgen.GetPreparedDeckRunForUpdateParams{OwnerID: owner, PreparationID: preparationID, ID: runID})
	run := preparedDeckRunFromModel(model)
	if err != nil {
		return run, missing(err)
	}
	if run.State == domain.PreparedDeckRunFinalizing || run.State == domain.PreparedDeckRunCompleted {
		return run, tx.Commit(ctx)
	}
	if run.State != domain.PreparedDeckRunTranslating {
		return run, ErrInvalidTransition
	}
	run, err = advancePreparedDeckRunTx(ctx, tx, run, insertFinalizer)
	if err != nil {
		return run, err
	}
	return run, tx.Commit(ctx)
}

func (s *PostgresStore) FailPreparedDeckBatchReconciliation(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token, providerStatus, errorClass, errorCode string) (err error) {
	if !terminalProviderStatus(providerStatus) || validateBoundedError(errorClass, errorCode) != nil {
		return ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	state, err := sqlcgen.New(tx).GetDeckPreparationStateForRun(ctx, sqlcgen.GetDeckPreparationStateForRunParams{OwnerID: owner, ID: preparationID, CurrentRunID: uuidArg(runID)})
	if err != nil {
		return missing(err)
	}
	if domain.DeckPreparationState(state) != domain.DeckPreparationPreparing {
		return ErrPreparedDeckClaimLost
	}
	if _, err = sqlcgen.New(tx).LockPreparedDeckRunTranslating(ctx, sqlcgen.LockPreparedDeckRunTranslatingParams{OwnerID: owner, PreparationID: preparationID, ID: runID}); err != nil {
		return missing(err)
	}
	rows, err := sqlcgen.New(tx).FailPreparedDeckBatchChunk(ctx, sqlcgen.FailPreparedDeckBatchChunkParams{OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, ReconciliationGeneration: generation, ReconciliationClaimToken: uuidArg(token), ProviderStatus: textArg(providerStatus), ErrorClass: errorClass, ErrorCode: errorCode})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrPreparedDeckClaimLost
	}
	runErrorClass := preparedDeckRunReconciliationErrorClass(errorClass)
	rows, err = sqlcgen.New(tx).FailPreparedDeckRun(ctx, sqlcgen.FailPreparedDeckRunParams{OwnerID: owner, PreparationID: preparationID, ID: runID, ErrorClass: runErrorClass, ErrorCode: errorCode})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrPreparedDeckClaimLost
	}
	if _, err = sqlcgen.New(tx).FailDeckPreparationTranslation(ctx, sqlcgen.FailDeckPreparationTranslationParams{OwnerID: owner, ID: preparationID, CurrentRunID: uuidArg(runID), Error: "prepared-deck translation results could not be verified"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// preparedDeckRunReconciliationErrorClass preserves the most useful bounded
// operator/user recovery category supported by the run schema. Provider
// messages and arbitrary error codes never reach this projection.
func preparedDeckRunReconciliationErrorClass(chunkClass string) string {
	switch chunkClass {
	case "provider", "upload", "poll", "expired", "cancelled":
		return "provider"
	case "validation", "malformed_result", "missing_result", "duplicate_result", "unknown_result":
		return "validation"
	case "configuration", "unsupported_model":
		return "configuration"
	default:
		return "reconciliation"
	}
}

func (s *PostgresStore) FailPreparedDeckBatchSubmission(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, state domain.PreparedDeckBatchChunkState, errorClass, errorCode string) (err error) {
	if state != domain.PreparedDeckBatchFailed && state != domain.PreparedDeckBatchAmbiguous || validateBoundedError(errorClass, errorCode) != nil {
		return ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	stateValue, err := sqlcgen.New(tx).GetDeckPreparationStateForRun(ctx, sqlcgen.GetDeckPreparationStateForRunParams{OwnerID: owner, ID: preparationID, CurrentRunID: uuidArg(runID)})
	if err != nil {
		return missing(err)
	}
	if domain.DeckPreparationState(stateValue) != domain.DeckPreparationPreparing {
		return ErrPreparedDeckClaimLost
	}
	if _, err = sqlcgen.New(tx).LockPreparedDeckRunTranslating(ctx, sqlcgen.LockPreparedDeckRunTranslatingParams{OwnerID: owner, PreparationID: preparationID, ID: runID}); err != nil {
		return missing(err)
	}
	rows, err := sqlcgen.New(tx).FailPreparedDeckBatchChunkSubmission(ctx, sqlcgen.FailPreparedDeckBatchChunkSubmissionParams{OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, Generation: generation, SubmissionClaimToken: uuidArg(token), State: string(state), ErrorClass: errorClass, ErrorCode: errorCode})
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrPreparedDeckClaimLost
	}
	runClass := "orchestration"
	if state == domain.PreparedDeckBatchAmbiguous {
		runClass = "ambiguous_submission"
	}
	if rows, err = sqlcgen.New(tx).FailPreparedDeckRun(ctx, sqlcgen.FailPreparedDeckRunParams{OwnerID: owner, PreparationID: preparationID, ID: runID, ErrorClass: runClass, ErrorCode: errorCode}); err != nil {
		return err
	}
	if rows != 1 {
		return ErrPreparedDeckClaimLost
	}
	if _, err = sqlcgen.New(tx).FailDeckPreparationTranslation(ctx, sqlcgen.FailDeckPreparationTranslationParams{OwnerID: owner, ID: preparationID, CurrentRunID: uuidArg(runID), Error: "prepared-deck translation could not be submitted safely"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func terminalProviderStatus(status string) bool {
	switch status {
	case "completed", "failed", "expired", "cancelled":
		return true
	default:
		return false
	}
}

func (s *PostgresStore) SetPreparedDeckBatchReconciliationJobTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID, chunkID string, generation int, jobID int64) error {
	if tx == nil || jobID < 1 {
		return ErrInvalidTransition
	}
	rows, err := sqlcgen.New(tx).SetPreparedDeckBatchReconciliationJob(ctx, sqlcgen.SetPreparedDeckBatchReconciliationJobParams{OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, ReconciliationGeneration: generation, ReconciliationJobID: pgtype.Int8{Int64: jobID, Valid: true}})
	if err == nil && rows != 1 {
		return ErrPreparedDeckClaimLost
	}
	return err
}

func (s *PostgresStore) ListPreparedDeckLiveBatchIDs(ctx context.Context, owner, preparationID, runID string) ([]string, error) {
	rows, err := s.queries().ListPreparedDeckLiveBatchIDs(ctx, sqlcgen.ListPreparedDeckLiveBatchIDsParams{OwnerID: owner, PreparationID: preparationID, RunID: runID})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, row := range rows {
		if id := pgText(row); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
