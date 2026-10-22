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

// preparedDeckFactStore is the transaction-scoped read model the planner uses.
type preparedDeckFactStore interface {
	LoadPreparedDeckInputFactsTx(context.Context, pgx.Tx, domain.DeckPreparation) (persistence.PreparedDeckInputFacts, error)
	LoadPreparedDeckCandidateFactsTx(context.Context, pgx.Tx, domain.DeckPreparation, []domain.SelectionCandidate) ([]persistence.PreparedDeckCandidateFacts, error)
}

// deckLanguageReader is an optional input-assembler capability that reports a
// preparation's language before its projections are assembled. It lets a
// source-evidence deck be planned without a configured provider.
type deckLanguageReader interface {
	DeckLanguage(context.Context, pgx.Tx, domain.DeckPreparation) (string, error)
}

// InputAssembler applies recurring-vocabulary selection to transaction-scoped
// persistence facts and returns the projections consumed by Presentation.
type InputAssembler struct {
	Store preparedDeckFactStore
}

func NewInputAssembler(store preparedDeckFactStore) *InputAssembler {
	return &InputAssembler{Store: store}
}

// DeckLanguage reports the language of the preparation's source material when
// the store can resolve it, and the empty string otherwise.
func (a *InputAssembler) DeckLanguage(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) (string, error) {
	if a == nil || a.Store == nil {
		return "", nil
	}
	reader, ok := a.Store.(interface {
		PreparedDeckLanguageTx(context.Context, pgx.Tx, domain.DeckPreparation) (string, error)
	})
	if !ok {
		return "", nil
	}
	return reader.PreparedDeckLanguageTx(ctx, tx, preparation)
}

func (a *InputAssembler) AssemblePreparedDeckInputs(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	if a == nil || a.Store == nil {
		return nil, "", errors.New("prepareddeck: input fact store is unavailable")
	}
	if err := lockDirectDeckVocabularyDecisions(ctx, tx, preparation); err != nil {
		return nil, "", err
	}
	facts, err := a.Store.LoadPreparedDeckInputFactsTx(ctx, tx, preparation)
	if err != nil {
		return nil, "", fmt.Errorf("load prepared deck input facts: %w", err)
	}
	selected, err := selectPreparedDeckCandidates(preparation, facts)
	if err != nil {
		return nil, "", err
	}
	selectedFacts, err := a.Store.LoadPreparedDeckCandidateFactsTx(ctx, tx, preparation, selected)
	if err != nil {
		return nil, "", fmt.Errorf("load selected prepared deck facts: %w", err)
	}
	return candidateProjections(preparation, facts.DeckName, selectedFacts), facts.DeckName, nil
}

// lockDirectDeckVocabularyDecisions serializes a direct deck's vocabulary
// decisions against the Book it belongs to. A preparation bound to a Current
// reading snapshot inherits that snapshot's lock instead.
func lockDirectDeckVocabularyDecisions(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) error {
	if preparation.SnapshotID != "" || tx == nil {
		return nil
	}
	var bookID *string
	if err := tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, preparation.OwnerID, preparation.SourceMaterialID).Scan(&bookID); err != nil {
		return fmt.Errorf("resolve direct-deck Book identity: %w", err)
	}
	if bookID == nil {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, preparation.OwnerID+":"+*bookID); err != nil {
		return fmt.Errorf("lock direct-deck vocabulary decisions: %w", err)
	}
	return nil
}

// selectPreparedDeckCandidates chooses the candidates to freeze: the Current
// reading snapshot when one is active, otherwise the recurring-vocabulary
// survivors of the projected effective candidates.
func selectPreparedDeckCandidates(preparation domain.DeckPreparation, facts persistence.PreparedDeckInputFacts) ([]domain.SelectionCandidate, error) {
	candidates, err := effectivePreparedDeckCandidates(preparation, facts)
	if err != nil {
		return nil, err
	}
	if preparation.SnapshotID != "" && !facts.CurrentReadingSnapshotActive {
		return nil, fmt.Errorf("prepareddeck: Current reading snapshot %q is unavailable", preparation.SnapshotID)
	}
	if facts.CurrentReadingSnapshotActive {
		selected := make([]domain.SelectionCandidate, 0, len(facts.CurrentReadingSnapshot))
		return append(selected, facts.CurrentReadingSnapshot...), nil
	}
	eligibility := selection.NewEligibility(facts.Known, facts.Reserved)
	selected := make([]domain.SelectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !eligibility.Allows(candidate, selection.DefaultRecurringMinOccurrences) {
			continue
		}
		selected = append(selected, candidate)
	}
	return selected, nil
}

