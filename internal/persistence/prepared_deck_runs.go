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
	config, manifestWork, err := validateNewPreparedDeckRunFreeze(ctx, tx, params, freezeState)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	runNumberValue, err := sqlcgen.New(tx).NextPreparedDeckRunNumber(ctx, sqlcgen.NextPreparedDeckRunNumberParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID})
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	runID, err := resolvePreparedDeckRunID(params.RunID)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	runFreeze := preparedDeckRunFreeze{runID: runID, runNumber: int(runNumberValue)}
	runFreeze.state, runFreeze.translationState, runFreeze.translationCompletedAt = preparedDeckRunInitialStates(manifestWork.pending)
	chunks, err := writePreparedDeckRunFreeze(ctx, tx, params, config, candidateDigests, digest, runFreeze, manifestWork)
	if err != nil {
		return FreezePreparedDeckRunResult{}, err
	}
	run, err := getPreparedDeckRun(ctx, tx, params.OwnerID, params.PreparationID, runID)
	pendingOrdinals := preparedDeckPendingOrdinals(params.Projection.Items, manifestWork.pending)
	return FreezePreparedDeckRunResult{Run: run, ManifestDigest: digest, Chunks: chunks, PendingOrdinals: pendingOrdinals, NeedsFinalizer: runFreeze.state == domain.PreparedDeckRunFinalizing}, err
}

// validateNewPreparedDeckRunFreeze checks everything that must hold before a new
// run may be written: an accepting preparation, a valid frozen configuration,
// and a manifest whose shape, cache identity and Batch plans match it.
func validateNewPreparedDeckRunFreeze(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, freezeState sqlcgen.GetDeckPreparationFreezeStateRow) (PreparedDeckRunConfig, preparedDeckManifestWork, error) {
	preparationState := domain.DeckPreparationState(freezeState.State)
	if preparationState != domain.DeckPreparationQueued && preparationState != domain.DeckPreparationPreparing {
		return PreparedDeckRunConfig{}, preparedDeckManifestWork{}, ErrInvalidTransition
	}
	config, err := validatePreparedDeckRunConfig(params.Config)
	if err != nil {
		return PreparedDeckRunConfig{}, preparedDeckManifestWork{}, err
	}
	if err := validatePreparedDeckManifestShape(params.Projection, params.OwnerID, freezeState.Filename); err != nil {
		return PreparedDeckRunConfig{}, preparedDeckManifestWork{}, err
	}
	manifestWork, err := classifyPreparedDeckManifest(ctx, tx, params.Projection)
	if err != nil {
		return PreparedDeckRunConfig{}, preparedDeckManifestWork{}, err
	}
	if err := validatePreparedDeckManifestCacheIdentity(params.Projection, config); err != nil {
		return PreparedDeckRunConfig{}, preparedDeckManifestWork{}, err
	}
	if err := validatePreparedDeckRunChunks(params.Chunks, config, manifestWork.pending); err != nil {
		return PreparedDeckRunConfig{}, preparedDeckManifestWork{}, err
	}
	return config, manifestWork, nil
}

// validatePreparedDeckRunChunks applies the Batch/standard split: Batch runs
// validate their chunk plans, and standard runs must carry none.
func validatePreparedDeckRunChunks(chunks []PreparedDeckBatchChunkPlan, config PreparedDeckRunConfig, pending map[int]cardexport.ManifestItem) error {
	if config.ExecutionMode == string(domain.PreparedDeckExecutionBatch) {
		return validateChunkPlans(chunks, config, pending)
	}
	if len(chunks) != 0 {
		return fmt.Errorf("%w: standard runs cannot contain Batch chunks", ErrImmutable)
	}
	return nil
}

// resolvePreparedDeckRunID keeps a caller-supplied durable identity stable and
// mints one otherwise, rejecting anything that is not a canonical UUID.
func resolvePreparedDeckRunID(runID string) (string, error) {
	if runID == "" {
		return uuid.NewString(), nil
	}
	if parsed, err := uuid.Parse(runID); err != nil || parsed.String() != runID {
		return "", fmt.Errorf("%w: invalid durable run identity", ErrImmutable)
	}
	return runID, nil
}

// preparedDeckRunFreeze carries the durable identity and initial state of the
// run row being written.
type preparedDeckRunFreeze struct {
	runID                  string
	runNumber              int
	state                  domain.PreparedDeckRunState
	translationState       domain.PreparedDeckTranslationState
	translationCompletedAt *time.Time
}

