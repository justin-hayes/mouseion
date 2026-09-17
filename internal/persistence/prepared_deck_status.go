package persistence

import (
	"context"
	"fmt"
	"time"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
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
		switch p.State {
		case domain.DeckPreparationQueued:
			p.Phase = "queued"
		case domain.DeckPreparationPreparing:
			p.Phase = "freezing"
		case domain.DeckPreparationReady:
			p.Phase = "completed"
		case domain.DeckPreparationFailed:
			p.Phase = "failed"
		case domain.DeckPreparationCancelled:
			p.Phase = "cancelled"
		}
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

	// A ready artifact already contains its committed completeness. While a run
	// is active, count only exact cache rows belonging to the frozen manifest.
	// This keeps status correct under duplicate polling and partial success.
	if p.State != domain.DeckPreparationReady && run.ExternalTranslationConsent && run.ExternalTranslationConfigured {
		coverage, coverageErr := s.queries().GetPreparedDeckTranslationCoverage(ctx, sqlcgen.GetPreparedDeckTranslationCoverageParams{Owner: owner, Preparation: preparationID, Run: run.ID})
		if coverageErr != nil {
			return p, coverageErr
		}
		p.CardsWithEnglish, coverageErr = checked.IntFromInt64(coverage.CardsWithEnglish)
		if coverageErr != nil {
			return p, fmt.Errorf("invalid cards-with-English count: %w", coverageErr)
		}
		p.CardsWithContextualSentenceTranslations, coverageErr = checked.IntFromInt64(coverage.CardsWithContextualSentenceTranslations)
		if coverageErr != nil {
			return p, fmt.Errorf("invalid contextual-translation count: %w", coverageErr)
		}
		p.TotalCards = progress.CandidateCount
		p.QualityOmissions = progress.ManifestOmissions
	}
	return p, nil
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