// effectivePreparedDeckCandidates projects the immutable analysis when no
// Current reading snapshot is active, and otherwise returns the loaded
// candidates unchanged.
func effectivePreparedDeckCandidates(preparation domain.DeckPreparation, facts persistence.PreparedDeckInputFacts) ([]domain.SelectionCandidate, error) {
	if facts.CurrentReadingSnapshotActive || facts.Analysis == nil {
		return facts.Candidates, nil
	}
	return projectPreparedDeckCandidates(preparation, facts.CorpusID, *facts.Analysis, facts.Corrections, facts.Structure)
}

// candidateProjections materializes the frozen projections consumed by
// Presentation from the selected candidate facts.
func candidateProjections(preparation domain.DeckPreparation, deckName string, selectedFacts []persistence.PreparedDeckCandidateFacts) []cardexport.CandidateProjection {
	projections := make([]cardexport.CandidateProjection, 0, len(selectedFacts))
	for _, fact := range selectedFacts {
		projections = append(projections, cardexport.CandidateProjection{OwnerID: preparation.OwnerID, DeckName: deckName, Candidate: fact.Candidate, Entry: fact.Entry, Sentences: fact.Sentences, RequireContextualGloss: false, ReferenceTranslations: fact.ReferenceTranslations, ReferencePin: fact.ReferencePin})
	}
	return projections
}

func projectPreparedDeckCandidates(preparation domain.DeckPreparation, corpusID string, corpus analyzer.Result, corrections []domain.OccurrenceLemmaCorrection, structure domain.SnapshotStructure) ([]domain.SelectionCandidate, error) {
	decisions, err := occurrenceDecisions(corrections)
	if err != nil {
		return nil, err
	}
	config := selection.DefaultConfig(corpusID)
	config.Structure = structure
	projected, err := selection.Project(corpus, config, decisions)
	if err != nil {
		return nil, fmt.Errorf("project effective prepared-deck vocabulary: %w", err)
	}
	result := make([]domain.SelectionCandidate, 0, len(projected))
	for _, candidate := range projected {
		mapped, err := projectSelectionCandidate(preparation.OwnerID, corpusID, candidate)
		if err != nil {
			return nil, err
		}
		result = append(result, mapped)
	}
	return result, nil
}

// occurrenceDecisions converts learner corrections into occurrence-scoped
// selection decisions, rejecting any correction without a usable source span.
func occurrenceDecisions(corrections []domain.OccurrenceLemmaCorrection) ([]selection.OccurrenceDecision, error) {
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
	return decisions, nil
}

// projectSelectionCandidate freezes one projected candidate into the persisted
// selection identity, marshalling its JSON facts and earliest source offset.
func projectSelectionCandidate(ownerID, corpusID string, candidate selection.Candidate) (domain.SelectionCandidate, error) {
	forms, err := json.Marshal(candidate.ObservedForms)
	if err != nil {
		return domain.SelectionCandidate{}, err
	}
	references, err := json.Marshal(candidate.SentenceReferences)
	if err != nil {
		return domain.SelectionCandidate{}, err
	}
	provenance, err := json.Marshal(candidate.Provenance)
	if err != nil {
		return domain.SelectionCandidate{}, err
	}
	firstEncounter, err := firstEncounterOffset(candidate.SentenceReferences)
	if err != nil {
		return domain.SelectionCandidate{}, err
	}
	return domain.SelectionCandidate{
		OwnerID: ownerID, CorpusID: corpusID,
		Language: candidate.Identity.Language, CanonicalLemma: candidate.Identity.CanonicalLemma, UPOS: candidate.Identity.UPOS,
		OccurrenceCount: candidate.OccurrenceCount, FirstEncounter: firstEncounter,
		ObservedForms: forms, SentenceReferences: references, Provenance: provenance,
	}, nil
}

