package persistence

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

const (
	PreparedDeckRetryPolicyVersion       = 1
	DefaultBatchMaxRequests              = 5000
	MaximumBatchGenerations              = 2
	DefaultBatchMaxBytes           int64 = 200 * 1024 * 1024
)

type PreparedDeckRunConfig struct {
	ExternalTranslationConsent, ExternalTranslationConfigured bool
	ContextMode, Provider, ProviderVersion, Endpoint, Model   string
	RetryPolicyVersion, MaxProviderAttempts                   int
	MaxBatchGenerations, BatchMaxRequests                     int
	BatchMaxBytes                                             int64
}

type PreparedDeckBatchChunkPlan struct {
	ID                           string
	ChunkIndex, Generation       int
	Model, Endpoint, SplitReason string
	InputDigest                  string
	InputBytes                   int64
	EstimatedPromptTokens        int64
	Ordinals                     []int
}

type FreezePreparedDeckRunParams struct {
	OwnerID, PreparationID string
	ExpectedManifestDigest string
	Manifest               cardexport.ManifestSnapshot
	Config                 PreparedDeckRunConfig
	Chunks                 []PreparedDeckBatchChunkPlan
}

type FreezePreparedDeckRunResult struct {
	Run            domain.PreparedDeckRun
	ManifestDigest string
	Chunks         []domain.PreparedDeckBatchChunk
	Existing       bool
	NeedsFinalizer bool
}

// PreparedDeckRunJobInserter lets a terminal outcome enqueue finalization in
// the same transaction that advances the run to finalizing.
type PreparedDeckRunJobInserter func(context.Context, pgx.Tx, domain.PreparedDeckRun) error

type preparedDeckRenderPayload struct {
	Morphology     string `json:"morphology"`
	SourceDocument string `json:"source_document"`
	Notes          string `json:"notes"`
}

