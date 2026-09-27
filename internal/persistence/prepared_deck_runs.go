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
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/checked"
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
	ExecutionMode, TargetLanguage                             string
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
	RunID                  string
	ExpectedManifestDigest string
	Projection             cardexport.StorageProjection
	Config                 PreparedDeckRunConfig
	Chunks                 []PreparedDeckBatchChunkPlan
}

type FreezePreparedDeckRunResult struct {
	Run             domain.PreparedDeckRun
	ManifestDigest  string
	Chunks          []domain.PreparedDeckBatchChunk
	Existing        bool
	NeedsFinalizer  bool
	PendingOrdinals []int
}

// PreparedDeckRunJobInserter lets a terminal outcome enqueue finalization in
// the same transaction that advances the run to finalizing.
type PreparedDeckRunJobInserter func(context.Context, pgx.Tx, domain.PreparedDeckRun) error

type preparedDeckRenderPayload struct {
	Morphology                string                    `json:"morphology"`
	Gloss                     string                    `json:"gloss"`
	Plural                    string                    `json:"plural,omitempty"`
	IPA                       string                    `json:"ipa,omitempty"`
	PrincipalParts            string                    `json:"principal_parts,omitempty"`
	DictionaryProviderVersion string                    `json:"dictionary_provider_version,omitempty"`
	OmittedEvidenceCount      int                       `json:"omitted_evidence_count,omitempty"`
	CandidateSenses           []enrichment.LexicalSense `json:"candidate_senses,omitempty"`
	SentenceTokens            []analyzer.Token          `json:"sentence_tokens,omitempty"`
	SourceDocument            string                    `json:"source_document"`
	Notes                     string                    `json:"notes"`
}

// FreezePreparedDeckRunTx stores the complete run inside the caller's
// repeatable-read transaction. River jobs can then be inserted with InsertTx
// before the same transaction commits.
func (s *PostgresStore) FreezePreparedDeckRunTx(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams) (FreezePreparedDeckRunResult, error) {
	if s == nil || tx == nil || strings.TrimSpace(params.OwnerID) == "" || strings.TrimSpace(params.PreparationID) == "" {
		return FreezePreparedDeckRunResult{}, ErrInvalidTransition
	}
	freezeState, err := sqlcgen.New(tx).GetDeckPreparationFreezeState(ctx, sqlcgen.GetDeckPreparationFreezeStateParams{OwnerID: params.OwnerID, ID: params.PreparationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return FreezePreparedDeckRunResult{}, ErrNotFound
	}
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	preparationState := domain.DeckPreparationState(freezeState.State)
	currentRunID := uuidString(freezeState.CurrentRunID)
	digest, candidateDigests, err := params.Projection.Digests()
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if params.ExpectedManifestDigest != "" && params.ExpectedManifestDigest != digest {
		return FreezePreparedDeckRunResult{}, ErrImmutable
	}
	if currentRunID != "" {
		return existingPreparedDeckRunFreezeResult(ctx, tx, params, currentRunID, digest)
	}
	if preparationState != domain.DeckPreparationQueued && preparationState != domain.DeckPreparationPreparing {
		return FreezePreparedDeckRunResult{}, ErrInvalidTransition
	}
	config, err := validatePreparedDeckRunConfig(params.Config)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if err := validatePreparedDeckManifestShape(params.Projection, params.OwnerID, freezeState.Filename); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	manifestWork, err := classifyPreparedDeckManifest(ctx, tx, params.Projection)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if err := validatePreparedDeckManifestCacheIdentity(params.Projection, config); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if config.ExecutionMode == string(domain.PreparedDeckExecutionBatch) {
		if err = validateChunkPlans(params.Chunks, config, manifestWork.pending); err != nil {
			return FreezePreparedDeckRunResult{}, err
		}
	} else if len(params.Chunks) != 0 {
		return FreezePreparedDeckRunResult{}, fmt.Errorf("%w: standard runs cannot contain Batch chunks", ErrImmutable)
	}
	runNumberValue, err := sqlcgen.New(tx).NextPreparedDeckRunNumber(ctx, sqlcgen.NextPreparedDeckRunNumberParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID})
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	runNumber := int(runNumberValue)
	runID := params.RunID
	if runID == "" {
		runID = uuid.NewString()
	} else if parsed, parseErr := uuid.Parse(runID); parseErr != nil || parsed.String() != runID {
		return FreezePreparedDeckRunResult{}, fmt.Errorf("%w: invalid durable run identity", ErrImmutable)
	}
	runState, translationState := domain.PreparedDeckRunTranslating, domain.PreparedDeckTranslationPending
	var translationCompletedAt *time.Time
	if len(manifestWork.pending) == 0 {
		runState, translationState = domain.PreparedDeckRunFinalizing, domain.PreparedDeckTranslationCompleted
		completedAt := time.Now().UTC()
		translationCompletedAt = &completedAt
	}
	selectedCount, acceptedCount, omittedCount := params.Projection.Counts()
	if err = insertPreparedDeckRunRow(ctx, tx, params, config, runID, runNumber, runState, translationState, acceptedCount, manifestWork.completedCount, translationCompletedAt); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if err = insertPreparedDeckManifestRow(ctx, tx, params, runID, digest, selectedCount, acceptedCount, omittedCount); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if err = insertPreparedDeckManifestItems(ctx, tx, params, config, candidateDigests, runID, manifestWork.pending); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	chunks, err := insertPreparedDeckChunkPlans(ctx, tx, params, candidateDigests, runID)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if err = sqlcgen.New(tx).AttachPreparedDeckRun(ctx, sqlcgen.AttachPreparedDeckRunParams{OwnerID: params.OwnerID, ID: params.PreparationID, CurrentRunID: uuidArg(runID)}); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	run, err := getPreparedDeckRun(ctx, tx, params.OwnerID, params.PreparationID, runID)
	pendingOrdinals := preparedDeckPendingOrdinals(params.Projection.Items, manifestWork.pending)
	return FreezePreparedDeckRunResult{Run: run, ManifestDigest: digest, Chunks: chunks, PendingOrdinals: pendingOrdinals, NeedsFinalizer: runState == domain.PreparedDeckRunFinalizing}, err
}