// preparedDeckRunInitialStates selects the run's starting state: a run with no
// provider work to do starts finalizing, otherwise it starts translating.
func preparedDeckRunInitialStates(pending map[int]cardexport.ManifestItem) (domain.PreparedDeckRunState, domain.PreparedDeckTranslationState, *time.Time) {
	if len(pending) == 0 {
		completedAt := time.Now().UTC()
		return domain.PreparedDeckRunFinalizing, domain.PreparedDeckTranslationCompleted, &completedAt
	}
	return domain.PreparedDeckRunTranslating, domain.PreparedDeckTranslationPending, nil
}

// writePreparedDeckRunFreeze writes the run, its immutable manifest and items,
// its Batch chunks, and the current-run pointer in the caller's transaction.
func writePreparedDeckRunFreeze(ctx context.Context, tx pgx.Tx, params FreezePreparedDeckRunParams, config PreparedDeckRunConfig, candidateDigests []string, digest string, freeze preparedDeckRunFreeze, manifestWork preparedDeckManifestWork) ([]domain.PreparedDeckBatchChunk, error) {
	selectedCount, acceptedCount, omittedCount := params.Projection.Counts()
	if err := insertPreparedDeckRunRow(ctx, tx, params, config, freeze.runID, freeze.runNumber, freeze.state, freeze.translationState, acceptedCount, manifestWork.completedCount, freeze.translationCompletedAt); err != nil {
		return nil, err
	}
	if err := insertPreparedDeckManifestRow(ctx, tx, params, freeze.runID, digest, selectedCount, acceptedCount, omittedCount); err != nil {
		return nil, err
	}
	if err := insertPreparedDeckManifestItems(ctx, tx, params, config, candidateDigests, freeze.runID, manifestWork.pending); err != nil {
		return nil, err
	}
	chunks, err := insertPreparedDeckChunkPlans(ctx, tx, params, candidateDigests, freeze.runID)
	if err != nil {
		return nil, err
	}
	if err := sqlcgen.New(tx).AttachPreparedDeckRun(ctx, sqlcgen.AttachPreparedDeckRunParams{OwnerID: params.OwnerID, ID: params.PreparationID, CurrentRunID: uuidArg(freeze.runID)}); err != nil {
		return nil, err
	}
	return chunks, nil
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
		if err := validatePreparedDeckManifestCacheItem(item, config, requested); err != nil {
			return err
		}
	}
	return nil
}

// validatePreparedDeckManifestCacheItem checks one accepted manifest item's
// frozen cache identity against the requested run configuration.
func validatePreparedDeckManifestCacheItem(item cardexport.ManifestItem, config PreparedDeckRunConfig, requested bool) error {
	if requested != (item.CacheKey != nil) {
		return fmt.Errorf("%w: manifest cache identity contradicts run configuration", ErrImmutable)
	}
	if item.CacheKey != nil && (item.CacheKey.Provider != config.Provider || item.CacheKey.ProviderVersion != config.ProviderVersion) {
		return fmt.Errorf("%w: manifest cache identity contradicts provider snapshot", ErrImmutable)
	}
	if item.CacheKey != nil && item.CacheKey.MeaningEvidenceHash != "" && item.CacheKey.MeaningEvidenceHash != enrichment.MeaningEvidenceHash(item.Entry.CandidateSenses) {
		return fmt.Errorf("%w: manifest cache identity contradicts frozen meaning evidence", ErrImmutable)
	}
	if item.CacheKey != nil && ((config.ContextMode == "sentence" && item.CacheKey.SentenceHash != enrichment.SentenceHash(item.Entry.Sentence)) || (config.ContextMode == "lemma_only" && item.CacheKey.SentenceHash != "")) {
		return fmt.Errorf("%w: manifest cache identity contradicts context mode", ErrImmutable)
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
			if err := sqlcgen.New(tx).InsertPreparedDeckBatchChunkItem(ctx, sqlcgen.InsertPreparedDeckBatchChunkItemParams{OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, ChunkID: chunkID, Generation: plan.Generation, Position: position, Ordinal: ordinal, CandidateDigest: candidateDigests[ordinal]}); err != nil {
				return nil, err
			}
		}
		chunks = append(chunks, domain.PreparedDeckBatchChunk{ID: chunkID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: runID, ChunkIndex: plan.ChunkIndex, Generation: plan.Generation, State: domain.PreparedDeckBatchPending, Model: plan.Model, Endpoint: plan.Endpoint, SplitReason: plan.SplitReason, FirstOrdinal: first, LastOrdinal: last, InputDigest: plan.InputDigest, RequestCount: len(plan.Ordinals), InputBytes: plan.InputBytes, EstimatedPromptTokens: plan.EstimatedPromptTokens, Ordinals: append([]int(nil), plan.Ordinals...)})
	}
	return chunks, nil
}

