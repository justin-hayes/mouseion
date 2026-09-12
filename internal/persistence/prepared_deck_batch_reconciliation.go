package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

type PreparedDeckBatchItemReconciliation struct {
	Ordinal    int
	State      domain.PreparedDeckOutcomeState
	CacheEntry *enrichment.CacheEntry
	ErrorClass string
	ErrorCode  string
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
func (s *PostgresStore) ReconcilePreparedDeckBatch(ctx context.Context, params PreparedDeckBatchReconcileParams, insertSubmission PreparedDeckBatchSubmissionJobInserter, insertFinalizer PreparedDeckRunJobInserter) (PreparedDeckBatchReconcileResult, error) {
	if s == nil || strings.TrimSpace(params.OwnerID) == "" || strings.TrimSpace(params.PreparationID) == "" || strings.TrimSpace(params.RunID) == "" || strings.TrimSpace(params.ChunkID) == "" || strings.TrimSpace(params.ClaimToken) == "" {
		return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
	}
	if params.Chunk.State != domain.PreparedDeckBatchCompleted || !terminalProviderStatus(params.Chunk.ProviderStatus) {
		return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
	}
	if params.Chunk.CompletedCount < 0 || params.Chunk.FailedCount < 0 || params.Chunk.ExpiredCount < 0 || params.Chunk.InputTokens < 0 || params.Chunk.OutputTokens < 0 {
		return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
	}
	if err := validateBoundedError(params.Chunk.ErrorClass, params.Chunk.ErrorCode); err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	defer tx.Rollback(ctx)
	runModel, err := sqlcgen.New(tx).GetPreparedDeckRunForUpdate(ctx, sqlcgen.GetPreparedDeckRunForUpdateParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, ID: params.RunID})
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, missing(err)
	}
	run := preparedDeckRunFromModel(runModel)
	if run.State != domain.PreparedDeckRunTranslating || (run.TranslationState != domain.PreparedDeckTranslationPending && run.TranslationState != domain.PreparedDeckTranslationRunning) {
		return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckClaimLost
	}
	chunkModel, err := sqlcgen.New(tx).GetPreparedDeckBatchChunkForUpdate(ctx, sqlcgen.GetPreparedDeckBatchChunkForUpdateParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, ID: params.ChunkID})
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, missing(err)
	}
	chunk := preparedDeckBatchChunkFromModel(chunkModel)
	if chunk.State != domain.PreparedDeckBatchReconciling || chunk.ReconciliationGeneration != params.ReconciliationGeneration || chunk.ReconciliationClaimToken != params.ClaimToken {
		return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckClaimLost
	}

	members, err := loadPreparedDeckBatchMembers(ctx, tx, params)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	if len(members) != chunk.RequestCount || len(params.Items) != len(members) || params.Chunk.CompletedCount+params.Chunk.FailedCount+params.Chunk.ExpiredCount > chunk.RequestCount {
		return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
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
			return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
		}
		if _, duplicate := seen[item.Ordinal]; duplicate {
			return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
		}
		seen[item.Ordinal] = struct{}{}
		if item.State != domain.PreparedDeckOutcomeCompleted && item.State != domain.PreparedDeckOutcomePending && item.State != domain.PreparedDeckOutcomeFailed {
			return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
		}
		if item.State == domain.PreparedDeckOutcomeFailed && strings.TrimSpace(item.ErrorClass) == "" {
			return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
		}
		if err = validateBoundedError(item.ErrorClass, item.ErrorCode); err != nil {
			return PreparedDeckBatchReconcileResult{}, err
		}
		outcomeModel, getErr := sqlcgen.New(tx).GetPreparedDeckTranslationOutcomeForUpdate(ctx, sqlcgen.GetPreparedDeckTranslationOutcomeForUpdateParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, Ordinal: item.Ordinal})
		outcome := preparedDeckOutcomeFromModel(outcomeModel)
		if getErr != nil {
			return PreparedDeckBatchReconcileResult{}, missing(getErr)
		}
		if outcome.State == domain.PreparedDeckOutcomeCompleted || outcome.State == domain.PreparedDeckOutcomeFailed || outcome.State == domain.PreparedDeckOutcomeCancelled {
			continue
		}
		if outcome.State != domain.PreparedDeckOutcomePending && outcome.State != domain.PreparedDeckOutcomeRunning {
			return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
		}
		if item.State == domain.PreparedDeckOutcomePending && chunk.Generation >= run.MaxBatchGenerations {
			return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
		}
		if item.State == domain.PreparedDeckOutcomeCompleted {
			if item.CacheEntry == nil || item.CacheEntry.CacheKey != member.Key || item.CacheEntry.CachedAt.IsZero() {
				return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
			}
			entry := item.CacheEntry
			if err = sqlcgen.New(tx).UpsertEnrichmentCache(ctx, sqlcgen.UpsertEnrichmentCacheParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash, Translation: entry.Translation, Gloss: entry.Gloss, SentenceTranslation: entry.SentenceTranslation, SentenceTranslationTarget: entry.SentenceTranslationTarget, CachedAt: entry.CachedAt}); err != nil {
				return PreparedDeckBatchReconcileResult{}, err
			}
			// The cache is immutable and first-writer-wins. Another run may have
			// populated this exact key after this Batch was submitted; that row is
			// the trusted result the finalizer must consume even when a stochastic
			// provider returned different text or this attempt has a later timestamp.
			if _, err = sqlcgen.New(tx).EnrichmentCacheLookup(ctx, sqlcgen.EnrichmentCacheLookupParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash}); err != nil {
				return PreparedDeckBatchReconcileResult{}, err
			}
		} else if item.CacheEntry != nil {
			return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
		}
		var terminalAt *time.Time
		if item.State != domain.PreparedDeckOutcomePending {
			terminal := time.Now().UTC()
			terminalAt = &terminal
		} else {
			retryOrdinals[item.Ordinal] = struct{}{}
		}
		if err = sqlcgen.New(tx).UpdatePreparedDeckOutcomeFromBatch(ctx, sqlcgen.UpdatePreparedDeckOutcomeFromBatchParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, Ordinal: item.Ordinal, State: string(item.State), ProviderAttemptCount: chunk.Generation, TerminalAt: pgTimeArgPtr(terminalAt), ErrorClass: item.ErrorClass, ErrorCode: item.ErrorCode}); err != nil {
			return PreparedDeckBatchReconcileResult{}, err
		}
	}
	if len(seen) != len(members) {
		return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
	}

	retryChunks, err := insertPreparedDeckRetryChunks(ctx, tx, params, run, chunk.Generation+1, retryOrdinals, insertSubmission)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	chunkModel, err = sqlcgen.New(tx).CompletePreparedDeckBatchChunk(ctx, sqlcgen.CompletePreparedDeckBatchChunkParams{Owner: params.OwnerID, Preparation: params.PreparationID, Run: params.RunID, ID: params.ChunkID, ProviderStatus: textArg(params.Chunk.ProviderStatus), OutputFileID: params.Chunk.OutputFileID, ErrorFileID: params.Chunk.ErrorFileID, CompletedCount: params.Chunk.CompletedCount, FailedCount: params.Chunk.FailedCount, ExpiredCount: params.Chunk.ExpiredCount, InputTokens: params.Chunk.InputTokens, OutputTokens: params.Chunk.OutputTokens, ErrorClass: params.Chunk.ErrorClass, ErrorCode: params.Chunk.ErrorCode, ProviderCompletedAt: pgTimeArgPtr(params.Chunk.ProviderCompletedAt)})
	chunk = preparedDeckBatchChunkFromModel(chunkModel)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, missing(err)
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