func existingPreparedDeckRunFreezeResult(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, runID, digest string) (FreezePreparedDeckRunResult, error) {
	run, err := getPreparedDeckRun(ctx, tx, params.OwnerID, params.PreparationID, runID)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	storedDigest, err := sqlcgen.New(tx).GetPreparedDeckManifestDigest(ctx, sqlcgen.GetPreparedDeckManifestDigestParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID})
	if err != nil {
		return FreezePreparedDeckRunResult{}, missing(err)
	}
	if storedDigest != digest || !runConfigMatches(run, params.Config) {
		return FreezePreparedDeckRunResult{}, ErrImmutable
	}
	chunks, err := listPreparedDeckBatchChunks(ctx, tx, params.OwnerID, params.PreparationID, runID)
	return FreezePreparedDeckRunResult{Run: run, ManifestDigest: storedDigest, Chunks: chunks, Existing: true, NeedsFinalizer: run.State == domain.PreparedDeckRunFinalizing}, err
}

type preparedDeckManifestWork struct {
	pending        map[int]cardexport.ManifestItem
	completedCount int
}

func classifyPreparedDeckManifest(ctx context.Context, tx pgx.Tx, projection cardexport.StorageProjection) (preparedDeckManifestWork, error) {
	manifestWork := preparedDeckManifestWork{pending: make(map[int]cardexport.ManifestItem)}
	q := sqlcgen.New(tx)
	for _, item := range projection.Items {
		if item.Disposition != cardexport.ManifestAccepted {
			continue
		}
		if item.CacheKey == nil {
			manifestWork.completedCount++
			continue
		}
		key := item.CacheKey
		found, err := q.PreparedDeckCacheExists(ctx, sqlcgen.PreparedDeckCacheExistsParams{Language: key.Language, TargetLanguage: key.TargetLanguage, CanonicalLemma: key.CanonicalLemma, Upos: key.UPOS, Provider: key.Provider, ProviderVersion: key.ProviderVersion, SentenceHash: key.SentenceHash, DictionaryProviderVersion: key.DictionaryProviderVersion, MeaningEvidenceHash: key.MeaningEvidenceHash})
		if err != nil {
			return preparedDeckManifestWork{}, err
		}
		if found {
			manifestWork.completedCount++
			continue
		}
		manifestWork.pending[item.Ordinal] = item
	}
	return manifestWork, nil
}