func validatePreparedDeckRunConfig(config PreparedDeckRunConfig) (PreparedDeckRunConfig, error) {
	config = applyPreparedDeckRunConfigDefaults(config)
	if err := validatePreparedDeckExecutionIdentity(config); err != nil {
		return config, err
	}
	if err := validatePreparedDeckTranslationIdentity(config); err != nil {
		return config, err
	}
	if err := validatePreparedDeckRunLimits(config); err != nil {
		return config, err
	}
	return config, nil
}

// applyPreparedDeckRunConfigDefaults fills every optional field a caller may
// leave zero, so a validated config is always fully frozen.
func applyPreparedDeckRunConfigDefaults(config PreparedDeckRunConfig) PreparedDeckRunConfig {
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
	return config
}

// validatePreparedDeckExecutionIdentity requires a known execution mode and the
// only supported target language.
func validatePreparedDeckExecutionIdentity(config PreparedDeckRunConfig) error {
	if (config.ExecutionMode != "standard" && config.ExecutionMode != "batch") || config.TargetLanguage != "en" {
		return fmt.Errorf("%w: invalid frozen execution identity", ErrInvalidTransition)
	}
	return nil
}

// validatePreparedDeckTranslationIdentity requires a complete provider identity
// exactly when external translation was consented to and configured.
func validatePreparedDeckTranslationIdentity(config PreparedDeckRunConfig) error {
	requested := config.ExternalTranslationConsent && config.ExternalTranslationConfigured
	if requested {
		if (config.ContextMode != "sentence" && config.ContextMode != "lemma_only") || config.Provider == "" || config.ProviderVersion == "" || config.Endpoint != "/v1/chat/completions" || config.Model == "" {
			return fmt.Errorf("%w: incomplete external translation identity", ErrInvalidTransition)
		}
		return nil
	}
	if config.ContextMode != "" || config.Provider != "" || config.ProviderVersion != "" || config.Endpoint != "" || config.Model != "" {
		return fmt.Errorf("%w: disabled external translation has provider identity", ErrInvalidTransition)
	}
	return nil
}

// validatePreparedDeckRunLimits requires every durable run limit to sit inside
// the supported range.
func validatePreparedDeckRunLimits(config PreparedDeckRunConfig) error {
	if config.RetryPolicyVersion < 1 || config.MaxProviderAttempts < 1 || config.MaxBatchGenerations < 1 || config.MaxBatchGenerations > MaximumBatchGenerations || config.BatchMaxRequests < 1 || config.BatchMaxRequests > 50000 || config.BatchMaxBytes < 1 || config.BatchMaxBytes > DefaultBatchMaxBytes {
		return fmt.Errorf("%w: invalid durable run limits", ErrInvalidTransition)
	}
	return nil
}

func validateChunkPlans(plans []PreparedDeckBatchChunkPlan, config PreparedDeckRunConfig, pending map[int]cardexport.ManifestItem) error {
	covered := make(map[int]struct{}, len(pending))
	identities := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		if err := validatePreparedDeckChunkPlan(plan, config); err != nil {
			return err
		}
		identity := fmt.Sprintf("%d/%d", plan.Generation, plan.ChunkIndex)
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("%w: duplicate Batch chunk identity", ErrImmutable)
		}
		identities[identity] = struct{}{}
		if err := validatePreparedDeckChunkOrdinals(plan.Ordinals, pending, covered); err != nil {
			return err
		}
	}
	if len(covered) != len(pending) {
		return fmt.Errorf("%w: Batch chunks do not cover pending items", ErrImmutable)
	}
	return nil
}

// validatePreparedDeckChunkPlan checks one Batch chunk plan's shape, split
// reason, input digest and frozen limits.
func validatePreparedDeckChunkPlan(plan PreparedDeckBatchChunkPlan, config PreparedDeckRunConfig) error {
	if !preparedDeckChunkSplitReasonValid(plan.SplitReason) || !preparedDeckChunkDigestValid(plan.InputDigest) || !preparedDeckChunkPlanWithinLimits(plan, config) {
		return fmt.Errorf("%w: invalid Batch chunk plan", ErrInvalidTransition)
	}
	return nil
}

// preparedDeckChunkSplitReasonValid reports whether a chunk carries a known
// split reason.
func preparedDeckChunkSplitReasonValid(reason string) bool {
	switch reason {
	case "run", "request_limit", "byte_limit", "token_limit", "retry":
		return true
	default:
		return false
	}
}