// firstEncounterOffset returns the earliest sentence offset in the candidate,
// rejecting a source offset PostgreSQL cannot store.
func firstEncounterOffset(references []selection.SentenceReference) (int64, error) {
	firstEncounter := int64(^uint64(0) >> 1)
	for _, reference := range references {
		startOffset, err := checked.Int64FromUint64(reference.Location.StartOffset)
		if err != nil {
			return 0, fmt.Errorf("prepareddeck: source offset exceeds PostgreSQL range: %w", err)
		}
		if startOffset < firstEncounter {
			firstEncounter = startOffset
		}
	}
	return firstEncounter, nil
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
	// Assessments are the recorded personal-use payload assessments the pass is
	// gated against. They default to the compiled reviewed set; a caller may
	// install a recorded assessment to exercise the pass.
	Assessments []enrichment.ProviderAssessment
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

// NewPreparedDeckPlanner returns the planner that selects the executor once,
// when a new run is frozen. DurableCoordinator intentionally bypasses it for an
// existing run.
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
	return p.planPreparedDeckRun(ctx, tx, preparation, domain.PreparedDeckExecutionStandard, p.StandardConfig.StandardMaxAttempts)
}

// errTranslationProviderRequired reports a deck that needs the contextual-gloss
// provider but has none configured.
var errTranslationProviderRequired = errors.New("prepareddeck: a configured translation provider is required for contextual Glosses")

func (p *BatchPlanner) planPreparedDeckRun(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, mode domain.PreparedDeckExecutionMode, standardAttempts int) (persistence.FreezePreparedDeckRunParams, error) {
	if p == nil || p.Assembler == nil || p.Presentation == nil || strings.TrimSpace(preparation.OwnerID) == "" || strings.TrimSpace(preparation.ID) == "" {
		return persistence.FreezePreparedDeckRunParams{}, ErrInvalidInput
	}
	providerConfigured := p.ExternalEnabled && p.Codec != nil
	sourceEvidence := false
	if !providerConfigured {
		// Without a configured provider only a source-evidence Greek deck can be
		// planned, and its language must be resolved before any input work so a
		// modern deck keeps its existing failure.
		language, err := p.deckLanguage(ctx, tx, preparation)
		if err != nil {
			return persistence.FreezePreparedDeckRunParams{}, err
		}
		sourceEvidence = cardexport.IsAncientGreek(language)
		if !sourceEvidence {
			return persistence.FreezePreparedDeckRunParams{}, errTranslationProviderRequired
		}
	}
	projections, deckName, err := p.Assembler.AssemblePreparedDeckInputs(ctx, tx, preparation)
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("assemble prepared deck inputs: %w", err)
	}
	sourceEvidence = sourceEvidence || sourceEvidenceDeck(projections)
	if sourceEvidence {
		return p.planGreekDeck(ctx, preparation, deckName, projections, mode, standardAttempts, providerConfigured)
	}
	if !providerConfigured {
		return persistence.FreezePreparedDeckRunParams{}, errTranslationProviderRequired
	}
	return p.planExternalRun(ctx, tx, preparation, projections, deckName, mode, standardAttempts)
}

// planGreekDeck freezes a source-evidence Ancient Greek deck. Like the LLM for
// modern languages, the evidence-assisted pass runs for every Greek deck once a
// provider is configured and its exact payload assessment is recorded; without
// either, the deck is the providerless default (ADR 0105).
func (p *BatchPlanner) planGreekDeck(ctx context.Context, preparation domain.DeckPreparation, deckName string, projections []cardexport.CandidateProjection, mode domain.PreparedDeckExecutionMode, standardAttempts int, providerConfigured bool) (persistence.FreezePreparedDeckRunParams, error) {
	if providerConfigured && p.evidenceAssistedPassAllowed(projections) {
		return p.planEvidenceAssistedRun(ctx, preparation, deckName, projections, standardAttempts)
	}
	return p.planSourceEvidenceRun(ctx, preparation, deckName, projections, mode)
}

// evidenceAssistedPassAllowed reports whether the configured provider, prompt
// contract and payload match a recorded assessment. A drifted or unrecorded
// assessment sends nothing: the deck falls back to the default card.
func (p *BatchPlanner) evidenceAssistedPassAllowed(projections []cardexport.CandidateProjection) bool {
	if p.Codec == nil || len(projections) == 0 || !enrichment.EvidenceAssistedPassOffered(p.assessments()) {
		return false
	}
	return enrichment.AllowExternalProviderCall(p.assessments(), projections[0].Candidate.Language, p.Codec.ProviderName(), p.Codec.EvidenceAssistedPassPromptContract(), enrichment.EvidenceAssistedPassPayloadFields()) == nil
}