func validatePreparedDeckManifestShape(projection cardexport.StorageProjection, owner, filename string) error {
	if projection.Owner != owner || projection.SchemaVersion != cardexport.ManifestSchemaVersion {
		return ErrImmutable
	}
	if projection.Filename != filename {
		return fmt.Errorf("%w: manifest filename contradicts preparation", ErrPreparedDeckIdentity)
	}
	return nil
}

func validatePreparedDeckManifestCacheIdentity(projection cardexport.StorageProjection, config PreparedDeckRunConfig) error {
	requested := config.ExternalTranslationConsent && config.ExternalTranslationConfigured
	for _, item := range projection.Items {
		if item.Disposition != cardexport.ManifestAccepted {
			continue
		}
		if requested != (item.CacheKey != nil) {
			return fmt.Errorf("%w: manifest cache identity contradicts run configuration", ErrImmutable)
		}
		if item.CacheKey != nil && (item.CacheKey.Provider != config.Provider || item.CacheKey.ProviderVersion != config.ProviderVersion) {
			return fmt.Errorf("%w: manifest cache identity contradicts provider snapshot", ErrImmutable)
		}
		if item.CacheKey != nil && config.ExecutionMode == string(domain.PreparedDeckExecutionStandard) && item.CacheKey.MeaningEvidenceHash != enrichment.MeaningEvidenceHash(item.Entry.CandidateSenses) {
			return fmt.Errorf("%w: standard manifest cache identity contradicts frozen meaning evidence", ErrImmutable)
		}
		if item.CacheKey != nil && config.ExecutionMode == string(domain.PreparedDeckExecutionBatch) && item.CacheKey.MeaningEvidenceHash != "" {
			return fmt.Errorf("%w: Batch manifest has contextual-gloss cache identity", ErrImmutable)
		}
		if item.CacheKey != nil && ((config.ContextMode == "sentence" && item.CacheKey.SentenceHash != enrichment.SentenceHash(item.Entry.Sentence)) || (config.ContextMode == "lemma_only" && item.CacheKey.SentenceHash != "")) {
			return fmt.Errorf("%w: manifest cache identity contradicts context mode", ErrImmutable)
		}
	}
	return nil
}

func insertPreparedDeckRunRow(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, config PreparedDeckRunConfig, runID string, runNumber int, runState domain.PreparedDeckRunState, translationState domain.PreparedDeckTranslationState, acceptedCount, completedCount int, translationCompletedAt *time.Time) error {
	return sqlcgen.New(tx).InsertPreparedDeckRun(ctx, sqlcgen.InsertPreparedDeckRunParams{ID: runID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunNumber: runNumber, State: string(runState), TranslationState: string(translationState), ExecutionMode: config.ExecutionMode, TargetLanguage: config.TargetLanguage, ExternalTranslationConsent: config.ExternalTranslationConsent, ExternalTranslationConfigured: config.ExternalTranslationConfigured, ContextMode: nullableTextArg(config.ContextMode), Provider: nullableTextArg(config.Provider), ProviderVersion: nullableTextArg(config.ProviderVersion), Endpoint: nullableTextArg(config.Endpoint), Model: nullableTextArg(config.Model), ManifestSchemaVersion: params.Projection.SchemaVersion, RetryPolicyVersion: config.RetryPolicyVersion, MaxProviderAttempts: config.MaxProviderAttempts, MaxBatchGenerations: config.MaxBatchGenerations, BatchMaxRequests: config.BatchMaxRequests, BatchMaxBytes: config.BatchMaxBytes, CandidateCount: acceptedCount, CompletedCount: completedCount, TranslationCompletedAt: pgTimeArgPtr(translationCompletedAt), RenderInputVersion: cardexport.RenderInputVersion, PresentationVersion: cardexport.PresentationVersion})
}

func insertPreparedDeckManifestRow(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, runID, digest string, selectedCount, acceptedCount, omittedCount int) error {
	return sqlcgen.New(tx).InsertPreparedDeckManifest(ctx, sqlcgen.InsertPreparedDeckManifestParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, SchemaVersion: params.Projection.SchemaVersion, ManifestDigest: digest, DeckName: params.Projection.DeckName, Filename: params.Projection.Filename, SelectedCount: selectedCount, AcceptedCount: acceptedCount, OmittedCount: omittedCount})
}