// preparedDeckChunkDigestValid reports whether a chunk carries a canonical
// lower-case sha256 input digest.
func preparedDeckChunkDigestValid(digest string) bool {
	digestBytes, err := hex.DecodeString(digest)
	return err == nil && len(digestBytes) == 32 && digest == strings.ToLower(digest)
}

// preparedDeckChunkPlanWithinLimits reports whether a chunk fits the frozen
// request, byte and generation limits and the frozen provider identity.
func preparedDeckChunkPlanWithinLimits(plan PreparedDeckBatchChunkPlan, config PreparedDeckRunConfig) bool {
	return len(plan.Ordinals) > 0 && len(plan.Ordinals) <= config.BatchMaxRequests && plan.InputBytes >= 1 && plan.InputBytes <= config.BatchMaxBytes && plan.Generation >= 1 && plan.Generation <= config.MaxBatchGenerations && plan.ChunkIndex >= 0 && plan.Model == config.Model && plan.Endpoint == config.Endpoint
}

// validatePreparedDeckChunkOrdinals checks one plan's ordinals are strictly
// ascending, pending, and not yet claimed by another plan.
func validatePreparedDeckChunkOrdinals(ordinals []int, pending map[int]cardexport.ManifestItem, covered map[int]struct{}) error {
	previous := -1
	for _, ordinal := range ordinals {
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
	return preparedDeckRunProgressFromRow(row)
}

// preparedDeckRunProgressFromRow narrows every count and derives the batch age
// from the raw progress row.
func preparedDeckRunProgressFromRow(row sqlcgen.GetPreparedDeckRunProgressRow) (domain.PreparedDeckRunProgress, error) {
	var progress domain.PreparedDeckRunProgress
	fields := [...]struct {
		name  string
		value int64
		dest  *int
	}{
		{"candidate count", row.CandidateCount, &progress.CandidateCount},
		{"pending count", row.PendingCount, &progress.PendingCount},
		{"running count", row.RunningCount, &progress.RunningCount},
		{"completed count", row.CompletedCount, &progress.CompletedCount},
		{"failed count", row.FailedCount, &progress.FailedCount},
		{"cancelled count", row.CancelledCount, &progress.CancelledCount},
		{"retrying count", row.RetryingCount, &progress.RetryingCount},
		{"manifest omission count", row.ManifestOmissions, &progress.ManifestOmissions},
		{"batch chunk count", row.BatchChunkCount, &progress.BatchChunkCount},
		{"submitted batch chunk count", row.BatchSubmittedChunks, &progress.BatchSubmittedChunks},
		{"polling batch chunk count", row.BatchPollingChunks, &progress.BatchPollingChunks},
		{"reconciling batch chunk count", row.BatchReconcilingChunks, &progress.BatchReconcilingChunks},
		{"completed batch chunk count", row.BatchCompletedChunks, &progress.BatchCompletedChunks},
		{"failed batch chunk count", row.BatchFailedChunks, &progress.BatchFailedChunks},
		{"cancelled batch chunk count", row.BatchCancelledChunks, &progress.BatchCancelledChunks},
		{"batch request count", row.BatchRequestCount, &progress.BatchRequestCount},
		{"completed batch request count", row.BatchCompletedRequests, &progress.BatchCompletedRequests},
		{"failed batch request count", row.BatchFailedRequests, &progress.BatchFailedRequests},
		{"expired batch request count", row.BatchExpiredRequests, &progress.BatchExpiredRequests},
	}
	for _, field := range fields {
		converted, err := checked.IntFromInt64(field.value)
		if err != nil {
			return domain.PreparedDeckRunProgress{}, fmt.Errorf("invalid %s: %w", field.name, err)
		}
		*field.dest = converted
	}
	progress.BatchInputTokens, progress.BatchOutputTokens = row.BatchInputTokens, row.BatchOutputTokens
	applyPreparedDeckBatchAge(&progress, row.BatchSubmittedEpoch)
	return progress, nil
}

// applyPreparedDeckBatchAge records how long the oldest submitted chunk has
// waited. An unsubmitted run has a zero epoch and no age.
func applyPreparedDeckBatchAge(progress *domain.PreparedDeckRunProgress, submittedEpoch float64) {
	if submittedEpoch <= 0 {
		return
	}
	submittedAt := time.Unix(0, int64(submittedEpoch*float64(time.Second)))
	if time.Now().After(submittedAt) {
		progress.BatchAge = time.Since(submittedAt)
	}
}
