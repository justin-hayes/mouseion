package persistence

import (
	"context"
	"fmt"
	"time"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// GetDeckPreparationStatus returns the public preparation row with a derived
// projection of its current durable run. All reads are owner-scoped and the
// projection contains counts only; provider object IDs remain internal.
func (s *PostgresStore) GetDeckPreparationStatus(ctx context.Context, owner, preparationID string) (domain.DeckPreparation, error) {
	p, err := s.GetDeckPreparation(ctx, owner, preparationID)
	if err != nil {
		return p, err
	}
	if p.CurrentRunID == "" {
		p.Phase = runlessPreparationPhase(p.State)
		return p, nil
	}
	run, err := s.GetPreparedDeckRun(ctx, owner, preparationID, p.CurrentRunID)
	if err != nil {
		return p, err
	}
	progress, err := s.PreparedDeckRunProgress(ctx, owner, preparationID, run.ID)
	if err != nil {
		return p, err
	}
	chunks, err := s.ListPreparedDeckBatchChunks(ctx, owner, preparationID, run.ID)
	if err != nil {
		return p, err
	}
	projection, _, err := s.LoadPreparedDeckStorageProjection(ctx, owner, preparationID, run.ID)
	if err != nil {
		return p, err
	}
	frozen, err := cardexport.NewPresentation(nil).Restore(projection)
	if err != nil {
		return p, err
	}
	if err := s.reportContextualGlosses(ctx, &p, projection); err != nil {
		return p, err
	}
	appendEvidenceCoverage(&p, frozen)
	outcomes, err := s.ListPreparedDeckTranslationOutcomes(ctx, owner, preparationID, run.ID)
	if err != nil {
		return p, err
	}
	appendMeaningOmissions(&p, outcomes, projection.Items)
	applyRunProgress(&p, run, progress, chunks)
	if p.State == domain.DeckPreparationReady {
		p.ProviderOmissions = countProviderOmissions(outcomes)
	}
	if err := s.applyActiveRunCompleteness(ctx, &p, owner, preparationID, run, progress); err != nil {
		return p, err
	}
	return p, nil
}

// runlessPreparationPhase is the phase of a preparation that has not frozen a
// durable run, derived from its state alone.
func runlessPreparationPhase(state domain.DeckPreparationState) string {
	switch state {
	case domain.DeckPreparationQueued:
		return "queued"
	case domain.DeckPreparationPreparing:
		return "freezing"
	case domain.DeckPreparationReady:
		return "completed"
	case domain.DeckPreparationFailed:
		return "failed"
	case domain.DeckPreparationCancelled:
		return "cancelled"
	}
	return ""
}

// reportContextualGlosses counts the contextual glosses of a ready deck whose
// manifest records meaning evidence.
func (s *PostgresStore) reportContextualGlosses(ctx context.Context, p *domain.DeckPreparation, projection cardexport.StorageProjection) error {
	if p.State != domain.DeckPreparationReady || projection.SchemaVersion < cardexport.ManifestSchemaVersionV8 {
		return nil
	}
	records, err := s.loadPreparedDeckStoredRecords(ctx, projection)
	if err != nil {
		return err
	}
	p.ContextualGlossesReported = true
	for _, record := range records {
		if !record.Found || record.CacheKey.MeaningEvidenceHash == "" {
			continue
		}
		p.ContextualGlosses++
		// For contextual-gloss identities, validated non-empty evidence IDs
		// become selection indices in the durable cache. An empty selection
		// therefore preserves the provider's explicit context-only outcome.
		if len(record.Entry.SenseSelection) == 0 {
			p.ContextOnlyGlosses++
		}
	}
	return nil
}

func appendEvidenceCoverage(p *domain.DeckPreparation, frozen cardexport.FrozenDeck) {
	for _, coverage := range frozen.Diagnostics().EvidenceCoverage {
		p.EvidenceCoverage = append(p.EvidenceCoverage, domain.DeckPreparationEvidenceCoverage{Source: coverage.Source, Configured: coverage.Configured, Selected: coverage.Selected, Matched: coverage.Matched, Candidates: coverage.Candidates, Omitted: coverage.Omitted})
	}
}

// appendMeaningOmissions names the target word of every completed item the
// provider explicitly left unresolved. An evidence-assisted pass abstention is
// not an unresolved meaning: the pass only adds, so it is skipped.
func appendMeaningOmissions(p *domain.DeckPreparation, outcomes []domain.PreparedDeckTranslationOutcome, items []cardexport.ManifestItem) {
	for _, outcome := range outcomes {
		if outcome.OmissionReason == "" || outcome.State != domain.PreparedDeckOutcomeCompleted || outcome.OmissionReason == domain.DeckPreparationEvidenceAssistedPassAbstentionReason {
			continue
		}
		for _, item := range items {
			if item.Ordinal == outcome.Ordinal {
				p.MeaningOmissions = append(p.MeaningOmissions, domain.DeckPreparationMeaningOmission{TargetWord: item.Entry.TargetWord, Reason: outcome.OmissionReason})
				break
			}
		}
	}
}

// countProviderOmissions counts the accepted identities a ready deck left out
// because the provider answered unusably. A ready deck that carries a
// non-response failure (possible only outside the standard fail-closed path)
// still reports that failure as a translation failure, not as an omission.
func countProviderOmissions(outcomes []domain.PreparedDeckTranslationOutcome) int {
	count := 0
	for _, outcome := range outcomes {
		if outcome.State == domain.PreparedDeckOutcomeFailed && tolerableProviderOmission(outcome.ErrorClass, outcome.ErrorCode) {
			count++
		}
	}
	return count
}

func applyRunProgress(p *domain.DeckPreparation, run domain.PreparedDeckRun, progress domain.PreparedDeckRunProgress, chunks []domain.PreparedDeckBatchChunk) {
	p.Phase = preparationPhase(p.State, run, progress, chunks)
	p.FailureClass = preparedDeckFailureClass(run, chunks)
	p.TranslationEligible = progress.CandidateCount
	p.TranslationDone = progress.CompletedCount
	p.TranslationPending = progress.PendingCount
	p.TranslationRunning = progress.RunningCount
	p.TranslationFailed = progress.FailedCount
	p.TranslationCancelled = progress.CancelledCount
	p.TranslationRetrying = progress.RetryingCount
	p.BatchChunkCount = progress.BatchChunkCount
	p.BatchSubmittedChunks = progress.BatchSubmittedChunks
	p.BatchPollingChunks = progress.BatchPollingChunks
	p.BatchReconcilingChunks = progress.BatchReconcilingChunks
	p.BatchCompletedChunks = progress.BatchCompletedChunks
	p.BatchFailedChunks = progress.BatchFailedChunks
	p.BatchCancelledChunks = progress.BatchCancelledChunks
	p.BatchRequestCount = progress.BatchRequestCount
	p.BatchCompletedRequests = progress.BatchCompletedRequests
	p.BatchFailedRequests = progress.BatchFailedRequests
	p.BatchExpiredRequests = progress.BatchExpiredRequests
	p.BatchInputTokens = progress.BatchInputTokens
	p.BatchOutputTokens = progress.BatchOutputTokens
	p.BatchAge = progress.BatchAge
}

// applyActiveRunCompleteness replaces the completeness counts of an active
// consented run. A ready artifact already contains its committed completeness.
// While a run is active, count only exact cache rows belonging to the frozen
// manifest. This keeps status correct under duplicate polling and partial
// success.
func (s *PostgresStore) applyActiveRunCompleteness(ctx context.Context, p *domain.DeckPreparation, owner, preparationID string, run domain.PreparedDeckRun, progress domain.PreparedDeckRunProgress) error {
	if p.State == domain.DeckPreparationReady || !run.ExternalTranslationConsent || !run.ExternalTranslationConfigured {
		return nil
	}
	coverage, err := s.queries().GetPreparedDeckTranslationCoverage(ctx, sqlcgen.GetPreparedDeckTranslationCoverageParams{Owner: owner, Preparation: preparationID, Run: run.ID})
	if err != nil {
		return err
	}
	p.CardsWithEnglish, err = checked.IntFromInt64(coverage.CardsWithEnglish)
	if err != nil {
		return fmt.Errorf("invalid cards-with-English count: %w", err)
	}
	p.CardsWithContextualSentenceTranslations, err = checked.IntFromInt64(coverage.CardsWithContextualSentenceTranslations)
	if err != nil {
		return fmt.Errorf("invalid contextual-translation count: %w", err)
	}
	p.TotalCards = progress.CandidateCount
	p.QualityOmissions = progress.ManifestOmissions
	return nil
}

// preparedDeckFailureClass keeps specific, bounded chunk diagnostics visible
// for rows written before run-level reconciliation classes were preserved.
// Only the failed chunk is considered; errors on an earlier completed chunk
// must not replace the current run failure.
func preparedDeckFailureClass(run domain.PreparedDeckRun, chunks []domain.PreparedDeckBatchChunk) string {
	if run.ErrorClass != "" && run.ErrorClass != "reconciliation" {
		return run.ErrorClass
	}
	for _, chunk := range chunks {
		if (chunk.State == domain.PreparedDeckBatchFailed || chunk.State == domain.PreparedDeckBatchAmbiguous) && chunk.ErrorClass != "" {
			return chunk.ErrorClass
		}
	}
	return run.ErrorClass
}

func preparationPhase(state domain.DeckPreparationState, run domain.PreparedDeckRun, progress domain.PreparedDeckRunProgress, chunks []domain.PreparedDeckBatchChunk) string {
	switch state {
	case domain.DeckPreparationQueued:
		return "queued"
	case domain.DeckPreparationReady:
		return "completed"
	case domain.DeckPreparationFailed:
		return "failed"
	case domain.DeckPreparationCancelled:
		return "cancelled"
	case domain.DeckPreparationPreparing:
		// The durable run and chunk state below determine the detailed phase.
	}
	if run.State == domain.PreparedDeckRunFinalizing {
		if run.ExecutionMode == domain.PreparedDeckExecutionStandard {
			return "assembling"
		}
		return "finalizing"
	}
	if run.State == domain.PreparedDeckRunFailed {
		return "failed"
	}
	if run.State == domain.PreparedDeckRunCancelled {
		return "cancelled"
	}
	if progress.RetryingCount > 0 {
		return "retrying"
	}
	for _, chunk := range chunks {
		switch chunk.State {
		case domain.PreparedDeckBatchReconciling:
			return "reconciling"
		case domain.PreparedDeckBatchSubmitted, domain.PreparedDeckBatchPolling:
			return "waiting"
		case domain.PreparedDeckBatchSubmitting, domain.PreparedDeckBatchPending:
			return "submitting"
		case domain.PreparedDeckBatchCompleted, domain.PreparedDeckBatchFailed, domain.PreparedDeckBatchCancelled, domain.PreparedDeckBatchAmbiguous:
			// Terminal chunks do not determine an active phase.
		}
	}
	return "translating"
}

// PreparedDeckStuckBatch is an aggregate operational projection. It is not
// exposed to learners and intentionally excludes provider object IDs and
// source-derived fields.
type PreparedDeckStuckBatch struct {
	OwnerID, PreparationID, RunID, ChunkID                  string
	State, ProviderStatus, ErrorClass                       string
	RequestCount, CompletedCount, FailedCount, ExpiredCount int
	Age                                                     time.Duration
}

// ListPreparedDeckStuckBatches finds old nonterminal chunks. Recovery remains
// bounded by limit and uses the same generation-fenced jobs as normal repair.
func (s *PostgresStore) ListPreparedDeckStuckBatches(ctx context.Context, olderThan time.Duration, limit int) ([]PreparedDeckStuckBatch, error) {
	if olderThan <= 0 || limit < 1 {
		return nil, ErrInvalidTransition
	}
	sqlLimit, err := checked.Int32FromInt(limit)
	if err != nil {
		return nil, fmt.Errorf("invalid stuck-batch limit: %w", err)
	}
	rows, err := s.queries().ListPreparedDeckStuckBatches(ctx, sqlcgen.ListPreparedDeckStuckBatchesParams{OlderSeconds: olderThan.Seconds(), Limit: sqlLimit})
	if err != nil {
		return nil, err
	}
	result := make([]PreparedDeckStuckBatch, 0, len(rows))
	for _, row := range rows {
		result = append(result, PreparedDeckStuckBatch{
			OwnerID: row.OwnerID, PreparationID: row.PreparationID, RunID: row.RunID, ChunkID: row.ID,
			State: row.State, ProviderStatus: row.ProviderStatus, ErrorClass: row.ErrorClass,
			RequestCount: row.RequestCount, CompletedCount: row.CompletedCount,
			FailedCount: row.FailedCount, ExpiredCount: row.ExpiredCount,
			Age: time.Duration(row.AgeSeconds * float64(time.Second)),
		})
	}
	return result, nil
}