func insertPreparedDeckManifestItems(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, config PreparedDeckRunConfig, candidateDigests []string, runID string, pending map[int]cardexport.ManifestItem) error {
	q := sqlcgen.New(tx)
	for index, item := range params.Projection.Items {
		if err := insertPreparedDeckManifestItem(ctx, q, params, config, candidateDigests[index], runID, item, pending); err != nil {
			return err
		}
	}
	return nil
}

func insertPreparedDeckManifestItem(ctx context.Context, q *sqlcgen.Queries, params FreezePreparedDeckRunParams, config PreparedDeckRunConfig, candidateDigest, runID string, item cardexport.ManifestItem, pending map[int]cardexport.ManifestItem) error {
	renderPayload, err := json.Marshal(preparedDeckRenderPayload{Morphology: item.Entry.Morphology, Gloss: item.Entry.Gloss, Plural: item.Entry.Plural, IPA: item.Entry.IPA, PrincipalParts: item.Entry.PrincipalParts, DictionaryProviderVersion: item.Entry.DictionaryProviderVersion, OmittedEvidenceCount: item.Entry.OmittedEvidenceCount, CandidateSenses: item.Entry.CandidateSenses, SentenceTokens: item.Entry.SentenceTokens, SourceDocument: item.Entry.SourceDocument, Notes: item.Entry.Notes})
	if err != nil {
		return err
	}
	var provider, providerVersion, sentenceHash pgtype.Text
	if item.CacheKey != nil {
		provider, providerVersion, sentenceHash = nullableTextArg(item.CacheKey.Provider), nullableTextArg(item.CacheKey.ProviderVersion), nullableTextArg(item.CacheKey.SentenceHash)
	}
	meaningEvidenceHash := pgtype.Text{}
	if item.CacheKey != nil && item.CacheKey.MeaningEvidenceHash != "" {
		meaningEvidenceHash = nullableTextArg(item.CacheKey.MeaningEvidenceHash)
	}
	if err := q.InsertPreparedDeckManifestItem(ctx, sqlcgen.InsertPreparedDeckManifestItemParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, Ordinal: item.Ordinal, Disposition: string(item.Disposition), Language: item.Entry.Language, TargetLanguage: config.TargetLanguage, CanonicalLemma: item.Entry.CanonicalLemma, Upos: item.Entry.UPOS, SourceSentence: item.Entry.Sentence, TestedTarget: item.Entry.TargetWord, FirstEncounter: item.Entry.FirstEncounter, QualityScore: item.Quality.Score, QualityGdexScore: item.Quality.GDEXScore, QualityReasons: item.Quality.Reasons, RenderPayload: renderPayload, Provider: provider, ProviderVersion: providerVersion, SentenceHash: sentenceHash, CandidateDigest: candidateDigest, CorpusID: nullableUUIDArg(item.CorpusID), SentenceOrdinal: nullableInt8Arg(item.CorpusID, item.SentenceOrdinal), MeaningEvidenceHash: meaningEvidenceHash}); err != nil {
		return err
	}
	if item.Disposition != cardexport.ManifestAccepted {
		return nil
	}
	state, cacheHits := domain.PreparedDeckOutcomeCompleted, 0
	terminalAt := time.Now().UTC()
	if _, waiting := pending[item.Ordinal]; waiting {
		state, terminalAt = domain.PreparedDeckOutcomePending, time.Time{}
	} else if item.CacheKey != nil {
		cacheHits = 1
	}
	var terminalAtPtr *time.Time
	if !terminalAt.IsZero() {
		terminalAtPtr = &terminalAt
	}
	return q.InsertPreparedDeckTranslationOutcome(ctx, sqlcgen.InsertPreparedDeckTranslationOutcomeParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, Ordinal: item.Ordinal, State: string(state), MaxProviderAttempts: config.MaxProviderAttempts, TerminalAt: pgTimeArgPtr(terminalAtPtr), CacheHitCount: cacheHits})
}

func preparedDeckPendingOrdinals(items []cardexport.ManifestItem, pending map[int]cardexport.ManifestItem) []int {
	ordinals := make([]int, 0, len(pending))
	for _, item := range items {
		if _, ok := pending[item.Ordinal]; ok {
			ordinals = append(ordinals, item.Ordinal)
		}
	}
	return ordinals
}

