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
	preparationState := domain.DeckPreparationState(freezeState.State)
	currentRunID := uuidString(freezeState.CurrentRunID)
	preparationFilename := freezeState.Filename
	if errors.Is(err, pgx.ErrNoRows) {
		return FreezePreparedDeckRunResult{}, ErrNotFound
	}
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	digest, err := params.Projection.Digest()
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
		if storedDigest, getErr = sqlcgen.New(tx).GetPreparedDeckManifestDigest(ctx, sqlcgen.GetPreparedDeckManifestDigestParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: currentRunID}); getErr != nil {
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
	if params.Projection.Owner != params.OwnerID || params.Projection.SchemaVersion != cardexport.ManifestSchemaVersion {
		return FreezePreparedDeckRunResult{}, ErrImmutable
	}
	if params.Projection.Filename != preparationFilename {
		return FreezePreparedDeckRunResult{}, fmt.Errorf("%w: manifest filename contradicts preparation", ErrPreparedDeckIdentity)
	}
	config, err := validatePreparedDeckRunConfig(params.Config)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	selectedCount, acceptedCount, omittedCount := params.Projection.Counts()
	pending := make(map[int]cardexport.ManifestItem)
	completedCount := 0
	for _, item := range params.Projection.Items {
		if item.Disposition != cardexport.ManifestAccepted {
			continue
		}
		if item.CacheKey == nil {
			completedCount++
			continue
		}
		key := item.CacheKey
		var found bool
		if found, err = sqlcgen.New(tx).PreparedDeckCacheExists(ctx, sqlcgen.PreparedDeckCacheExistsParams{Language: key.Language, TargetLanguage: key.TargetLanguage, CanonicalLemma: key.CanonicalLemma, Upos: key.UPOS, Provider: key.Provider, ProviderVersion: key.ProviderVersion, SentenceHash: key.SentenceHash, DictionaryProviderVersion: key.DictionaryProviderVersion}); err != nil {
			return FreezePreparedDeckRunResult{}, err
		}
		if found {
			completedCount++
		} else {
			pending[item.Ordinal] = item
		}
	}
	requested := config.ExternalTranslationConsent && config.ExternalTranslationConfigured
	for _, item := range params.Projection.Items {
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
	if config.ExecutionMode == string(domain.PreparedDeckExecutionBatch) {
		if err = validateChunkPlans(params.Chunks, config, pending); err != nil {
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
	if len(pending) == 0 {
		runState, translationState = domain.PreparedDeckRunFinalizing, domain.PreparedDeckTranslationCompleted
		completedAt := time.Now().UTC()
		translationCompletedAt = &completedAt
	}
	if err = sqlcgen.New(tx).InsertPreparedDeckRun(ctx, sqlcgen.InsertPreparedDeckRunParams{ID: runID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunNumber: runNumber, State: string(runState), TranslationState: string(translationState), ExecutionMode: config.ExecutionMode, TargetLanguage: config.TargetLanguage, ExternalTranslationConsent: config.ExternalTranslationConsent, ExternalTranslationConfigured: config.ExternalTranslationConfigured, ContextMode: nullableTextArg(config.ContextMode), Provider: nullableTextArg(config.Provider), ProviderVersion: nullableTextArg(config.ProviderVersion), Endpoint: nullableTextArg(config.Endpoint), Model: nullableTextArg(config.Model), ManifestSchemaVersion: params.Projection.SchemaVersion, RetryPolicyVersion: config.RetryPolicyVersion, MaxProviderAttempts: config.MaxProviderAttempts, MaxBatchGenerations: config.MaxBatchGenerations, BatchMaxRequests: config.BatchMaxRequests, BatchMaxBytes: config.BatchMaxBytes, CandidateCount: acceptedCount, CompletedCount: completedCount, TranslationCompletedAt: pgTimeArgPtr(translationCompletedAt), RenderInputVersion: cardexport.RenderInputVersion, PresentationVersion: cardexport.PresentationVersion}); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if err = sqlcgen.New(tx).InsertPreparedDeckManifest(ctx, sqlcgen.InsertPreparedDeckManifestParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, SchemaVersion: params.Projection.SchemaVersion, ManifestDigest: digest, DeckName: params.Projection.DeckName, Filename: params.Projection.Filename, SelectedCount: selectedCount, AcceptedCount: acceptedCount, OmittedCount: omittedCount}); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	for _, item := range params.Projection.Items {
		candidateDigest, digestErr := cardexport.CandidateDigest(item)
		if digestErr != nil {
			return FreezePreparedDeckRunResult{}, digestErr
		}
		renderPayload, marshalErr := json.Marshal(preparedDeckRenderPayload{Morphology: item.Entry.Morphology, Gloss: item.Entry.Gloss, Plural: item.Entry.Plural, IPA: item.Entry.IPA, PrincipalParts: item.Entry.PrincipalParts, DictionaryProviderVersion: item.Entry.DictionaryProviderVersion, CandidateSenses: item.Entry.CandidateSenses, SentenceTokens: item.Entry.SentenceTokens, SourceDocument: item.Entry.SourceDocument, Notes: item.Entry.Notes})
		if marshalErr != nil {
			return FreezePreparedDeckRunResult{}, marshalErr
		}
		var provider, providerVersion, sentenceHash pgtype.Text
		if item.CacheKey != nil {
			provider, providerVersion, sentenceHash = nullableTextArg(item.CacheKey.Provider), nullableTextArg(item.CacheKey.ProviderVersion), nullableTextArg(item.CacheKey.SentenceHash)
		}
		if err = sqlcgen.New(tx).InsertPreparedDeckManifestItem(ctx, sqlcgen.InsertPreparedDeckManifestItemParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, Ordinal: item.Ordinal, Disposition: string(item.Disposition), Language: item.Entry.Language, TargetLanguage: config.TargetLanguage, CanonicalLemma: item.Entry.CanonicalLemma, Upos: item.Entry.UPOS, SourceSentence: item.Entry.Sentence, TestedTarget: item.Entry.TargetWord, FirstEncounter: item.Entry.FirstEncounter, QualityScore: item.Quality.Score, QualityGdexScore: item.Quality.GDEXScore, QualityReasons: item.Quality.Reasons, RenderPayload: renderPayload, Provider: provider, ProviderVersion: providerVersion, SentenceHash: sentenceHash, CandidateDigest: candidateDigest, CorpusID: nullableUUIDArg(item.CorpusID), SentenceOrdinal: nullableInt8Arg(item.CorpusID, item.SentenceOrdinal)}); err != nil {
			return FreezePreparedDeckRunResult{}, err
		}
		if item.Disposition == cardexport.ManifestAccepted {
			state, cacheHits := domain.PreparedDeckOutcomeCompleted, 0
			var terminalAt *time.Time
			terminal := time.Now().UTC()
			terminalAt = &terminal
			if _, waiting := pending[item.Ordinal]; waiting {
				state, terminalAt = domain.PreparedDeckOutcomePending, nil
			} else if item.CacheKey != nil {
				cacheHits = 1
			}
			if err = sqlcgen.New(tx).InsertPreparedDeckTranslationOutcome(ctx, sqlcgen.InsertPreparedDeckTranslationOutcomeParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, Ordinal: item.Ordinal, State: string(state), MaxProviderAttempts: config.MaxProviderAttempts, TerminalAt: pgTimeArgPtr(terminalAt), CacheHitCount: cacheHits}); err != nil {
				return FreezePreparedDeckRunResult{}, err
			}
		}
	}
	chunks, err := insertPreparedDeckChunkPlans(ctx, tx, params, config, runID, pending)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	if err = sqlcgen.New(tx).AttachPreparedDeckRun(ctx, sqlcgen.AttachPreparedDeckRunParams{OwnerID: params.OwnerID, ID: params.PreparationID, CurrentRunID: uuidArg(runID)}); err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	run, err := getPreparedDeckRun(ctx, tx, params.OwnerID, params.PreparationID, runID)
	pendingOrdinals := make([]int, 0, len(pending))
	for _, item := range params.Projection.Items {
		if _, ok := pending[item.Ordinal]; ok {
			pendingOrdinals = append(pendingOrdinals, item.Ordinal)
		}
	}
	return FreezePreparedDeckRunResult{Run: run, ManifestDigest: digest, Chunks: chunks, PendingOrdinals: pendingOrdinals, NeedsFinalizer: runState == domain.PreparedDeckRunFinalizing}, err
}