// planExternalRun freezes a deck whose contextual Glosses come from the
// configured provider, exactly as before the source-evidence path existed.
func (p *BatchPlanner) planExternalRun(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, projections []cardexport.CandidateProjection, deckName string, mode domain.PreparedDeckExecutionMode, standardAttempts int) (persistence.FreezePreparedDeckRunParams, error) {
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

// deckLanguage resolves the preparation's source language when the assembler
// can report it before its projections are assembled, and the empty string
// otherwise.
func (p *BatchPlanner) deckLanguage(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) (string, error) {
	reader, ok := p.Assembler.(deckLanguageReader)
	if !ok {
		return "", nil
	}
	language, err := reader.DeckLanguage(ctx, tx, preparation)
	if err != nil {
		return "", fmt.Errorf("resolve prepared deck language: %w", err)
	}
	return language, nil
}

// sourceEvidenceDeck reports whether every selected candidate is Ancient Greek,
// whose default deck is built from the Reading snapshot and representative
// sentences alone. Such a deck makes no provider call and uses no meaning
// evidence. An empty deck carries no language and keeps the provider contract.
func sourceEvidenceDeck(projections []cardexport.CandidateProjection) bool {
	if len(projections) == 0 {
		return false
	}
	for _, projection := range projections {
		if !cardexport.IsAncientGreek(projection.Candidate.Language) {
			return false
		}
	}
	return true
}

// planSourceEvidenceRun freezes a providerless deck: no candidate carries a
// provider, cache identity, contextual Gloss or Wiktionary meaning evidence, so
// the durable run has no provider work and advances straight to finalization. An
// Ancient Greek candidate still freezes its matched LSJ dictionary entry.
func (p *BatchPlanner) planSourceEvidenceRun(ctx context.Context, preparation domain.DeckPreparation, deckName string, projections []cardexport.CandidateProjection, mode domain.PreparedDeckExecutionMode) (persistence.FreezePreparedDeckRunParams, error) {
	for i := range projections {
		projections[i].Provider = ""
		projections[i].ProviderVersion = ""
		projections[i].TargetLanguage = ""
		projections[i].RequireContextualGloss = false
		projections[i].SkipMeaningEvidence = true
	}
	deck, _, err := p.Presentation.Freeze(ctx, preparation.OwnerID, deckName, projections)
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("freeze prepared deck presentation: %w", err)
	}
	config := persistence.PreparedDeckRunConfig{ExecutionMode: string(mode), TargetLanguage: "en"}
	return persistence.FreezePreparedDeckRunParams{RunID: uuid.NewString(), Projection: deck.StorageProjection(), Config: config}, nil
}