func insertPreparedDeckChunkPlans(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, candidateDigests []string, runID string) ([]domain.PreparedDeckBatchChunk, error) {
	chunks := make([]domain.PreparedDeckBatchChunk, 0, len(params.Chunks))
	for _, plan := range params.Chunks {
		chunkID := plan.ID
		if chunkID == "" {
			chunkID = uuid.NewString()
		}
		first, last := plan.Ordinals[0], plan.Ordinals[len(plan.Ordinals)-1]
		err := sqlcgen.New(tx).InsertPreparedDeckBatchChunk(ctx, sqlcgen.InsertPreparedDeckBatchChunkParams{ID: chunkID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, ChunkIndex: plan.ChunkIndex, Generation: plan.Generation, Model: plan.Model, Endpoint: plan.Endpoint, SplitReason: plan.SplitReason, FirstOrdinal: first, LastOrdinal: last, InputDigest: plan.InputDigest, RequestCount: len(plan.Ordinals), InputBytes: plan.InputBytes, EstimatedPromptTokens: plan.EstimatedPromptTokens})
		if err != nil {
			return nil, err
		}
		for position, ordinal := range plan.Ordinals {
			if err = sqlcgen.New(tx).InsertPreparedDeckBatchChunkItem(ctx, sqlcgen.InsertPreparedDeckBatchChunkItemParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, ChunkID: chunkID, Generation: plan.Generation, Position: position, Ordinal: ordinal, CandidateDigest: candidateDigests[ordinal]}); err != nil {
				return nil, err
			}
		}
		chunks = append(chunks, domain.PreparedDeckBatchChunk{ID: chunkID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, ChunkIndex: plan.ChunkIndex, Generation: plan.Generation, State: domain.PreparedDeckBatchPending, Model: plan.Model, Endpoint: plan.Endpoint, SplitReason: plan.SplitReason, FirstOrdinal: first, LastOrdinal: last, InputDigest: plan.InputDigest, RequestCount: len(plan.Ordinals), InputBytes: plan.InputBytes, EstimatedPromptTokens: plan.EstimatedPromptTokens, Ordinals: append([]int(nil), plan.Ordinals...)})
	}
	return chunks, nil
}