func loadPreparedDeckBatchMembers(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams) ([]preparedDeckBatchMember, error) {
	rows, err := sqlcgen.New(tx).ListPreparedDeckBatchChunkMembers(ctx, sqlcgen.ListPreparedDeckBatchChunkMembersParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, ChunkID: params.ChunkID})
	if err != nil {
		return nil, err
	}
	var members []preparedDeckBatchMember
	for _, row := range rows {
		member := preparedDeckBatchMember{Ordinal: int(row.Ordinal), Key: enrichment.CacheKey{Language: row.Language, TargetLanguage: row.TargetLanguage, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, Provider: row.Provider, ProviderVersion: row.ProviderVersion, SentenceHash: row.SentenceHash}}
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
	completed, failed, nonterminal := int(counts.CompletedCount), int(counts.FailedCount), int(counts.NonterminalCount)
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

func (s *PostgresStore) AdvancePreparedDeckRunIfTerminal(ctx context.Context, owner, preparationID, runID string, insertFinalizer PreparedDeckRunJobInserter) (domain.PreparedDeckRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PreparedDeckRun{}, err
	}
	defer tx.Rollback(ctx)
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

func (s *PostgresStore) FailPreparedDeckBatchReconciliation(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token, providerStatus, errorClass, errorCode string) error {
	if !terminalProviderStatus(providerStatus) || validateBoundedError(errorClass, errorCode) != nil {
		return ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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

func (s *PostgresStore) FailPreparedDeckBatchSubmission(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, state domain.PreparedDeckBatchChunkState, errorClass, errorCode string) error {
	if state != domain.PreparedDeckBatchFailed && state != domain.PreparedDeckBatchAmbiguous || validateBoundedError(errorClass, errorCode) != nil {
		return ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