// planEvidenceAssistedRun freezes a source-evidence Ancient Greek deck that also
// runs the provider pass. The base card is unchanged: the projection still
// skips Wiktionary meaning evidence and freezes its LSJ entry and passage
// context. The pass adds an exact provider cache identity, so the durable run
// makes one bounded standard provider call per accepted item with the
// representative sentence, its passage context and the pinned LSJ senses. The
// pass always uses the standard executor, because its best-effort per-item
// semantics do not fit the Batch omission machinery.
//
// The pass runs only while a matching provider payload assessment is recorded.
// The gate is checked again before the presentation is frozen, so an
// unrecorded payload fails closed and nothing is sent (ADR 0101, #2171).
func (p *BatchPlanner) planEvidenceAssistedRun(ctx context.Context, preparation domain.DeckPreparation, deckName string, projections []cardexport.CandidateProjection, standardAttempts int) (persistence.FreezePreparedDeckRunParams, error) {
	if p == nil || p.Codec == nil || len(projections) == 0 {
		return persistence.FreezePreparedDeckRunParams{}, errTranslationProviderRequired
	}
	provider := p.Codec.ProviderName()
	if !enrichment.EvidenceAssistedPassOffered(p.assessments()) {
		// The pass runs only while its exact payload assessment is recorded:
		// otherwise no request is transmitted.
		return persistence.FreezePreparedDeckRunParams{}, enrichment.ErrProviderAssessmentNotRecorded
	}
	if err := enrichment.AllowExternalProviderCall(p.assessments(), projections[0].Candidate.Language, provider, p.Codec.EvidenceAssistedPassPromptContract(), enrichment.EvidenceAssistedPassPayloadFields()); err != nil {
		return persistence.FreezePreparedDeckRunParams{}, err
	}
	// The context-free LSJ sense summary is a second, separately assessed
	// payload. It is offered only while its own matching assessment is recorded,
	// and it rides along with the pass: when it is not offered the pass deck is
	// unchanged and no summary is requested (ADR 0101).
	summaryVersion := ""
	if enrichment.LSJSenseSummaryOffered(p.assessments()) && enrichment.AllowExternalProviderCall(p.assessments(), projections[0].Candidate.Language, provider, p.Codec.LSJSenseSummaryPromptContract(), enrichment.LSJSenseSummaryPayloadFields()) == nil {
		summaryVersion = p.Codec.LSJSenseSummaryProviderVersion()
	}
	config := persistence.PreparedDeckRunConfig{
		ExternalTranslationConsent:    true,
		ExternalTranslationConfigured: true,
		ExecutionMode:                 string(domain.PreparedDeckExecutionStandard),
		TargetLanguage:                "en",
		ContextMode:                   string(enrichment.SentenceContext),
		Provider:                      provider,
		ProviderVersion:               p.Codec.EvidenceAssistedPassProviderVersion(),
		Endpoint:                      enrichment.OpenAIChatCompletionsEndpoint,
		Model:                         p.Codec.Model(),
	}
	if standardAttempts > 0 {
		config.MaxProviderAttempts = standardAttempts
	}
	for i := range projections {
		projections[i].Provider = config.Provider
		projections[i].ProviderVersion = config.ProviderVersion
		projections[i].TargetLanguage = config.TargetLanguage
		projections[i].RequireContextualGloss = false
		projections[i].SkipMeaningEvidence = true
		projections[i].EvidenceAssistedPass = true
		projections[i].LSJSummaryProviderVersion = summaryVersion
	}
	deck, _, err := p.Presentation.Freeze(ctx, preparation.OwnerID, deckName, projections)
	if err != nil {
		return persistence.FreezePreparedDeckRunParams{}, fmt.Errorf("freeze prepared deck presentation: %w", err)
	}
	return persistence.FreezePreparedDeckRunParams{RunID: uuid.NewString(), Projection: deck.StorageProjection(), Config: config}, nil
}

// assessments resolves the recorded personal-use payload assessments this
// planner gates the pass against.
func (p *BatchPlanner) assessments() []enrichment.ProviderAssessment {
	if p == nil || p.Assessments == nil {
		return enrichment.ProviderAssessments()
	}
	return p.Assessments
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
	AddPreparedDeckWorkerWithAssessments(workers, store, presentation, client, codec, batchConfig, preparedConfig, externalEnabled, enrichment.ProviderAssessments())
}

// AddPreparedDeckWorkerWithAssessments registers the freeze worker with an
// explicit set of recorded personal-use payload assessments. Production uses
// the compiled reviewed set through AddPreparedDeckWorker; a caller installs a
// recorded assessment here to exercise a gated payload.
func AddPreparedDeckWorkerWithAssessments(workers *river.Workers, store *persistence.PostgresStore, presentation *cardexport.Presentation, client riverClient, codec *enrichment.TranslationCodec, batchConfig BatchConfig, preparedConfig PreparedDeckConfig, externalEnabled bool, assessments []enrichment.ProviderAssessment) {
	planner := NewPreparedDeckPlanner(NewInputAssembler(store), presentation, codec, externalEnabled, batchConfig, preparedConfig)
	if setter, ok := planner.(interface {
		SetAssessments([]enrichment.ProviderAssessment)
	}); ok {
		setter.SetAssessments(assessments)
	}
	river.AddWorker(workers, &Worker{Coordinator: NewDurableCoordinator(store, client, planner), Store: store})
}

// SetAssessments installs the recorded personal-use payload assessments the
// pass is gated against.
func (p *BatchPlanner) SetAssessments(assessments []enrichment.ProviderAssessment) {
	if p != nil {
		p.Assessments = assessments
	}
}