// FreezePreparedDeckRunTx stores the complete run inside the caller's
// repeatable-read transaction. River jobs can then be inserted with InsertTx
// before the same transaction commits.
func (s *PostgresStore) FreezePreparedDeckRunTx(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams) (FreezePreparedDeckRunResult, error) {
	if s == nil || tx == nil || strings.TrimSpace(params.OwnerID) == "" || strings.TrimSpace(params.PreparationID) == "" {
		return FreezePreparedDeckRunResult{}, ErrInvalidTransition
	}
	var preparationState domain.DeckPreparationState
	var currentRunID, preparationFilename string
	err := tx.QueryRow(ctx, `SELECT state,COALESCE(current_run_id::text,''),filename FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, params.OwnerID, params.PreparationID).Scan(&preparationState, &currentRunID, &preparationFilename)
	if errors.Is(err, pgx.ErrNoRows) {
		return FreezePreparedDeckRunResult{}, ErrNotFound
	}
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	digest, err := params.Manifest.Digest()
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if params.ExpectedManifestDigest != "" && params.ExpectedManifestDigest != digest {
		return FreezePreparedDeckRunResult{}, ErrImmutable
	}
	if currentRunID != "" {
		run, getErr := getPreparedDeckRun(ctx, tx, params.OwnerID, params.PreparationID, currentRunID)
		if getErr != nil {
			return FreezePreparedDeckRunResult{}, getErr
		}
		var storedDigest string
		if getErr = tx.QueryRow(ctx, `SELECT manifest_digest FROM deck_preparation_manifests WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`, params.OwnerID, params.PreparationID, currentRunID).Scan(&storedDigest); getErr != nil {
			return FreezePreparedDeckRunResult{}, missing(getErr)
		}
		if storedDigest != digest || !runConfigMatches(run, params.Config) {
			return FreezePreparedDeckRunResult{}, ErrImmutable
		}
		chunks, getErr := listPreparedDeckBatchChunks(ctx, tx, params.OwnerID, params.PreparationID, currentRunID)
		return FreezePreparedDeckRunResult{Run: run, ManifestDigest: storedDigest, Chunks: chunks, Existing: true, NeedsFinalizer: run.State == domain.PreparedDeckRunFinalizing}, getErr
	}
	if preparationState != domain.DeckPreparationQueued && preparationState != domain.DeckPreparationPreparing {
		return FreezePreparedDeckRunResult{}, ErrInvalidTransition
	}
	if params.Manifest.Owner != params.OwnerID || params.Manifest.SchemaVersion != cardexport.ManifestSchemaVersion {
		return FreezePreparedDeckRunResult{}, ErrImmutable
	}
	if params.Manifest.Filename != preparationFilename {
		return FreezePreparedDeckRunResult{}, fmt.Errorf("%w: manifest filename contradicts preparation", ErrPreparedDeckIdentity)
	}
	config, err := validatePreparedDeckRunConfig(params.Config)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	selectedCount, acceptedCount, omittedCount := params.Manifest.Counts()
	pending := make(map[int]cardexport.ManifestItem)
	completedCount := 0
	for _, item := range params.Manifest.Items {
		if item.Disposition != cardexport.ManifestAccepted {
			continue
		}
		if item.CacheKey == nil {
			completedCount++
			continue
		}
		key := item.CacheKey
		var found bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM enrichment_cache WHERE language=$1 AND canonical_lemma=$2 AND upos=$3 AND provider=$4 AND provider_version=$5 AND sentence_hash=$6)`, key.Language, key.CanonicalLemma, key.UPOS, key.Provider, key.ProviderVersion, key.SentenceHash).Scan(&found); err != nil {
			return FreezePreparedDeckRunResult{}, err
		}
		if found {
			completedCount++
		} else {
			pending[item.Ordinal] = item
		}
	}
	requested := config.ExternalTranslationConsent && config.ExternalTranslationConfigured
	for _, item := range params.Manifest.Items {
		if item.Disposition != cardexport.ManifestAccepted {
			continue
		}
		if requested != (item.CacheKey != nil) {
			return FreezePreparedDeckRunResult{}, fmt.Errorf("%w: manifest cache identity contradicts run configuration", ErrImmutable)
		}
		if item.CacheKey != nil && (item.CacheKey.Provider != config.Provider || item.CacheKey.ProviderVersion != config.ProviderVersion) {
			return FreezePreparedDeckRunResult{}, fmt.Errorf("%w: manifest cache identity contradicts provider snapshot", ErrImmutable)
		}
		if item.CacheKey != nil && ((config.ContextMode == "sentence" && item.CacheKey.SentenceHash != enrichment.SentenceHash(item.Entry.Sentence)) || (config.ContextMode == "lemma_only" && item.CacheKey.SentenceHash != "")) {
			return FreezePreparedDeckRunResult{}, fmt.Errorf("%w: manifest cache identity contradicts context mode", ErrImmutable)
		}
	}
	if err = validateChunkPlans(params.Chunks, config, pending); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	var runNumber int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(run_number),0)+1 FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2`, params.OwnerID, params.PreparationID).Scan(&runNumber); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	runID := uuid.NewString()
	runState, translationState := domain.PreparedDeckRunTranslating, domain.PreparedDeckTranslationPending
	var translationCompletedAt any
	if len(pending) == 0 {
		runState, translationState = domain.PreparedDeckRunFinalizing, domain.PreparedDeckTranslationCompleted
		translationCompletedAt = time.Now().UTC()
	}
	nullable := func(value string) any {
		if requested {
			return value
		}
		return nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO deck_preparation_runs(id,owner_id,preparation_id,run_number,state,translation_state,external_translation_consent,external_translation_configured,context_mode,provider,provider_version,endpoint,model,manifest_schema_version,retry_policy_version,max_provider_attempts,max_batch_generations,batch_max_requests,batch_max_bytes,candidate_count,completed_count,translation_completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`, runID, params.OwnerID, params.PreparationID, runNumber, runState, translationState, config.ExternalTranslationConsent, config.ExternalTranslationConfigured, nullable(config.ContextMode), nullable(config.Provider), nullable(config.ProviderVersion), nullable(config.Endpoint), nullable(config.Model), params.Manifest.SchemaVersion, config.RetryPolicyVersion, config.MaxProviderAttempts, config.MaxBatchGenerations, config.BatchMaxRequests, config.BatchMaxBytes, acceptedCount, completedCount, translationCompletedAt)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO deck_preparation_manifests(owner_id,preparation_id,run_id,schema_version,manifest_digest,deck_name,filename,selected_count,accepted_count,omitted_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, params.OwnerID, params.PreparationID, runID, params.Manifest.SchemaVersion, digest, params.Manifest.DeckName, params.Manifest.Filename, selectedCount, acceptedCount, omittedCount)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	for _, item := range params.Manifest.Items {
		candidateDigest, digestErr := cardexport.CandidateDigest(item)
		if digestErr != nil {
			return FreezePreparedDeckRunResult{}, digestErr
		}
		renderPayload, marshalErr := json.Marshal(preparedDeckRenderPayload{Morphology: item.Entry.Morphology, SourceDocument: item.Entry.SourceDocument, Notes: item.Entry.Notes})
		if marshalErr != nil {
			return FreezePreparedDeckRunResult{}, marshalErr
		}
		var provider, providerVersion, sentenceHash any
		if item.CacheKey != nil {
			provider, providerVersion, sentenceHash = item.CacheKey.Provider, item.CacheKey.ProviderVersion, item.CacheKey.SentenceHash
		}
		_, err = tx.Exec(ctx, `INSERT INTO deck_preparation_manifest_items(owner_id,preparation_id,run_id,ordinal,disposition,language,canonical_lemma,upos,source_sentence,tested_target,first_encounter,quality_score,quality_reasons,render_payload,provider,provider_version,sentence_hash,candidate_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, params.OwnerID, params.PreparationID, runID, item.Ordinal, item.Disposition, item.Entry.Language, item.Entry.CanonicalLemma, item.Entry.UPOS, item.Entry.Sentence, item.Entry.TargetWord, item.Entry.FirstEncounter, item.Quality.Score, item.Quality.Reasons, renderPayload, provider, providerVersion, sentenceHash, candidateDigest)
		if err != nil {
			return FreezePreparedDeckRunResult{}, err
		}
		if item.Disposition == cardexport.ManifestAccepted {
			state, cacheHits := domain.PreparedDeckOutcomeCompleted, 0
			var terminalAt any = time.Now().UTC()
			if _, waiting := pending[item.Ordinal]; waiting {
				state, terminalAt = domain.PreparedDeckOutcomePending, nil
			} else if item.CacheKey != nil {
				cacheHits = 1
			}
			_, err = tx.Exec(ctx, `INSERT INTO deck_preparation_translation_outcomes(owner_id,preparation_id,run_id,ordinal,state,max_provider_attempts,terminal_at,cache_hit_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, params.OwnerID, params.PreparationID, runID, item.Ordinal, state, config.MaxProviderAttempts, terminalAt, cacheHits)
			if err != nil {
				return FreezePreparedDeckRunResult{}, err
			}
		}
	}
	chunks, err := insertPreparedDeckChunkPlans(ctx, tx, params, config, runID, pending)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE deck_preparations SET state='preparing',current_run_id=$3,started_at=COALESCE(started_at,now()),completed_at=NULL,error='',updated_at=now() WHERE owner_id=$1 AND id=$2`, params.OwnerID, params.PreparationID, runID); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	run, err := getPreparedDeckRun(ctx, tx, params.OwnerID, params.PreparationID, runID)
	return FreezePreparedDeckRunResult{Run: run, ManifestDigest: digest, Chunks: chunks, NeedsFinalizer: runState == domain.PreparedDeckRunFinalizing}, err
}

func insertPreparedDeckChunkPlans(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, _ PreparedDeckRunConfig, runID string, pending map[int]cardexport.ManifestItem) ([]domain.PreparedDeckBatchChunk, error) {
	chunks := make([]domain.PreparedDeckBatchChunk, 0, len(params.Chunks))
	for _, plan := range params.Chunks {
		chunkID := plan.ID
		if chunkID == "" {
			chunkID = uuid.NewString()
		}
		first, last := plan.Ordinals[0], plan.Ordinals[len(plan.Ordinals)-1]
		_, err := tx.Exec(ctx, `INSERT INTO deck_preparation_batch_chunks(id,owner_id,preparation_id,run_id,chunk_index,generation,model,endpoint,split_reason,first_ordinal,last_ordinal,input_digest,request_count,input_bytes,estimated_prompt_tokens) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, chunkID, params.OwnerID, params.PreparationID, runID, plan.ChunkIndex, plan.Generation, plan.Model, plan.Endpoint, plan.SplitReason, first, last, plan.InputDigest, len(plan.Ordinals), plan.InputBytes, plan.EstimatedPromptTokens)
		if err != nil {
			return nil, err
		}
		for position, ordinal := range plan.Ordinals {
			candidateDigest, digestErr := cardexport.CandidateDigest(pending[ordinal])
			if digestErr != nil {
				return nil, digestErr
			}
			if _, err = tx.Exec(ctx, `INSERT INTO deck_preparation_batch_chunk_items(owner_id,preparation_id,run_id,chunk_id,generation,position,ordinal,candidate_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, params.OwnerID, params.PreparationID, runID, chunkID, plan.Generation, position, ordinal, candidateDigest); err != nil {
				return nil, err
			}
		}
		chunks = append(chunks, domain.PreparedDeckBatchChunk{ID: chunkID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, ChunkIndex: plan.ChunkIndex, Generation: plan.Generation, State: domain.PreparedDeckBatchPending, Model: plan.Model, Endpoint: plan.Endpoint, SplitReason: plan.SplitReason, FirstOrdinal: first, LastOrdinal: last, InputDigest: plan.InputDigest, RequestCount: len(plan.Ordinals), InputBytes: plan.InputBytes, EstimatedPromptTokens: plan.EstimatedPromptTokens, Ordinals: append([]int(nil), plan.Ordinals...)})
	}
	return chunks, nil
}

func validatePreparedDeckRunConfig(config PreparedDeckRunConfig) (PreparedDeckRunConfig, error) {
	if config.RetryPolicyVersion == 0 {
		config.RetryPolicyVersion = PreparedDeckRetryPolicyVersion
	}
	if config.MaxProviderAttempts == 0 {
		config.MaxProviderAttempts = 2
	}
	if config.MaxBatchGenerations == 0 {
		config.MaxBatchGenerations = 2
	}
	if config.BatchMaxRequests == 0 {
		config.BatchMaxRequests = DefaultBatchMaxRequests
	}
	if config.BatchMaxBytes == 0 {
		config.BatchMaxBytes = DefaultBatchMaxBytes
	}
	requested := config.ExternalTranslationConsent && config.ExternalTranslationConfigured
	if requested {
		if (config.ContextMode != "sentence" && config.ContextMode != "lemma_only") || config.Provider == "" || config.ProviderVersion == "" || config.Endpoint != "/v1/chat/completions" || config.Model == "" {
			return config, fmt.Errorf("%w: incomplete external translation identity", ErrInvalidTransition)
		}
	} else if config.ContextMode != "" || config.Provider != "" || config.ProviderVersion != "" || config.Endpoint != "" || config.Model != "" {
		return config, fmt.Errorf("%w: disabled external translation has provider identity", ErrInvalidTransition)
	}
	if config.RetryPolicyVersion < 1 || config.MaxProviderAttempts < 1 || config.MaxBatchGenerations < 1 || config.MaxBatchGenerations > MaximumBatchGenerations || config.BatchMaxRequests < 1 || config.BatchMaxRequests > 50000 || config.BatchMaxBytes < 1 || config.BatchMaxBytes > DefaultBatchMaxBytes {
		return config, fmt.Errorf("%w: invalid durable run limits", ErrInvalidTransition)
	}
	return config, nil
}

func validateChunkPlans(plans []PreparedDeckBatchChunkPlan, config PreparedDeckRunConfig, pending map[int]cardexport.ManifestItem) error {
	covered := make(map[int]struct{}, len(pending))
	identities := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		validSplit := plan.SplitReason == "run" || plan.SplitReason == "request_limit" || plan.SplitReason == "byte_limit" || plan.SplitReason == "token_limit" || plan.SplitReason == "retry"
		digestBytes, digestErr := hex.DecodeString(plan.InputDigest)
		validDigest := digestErr == nil && len(digestBytes) == 32 && plan.InputDigest == strings.ToLower(plan.InputDigest)
		if len(plan.Ordinals) == 0 || len(plan.Ordinals) > config.BatchMaxRequests || plan.InputBytes < 1 || plan.InputBytes > config.BatchMaxBytes || plan.Generation < 1 || plan.Generation > config.MaxBatchGenerations || plan.ChunkIndex < 0 || plan.Model != config.Model || plan.Endpoint != config.Endpoint || !validSplit || !validDigest {
			return fmt.Errorf("%w: invalid Batch chunk plan", ErrInvalidTransition)
		}
		identity := fmt.Sprintf("%d/%d", plan.Generation, plan.ChunkIndex)
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("%w: duplicate Batch chunk identity", ErrImmutable)
		}
		identities[identity] = struct{}{}
		previous := -1
		for _, ordinal := range plan.Ordinals {
			if ordinal <= previous {
				return fmt.Errorf("%w: Batch chunk ordinals are not ordered", ErrImmutable)
			}
			if _, exists := pending[ordinal]; !exists {
				return fmt.Errorf("%w: Batch chunk contains a non-pending item", ErrImmutable)
			}
			if _, exists := covered[ordinal]; exists {
				return fmt.Errorf("%w: Batch item appears in multiple chunks", ErrImmutable)
			}
			covered[ordinal], previous = struct{}{}, ordinal
		}
	}
	if len(covered) != len(pending) {
		return fmt.Errorf("%w: Batch chunks do not cover pending items", ErrImmutable)
	}
	return nil
}

func runConfigMatches(run domain.PreparedDeckRun, config PreparedDeckRunConfig) bool {
	validated, err := validatePreparedDeckRunConfig(config)
	return err == nil && run.ExternalTranslationConsent == validated.ExternalTranslationConsent && run.ExternalTranslationConfigured == validated.ExternalTranslationConfigured && run.ContextMode == validated.ContextMode && run.Provider == validated.Provider && run.ProviderVersion == validated.ProviderVersion && run.Endpoint == validated.Endpoint && run.Model == validated.Model && run.RetryPolicyVersion == validated.RetryPolicyVersion && run.MaxProviderAttempts == validated.MaxProviderAttempts && run.MaxBatchGenerations == validated.MaxBatchGenerations && run.BatchMaxRequests == validated.BatchMaxRequests && run.BatchMaxBytes == validated.BatchMaxBytes
}

const preparedDeckRunColumns = `id::text,owner_id::text,preparation_id::text,run_number,state,translation_state,external_translation_consent,external_translation_configured,COALESCE(context_mode,''),COALESCE(provider,''),COALESCE(provider_version,''),COALESCE(endpoint,''),COALESCE(model,''),manifest_schema_version,retry_policy_version,max_provider_attempts,max_batch_generations,batch_max_requests,batch_max_bytes,candidate_count,completed_count,failed_count,finalization_dispatch_generation,finalization_dispatch_count,COALESCE(finalization_job_id,0),COALESCE(finalization_claim_token::text,''),finalization_claimed_at,finalization_lease_expires_at,error_class,error_code,created_at,updated_at,translation_completed_at,completed_at`

func scanPreparedDeckRun(row rowScanner) (domain.PreparedDeckRun, error) {
	var run domain.PreparedDeckRun
	err := row.Scan(&run.ID, &run.OwnerID, &run.PreparationID, &run.RunNumber, &run.State, &run.TranslationState, &run.ExternalTranslationConsent, &run.ExternalTranslationConfigured, &run.ContextMode, &run.Provider, &run.ProviderVersion, &run.Endpoint, &run.Model, &run.ManifestSchemaVersion, &run.RetryPolicyVersion, &run.MaxProviderAttempts, &run.MaxBatchGenerations, &run.BatchMaxRequests, &run.BatchMaxBytes, &run.CandidateCount, &run.CompletedCount, &run.FailedCount, &run.FinalizationDispatchGeneration, &run.FinalizationDispatchCount, &run.FinalizationJobID, &run.FinalizationClaimToken, &run.FinalizationClaimedAt, &run.FinalizationLeaseExpiresAt, &run.ErrorClass, &run.ErrorCode, &run.CreatedAt, &run.UpdatedAt, &run.TranslationCompletedAt, &run.CompletedAt)
	return run, missing(err)
}

type preparedDeckQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getPreparedDeckRun(ctx context.Context, q preparedDeckQueryRower, owner, preparationID, runID string) (domain.PreparedDeckRun, error) {
	return scanPreparedDeckRun(q.QueryRow(ctx, `SELECT `+preparedDeckRunColumns+` FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3`, owner, preparationID, runID))
}

func (s *PostgresStore) GetPreparedDeckRun(ctx context.Context, owner, preparationID, runID string) (domain.PreparedDeckRun, error) {
	return getPreparedDeckRun(ctx, s.pool, owner, preparationID, runID)
}

func (s *PostgresStore) GetCurrentPreparedDeckRun(ctx context.Context, owner, preparationID string) (domain.PreparedDeckRun, error) {
	return scanPreparedDeckRun(s.pool.QueryRow(ctx, `SELECT `+preparedDeckRunColumns+` FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=(SELECT current_run_id FROM deck_preparations WHERE owner_id=$1 AND id=$2)`, owner, preparationID))
}

func (s *PostgresStore) LoadPreparedDeckManifest(ctx context.Context, owner, preparationID, runID string) (cardexport.ManifestSnapshot, string, error) {
	var snapshot cardexport.ManifestSnapshot
	var storedDigest string
	var selectedCount, acceptedCount, omittedCount int
	err := s.pool.QueryRow(ctx, `SELECT schema_version,deck_name,filename,manifest_digest,selected_count,accepted_count,omitted_count FROM deck_preparation_manifests WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`, owner, preparationID, runID).Scan(&snapshot.SchemaVersion, &snapshot.DeckName, &snapshot.Filename, &storedDigest, &selectedCount, &acceptedCount, &omittedCount)
	if err != nil {
		return snapshot, "", missing(err)
	}
	snapshot.Owner = owner
	rows, err := s.pool.Query(ctx, `SELECT ordinal,disposition,language,canonical_lemma,upos,source_sentence,tested_target,first_encounter,quality_score,quality_reasons,render_payload,COALESCE(provider,''),COALESCE(provider_version,''),COALESCE(sentence_hash,''),candidate_digest FROM deck_preparation_manifest_items WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 ORDER BY ordinal`, owner, preparationID, runID)
	if err != nil {
		return snapshot, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var item cardexport.ManifestItem
		var payload []byte
		var provider, providerVersion, sentenceHash, candidateDigest string
		if err = rows.Scan(&item.Ordinal, &item.Disposition, &item.Entry.Language, &item.Entry.CanonicalLemma, &item.Entry.UPOS, &item.Entry.Sentence, &item.Entry.TargetWord, &item.Entry.FirstEncounter, &item.Quality.Score, &item.Quality.Reasons, &payload, &provider, &providerVersion, &sentenceHash, &candidateDigest); err != nil {
			return snapshot, "", err
		}
		item.Quality.Accepted = item.Disposition == cardexport.ManifestAccepted
		var render preparedDeckRenderPayload
		if err = json.Unmarshal(payload, &render); err != nil {
			return snapshot, "", fmt.Errorf("decode durable manifest render payload: %w", err)
		}
		item.Entry.Morphology, item.Entry.SourceDocument, item.Entry.Notes = render.Morphology, render.SourceDocument, render.Notes
		if provider != "" {
			item.CacheKey = &enrichment.CacheKey{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, Provider: provider, ProviderVersion: providerVersion, SentenceHash: sentenceHash}
		}
		calculated, digestErr := cardexport.CandidateDigest(item)
		if digestErr != nil || calculated != candidateDigest {
			return snapshot, "", ErrPreparedDeckIdentity
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if err = rows.Err(); err != nil {
		return snapshot, "", err
	}
	selected, accepted, omitted := snapshot.Counts()
	if selected != selectedCount || accepted != acceptedCount || omitted != omittedCount {
		return snapshot, "", ErrPreparedDeckIdentity
	}
	calculated, err := snapshot.Digest()
	if err != nil || calculated != storedDigest {
		return snapshot, "", ErrPreparedDeckIdentity
	}
	return snapshot, storedDigest, nil
}

func (s *PostgresStore) PreparedDeckRunProgress(ctx context.Context, owner, preparationID, runID string) (domain.PreparedDeckRunProgress, error) {
	var progress domain.PreparedDeckRunProgress
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3)`, owner, preparationID, runID).Scan(&exists)
	if err != nil {
		return progress, err
	}
	if !exists {
		return progress, ErrNotFound
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE state='pending'),count(*) FILTER (WHERE state='running'),count(*) FILTER (WHERE state='completed'),count(*) FILTER (WHERE state='failed'),count(*) FILTER (WHERE state='cancelled') FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`, owner, preparationID, runID).Scan(&progress.CandidateCount, &progress.PendingCount, &progress.RunningCount, &progress.CompletedCount, &progress.FailedCount, &progress.CancelledCount)
	return progress, err
}