func validatePreparedDeckRunConfig(config PreparedDeckRunConfig) (PreparedDeckRunConfig, error) {
	if config.ExecutionMode == "" {
		config.ExecutionMode = "batch"
	}
	if config.TargetLanguage == "" {
		config.TargetLanguage = "en"
	}
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
	if (config.ExecutionMode != "standard" && config.ExecutionMode != "batch") || config.TargetLanguage != "en" {
		return config, fmt.Errorf("%w: invalid frozen execution identity", ErrInvalidTransition)
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
	return err == nil && run.ExecutionMode == domain.PreparedDeckExecutionMode(validated.ExecutionMode) && run.TargetLanguage == validated.TargetLanguage && run.ExternalTranslationConsent == validated.ExternalTranslationConsent && run.ExternalTranslationConfigured == validated.ExternalTranslationConfigured && run.ContextMode == validated.ContextMode && run.Provider == validated.Provider && run.ProviderVersion == validated.ProviderVersion && run.Endpoint == validated.Endpoint && run.Model == validated.Model && run.RetryPolicyVersion == validated.RetryPolicyVersion && run.MaxProviderAttempts == validated.MaxProviderAttempts && run.MaxBatchGenerations == validated.MaxBatchGenerations && run.BatchMaxRequests == validated.BatchMaxRequests && run.BatchMaxBytes == validated.BatchMaxBytes
}

func getPreparedDeckRun(ctx context.Context, q sqlcgen.DBTX, owner, preparationID, runID string) (domain.PreparedDeckRun, error) {
	model, err := sqlcgen.New(q).GetPreparedDeckRun(ctx, sqlcgen.GetPreparedDeckRunParams{OwnerID: owner, PreparationID: preparationID, ID: runID})
	if err != nil {
		return domain.PreparedDeckRun{}, missing(err)
	}
	return preparedDeckRunFromModel(model), nil
}

func (s *PostgresStore) GetPreparedDeckRun(ctx context.Context, owner, preparationID, runID string) (domain.PreparedDeckRun, error) {
	return getPreparedDeckRun(ctx, s.pool, owner, preparationID, runID)
}

func (s *PostgresStore) GetCurrentPreparedDeckRun(ctx context.Context, owner, preparationID string) (domain.PreparedDeckRun, error) {
	model, err := sqlcgen.New(s.pool).GetCurrentPreparedDeckRun(ctx, sqlcgen.GetCurrentPreparedDeckRunParams{OwnerID: owner, PreparationID: preparationID})
	if err != nil {
		return domain.PreparedDeckRun{}, missing(err)
	}
	return preparedDeckRunFromModel(model), nil
}

// LoadPreparedDeckStorageProjection loads the normalized durable projection
// without turning it into a presentation manifest or enrichment result.
func (s *PostgresStore) LoadPreparedDeckStorageProjection(ctx context.Context, owner, preparationID, runID string) (cardexport.StorageProjection, string, error) {
	var snapshot cardexport.ManifestSnapshot
	manifest, err := sqlcgen.New(s.pool).GetPreparedDeckManifest(ctx, sqlcgen.GetPreparedDeckManifestParams{OwnerID: owner, PreparationID: preparationID, RunID: runID})
	if err != nil {
		return snapshot, "", missing(err)
	}
	snapshot.Owner = owner
	snapshot.SchemaVersion = manifest.SchemaVersion
	snapshot.DeckName = manifest.DeckName
	snapshot.Filename = manifest.Filename
	storedDigest := manifest.ManifestDigest
	selectedCount, acceptedCount, omittedCount := manifest.SelectedCount, manifest.AcceptedCount, manifest.OmittedCount
	items, err := sqlcgen.New(s.pool).ListPreparedDeckManifestItems(ctx, sqlcgen.ListPreparedDeckManifestItemsParams{OwnerID: owner, PreparationID: preparationID, RunID: runID})
	if err != nil {
		return snapshot, "", err
	}
	candidateDigests := make([]string, 0, len(items))
	for _, model := range items {
		var item cardexport.ManifestItem
		item.Ordinal = model.Ordinal
		item.Disposition = cardexport.ManifestDisposition(model.Disposition)
		item.Entry.Language, item.Entry.CanonicalLemma, item.Entry.UPOS = model.Language, model.CanonicalLemma, model.Upos
		item.Entry.Sentence, item.Entry.TargetWord, item.Entry.FirstEncounter = model.SourceSentence, model.TestedTarget, model.FirstEncounter
		item.Quality.Score, item.Quality.GDEXScore, item.Quality.Reasons = model.QualityScore, model.QualityGdexScore, model.QualityReasons
		item.Quality.Accepted = item.Disposition == cardexport.ManifestAccepted
		var render preparedDeckRenderPayload
		if err = json.Unmarshal(model.RenderPayload, &render); err != nil {
			return snapshot, "", fmt.Errorf("%w: decode durable manifest render payload: %w", ErrPreparedDeckIdentity, err)
		}
		item.Entry.Morphology, item.Entry.Gloss, item.Entry.Plural, item.Entry.IPA, item.Entry.PrincipalParts, item.Entry.DictionaryProviderVersion = render.Morphology, render.Gloss, render.Plural, render.IPA, render.PrincipalParts, render.DictionaryProviderVersion
		item.Entry.CandidateSenses = render.CandidateSenses
		item.Entry.OmittedEvidenceCount = render.OmittedEvidenceCount
		item.Entry.SentenceTokens = render.SentenceTokens
		item.Entry.SourceDocument, item.Entry.Notes = render.SourceDocument, render.Notes
		item.CorpusID = uuidString(model.CorpusID)
		item.SentenceOrdinal = pgInt8(model.SentenceOrdinal)
		item.Entry.CorpusID = item.CorpusID
		item.Entry.SentenceOrdinal = item.SentenceOrdinal
		provider := pgText(model.Provider)
		if provider != "" {
			item.CacheKey = &enrichment.CacheKey{Language: item.Entry.Language, TargetLanguage: model.TargetLanguage, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, Provider: provider, ProviderVersion: pgText(model.ProviderVersion), DictionaryProviderVersion: item.Entry.DictionaryProviderVersion, SentenceHash: pgText(model.SentenceHash), MeaningEvidenceHash: pgText(model.MeaningEvidenceHash)}
		}
		candidateDigests = append(candidateDigests, model.CandidateDigest)
		snapshot.Items = append(snapshot.Items, item)
	}
	selected, accepted, omitted := snapshot.Counts()
	if selected != selectedCount || accepted != acceptedCount || omitted != omittedCount {
		return snapshot, "", ErrPreparedDeckIdentity
	}
	calculatedManifest, calculatedCandidates, err := snapshot.Digests()
	if err != nil {
		return snapshot, "", err
	}
	if calculatedManifest != storedDigest || len(calculatedCandidates) != len(candidateDigests) {
		return snapshot, "", ErrPreparedDeckIdentity
	}
	for i := range calculatedCandidates {
		if calculatedCandidates[i] != candidateDigests[i] {
			return snapshot, "", ErrPreparedDeckIdentity
		}
	}
	return snapshot, storedDigest, nil
}

func (s *PostgresStore) PreparedDeckRunProgress(ctx context.Context, owner, preparationID, runID string) (domain.PreparedDeckRunProgress, error) {
	exists, err := s.queries().PreparedDeckRunExists(ctx, sqlcgen.PreparedDeckRunExistsParams{Owner: owner, Preparation: preparationID, Run: runID})
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	if !exists {
		return domain.PreparedDeckRunProgress{}, ErrNotFound
	}
	row, err := s.queries().GetPreparedDeckRunProgress(ctx, sqlcgen.GetPreparedDeckRunProgressParams{Owner: owner, Preparation: preparationID, Run: runID})
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	count := func(name string, value int64) (int, error) {
		converted, conversionErr := checked.IntFromInt64(value)
		if conversionErr != nil {
			return 0, fmt.Errorf("invalid %s: %w", name, conversionErr)
		}
		return converted, nil
	}
	candidateCount, err := count("candidate count", row.CandidateCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	pendingCount, err := count("pending count", row.PendingCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	runningCount, err := count("running count", row.RunningCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	completedCount, err := count("completed count", row.CompletedCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	failedCount, err := count("failed count", row.FailedCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	cancelledCount, err := count("cancelled count", row.CancelledCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	retryingCount, err := count("retrying count", row.RetryingCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	manifestOmissions, err := count("manifest omission count", row.ManifestOmissions)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchChunkCount, err := count("batch chunk count", row.BatchChunkCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchSubmittedChunks, err := count("submitted batch chunk count", row.BatchSubmittedChunks)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchPollingChunks, err := count("polling batch chunk count", row.BatchPollingChunks)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchReconcilingChunks, err := count("reconciling batch chunk count", row.BatchReconcilingChunks)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchCompletedChunks, err := count("completed batch chunk count", row.BatchCompletedChunks)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchFailedChunks, err := count("failed batch chunk count", row.BatchFailedChunks)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchCancelledChunks, err := count("cancelled batch chunk count", row.BatchCancelledChunks)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchRequestCount, err := count("batch request count", row.BatchRequestCount)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchCompletedRequests, err := count("completed batch request count", row.BatchCompletedRequests)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchFailedRequests, err := count("failed batch request count", row.BatchFailedRequests)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	batchExpiredRequests, err := count("expired batch request count", row.BatchExpiredRequests)
	if err != nil {
		return domain.PreparedDeckRunProgress{}, err
	}
	progress := domain.PreparedDeckRunProgress{
		CandidateCount: candidateCount, PendingCount: pendingCount, RunningCount: runningCount,
		CompletedCount: completedCount, FailedCount: failedCount, CancelledCount: cancelledCount,
		RetryingCount: retryingCount, ManifestOmissions: manifestOmissions,
		BatchChunkCount: batchChunkCount, BatchSubmittedChunks: batchSubmittedChunks,
		BatchPollingChunks: batchPollingChunks, BatchReconcilingChunks: batchReconcilingChunks,
		BatchCompletedChunks: batchCompletedChunks, BatchFailedChunks: batchFailedChunks,
		BatchCancelledChunks: batchCancelledChunks, BatchRequestCount: batchRequestCount,
		BatchCompletedRequests: batchCompletedRequests, BatchFailedRequests: batchFailedRequests,
		BatchExpiredRequests: batchExpiredRequests, BatchInputTokens: row.BatchInputTokens,
		BatchOutputTokens: row.BatchOutputTokens,
	}
	if row.BatchSubmittedEpoch > 0 {
		submittedAt := time.Unix(0, int64(row.BatchSubmittedEpoch*float64(time.Second)))
		if time.Now().After(submittedAt) {
			progress.BatchAge = time.Since(submittedAt)
		}
	}
	return progress, nil
}