func insertPreparedDeckChunkPlans(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, _ PreparedDeckRunConfig, runID string, pending map[int]cardexport.ManifestItem) ([]domain.PreparedDeckBatchChunk, error) {
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
			candidateDigest, digestErr := cardexport.CandidateDigest(pending[ordinal])
			if digestErr != nil {
				return nil, digestErr
			}
			if err = sqlcgen.New(tx).InsertPreparedDeckBatchChunkItem(ctx, sqlcgen.InsertPreparedDeckBatchChunkItemParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, ChunkID: chunkID, Generation: plan.Generation, Position: position, Ordinal: ordinal, CandidateDigest: candidateDigest}); err != nil {
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
	snapshot.SchemaVersion = int(manifest.SchemaVersion)
	snapshot.DeckName = manifest.DeckName
	snapshot.Filename = manifest.Filename
	storedDigest := manifest.ManifestDigest
	selectedCount, acceptedCount, omittedCount := int(manifest.SelectedCount), int(manifest.AcceptedCount), int(manifest.OmittedCount)
	items, err := sqlcgen.New(s.pool).ListPreparedDeckManifestItems(ctx, sqlcgen.ListPreparedDeckManifestItemsParams{OwnerID: owner, PreparationID: preparationID, RunID: runID})
	if err != nil {
		return snapshot, "", err
	}
	for _, model := range items {
		var item cardexport.ManifestItem
		item.Ordinal = int(model.Ordinal)
		item.Disposition = cardexport.ManifestDisposition(model.Disposition)
		item.Entry.Language, item.Entry.CanonicalLemma, item.Entry.UPOS = model.Language, model.CanonicalLemma, model.Upos
		item.Entry.Sentence, item.Entry.TargetWord, item.Entry.FirstEncounter = model.SourceSentence, model.TestedTarget, model.FirstEncounter
		item.Quality.Score, item.Quality.GDEXScore, item.Quality.Reasons = int(model.QualityScore), model.QualityGdexScore, model.QualityReasons
		item.Quality.Accepted = item.Disposition == cardexport.ManifestAccepted
		var render preparedDeckRenderPayload
		if err = json.Unmarshal(model.RenderPayload, &render); err != nil {
			return snapshot, "", fmt.Errorf("%w: decode durable manifest render payload: %v", ErrPreparedDeckIdentity, err)
		}
		item.Entry.Morphology, item.Entry.Gloss, item.Entry.Plural, item.Entry.IPA, item.Entry.PrincipalParts, item.Entry.DictionaryProviderVersion = render.Morphology, render.Gloss, render.Plural, render.IPA, render.PrincipalParts, render.DictionaryProviderVersion
		item.Entry.CandidateSenses = render.CandidateSenses
		item.Entry.SentenceTokens = render.SentenceTokens
		item.Entry.SourceDocument, item.Entry.Notes = render.SourceDocument, render.Notes
		item.CorpusID = uuidString(model.CorpusID)
		item.SentenceOrdinal = pgInt8(model.SentenceOrdinal)
		item.Entry.CorpusID = item.CorpusID
		item.Entry.SentenceOrdinal = item.SentenceOrdinal
		provider := pgText(model.Provider)
		if provider != "" {
			item.CacheKey = &enrichment.CacheKey{Language: item.Entry.Language, TargetLanguage: model.TargetLanguage, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, Provider: provider, ProviderVersion: pgText(model.ProviderVersion), DictionaryProviderVersion: item.Entry.DictionaryProviderVersion, SentenceHash: pgText(model.SentenceHash)}
		}
		calculated, digestErr := cardexport.CandidateDigestVersion(item, snapshot.SchemaVersion)
		if digestErr != nil || calculated != model.CandidateDigest {
			return snapshot, "", ErrPreparedDeckIdentity
		}
		snapshot.Items = append(snapshot.Items, item)
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

// LoadPreparedDeckManifest is retained for orchestration paths that have not
// yet migrated to the projection name. It returns the same durable facts.
func (s *PostgresStore) LoadPreparedDeckManifest(ctx context.Context, owner, preparationID, runID string) (cardexport.ManifestSnapshot, string, error) {
	return s.LoadPreparedDeckStorageProjection(ctx, owner, preparationID, runID)
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
	progress := domain.PreparedDeckRunProgress{
		CandidateCount: int(row.CandidateCount), PendingCount: int(row.PendingCount), RunningCount: int(row.RunningCount),
		CompletedCount: int(row.CompletedCount), FailedCount: int(row.FailedCount), CancelledCount: int(row.CancelledCount),
		RetryingCount: int(row.RetryingCount), ManifestOmissions: int(row.ManifestOmissions),
		BatchChunkCount: int(row.BatchChunkCount), BatchSubmittedChunks: int(row.BatchSubmittedChunks),
		BatchPollingChunks: int(row.BatchPollingChunks), BatchReconcilingChunks: int(row.BatchReconcilingChunks),
		BatchCompletedChunks: int(row.BatchCompletedChunks), BatchFailedChunks: int(row.BatchFailedChunks),
		BatchCancelledChunks: int(row.BatchCancelledChunks), BatchRequestCount: int(row.BatchRequestCount),
		BatchCompletedRequests: int(row.BatchCompletedRequests), BatchFailedRequests: int(row.BatchFailedRequests),
		BatchExpiredRequests: int(row.BatchExpiredRequests), BatchInputTokens: row.BatchInputTokens,
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
