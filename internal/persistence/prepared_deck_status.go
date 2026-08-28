package persistence

import (
	"context"
	"time"

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
	p.FailureClass = run.ErrorClass
	if p.FailureClass == "" {
		for _, chunk := range chunks {
			if chunk.ErrorClass != "" {
				p.FailureClass = chunk.ErrorClass
				break
			}
		}
	}
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
		if err = s.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE COALESCE(ec.translation,'') <> ''), count(*) FILTER (WHERE COALESCE(ec.sentence_translation,'') <> '')
			FROM deck_preparation_manifest_items mi
			JOIN deck_preparation_translation_outcomes o ON o.owner_id=mi.owner_id AND o.preparation_id=mi.preparation_id AND o.run_id=mi.run_id AND o.ordinal=mi.ordinal AND o.state='completed'
			LEFT JOIN enrichment_cache ec ON ec.language=mi.language AND ec.canonical_lemma=mi.canonical_lemma AND ec.upos=mi.upos AND ec.provider=mi.provider AND ec.provider_version=mi.provider_version AND ec.sentence_hash=COALESCE(mi.sentence_hash,'')
			WHERE mi.owner_id=$1 AND mi.preparation_id=$2 AND mi.run_id=$3 AND mi.disposition='accepted'`, owner, preparationID, run.ID).Scan(&p.CardsWithEnglish, &p.CardsWithContextualSentenceTranslations); err != nil {
			return p, err
		}
		p.TotalCards = progress.CandidateCount
		p.QualityOmissions = progress.ManifestOmissions
	}
	return p, nil
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
	}
	if run.State == domain.PreparedDeckRunFinalizing {
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
	rows, err := s.pool.Query(ctx, `SELECT owner_id::text,preparation_id::text,run_id::text,id::text,state,COALESCE(provider_status,''),error_class,request_count,completed_count,failed_count,expired_count,EXTRACT(EPOCH FROM (now()-updated_at))
		FROM deck_preparation_batch_chunks
		WHERE state IN ('pending','submitting','submitted','polling','reconciling') AND updated_at <= now()-($1 * interval '1 second')
		ORDER BY updated_at ASC LIMIT $2`, olderThan.Seconds(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PreparedDeckStuckBatch
	for rows.Next() {
		var item PreparedDeckStuckBatch
		var ageSeconds float64
		if err = rows.Scan(&item.OwnerID, &item.PreparationID, &item.RunID, &item.ChunkID, &item.State, &item.ProviderStatus, &item.ErrorClass, &item.RequestCount, &item.CompletedCount, &item.FailedCount, &item.ExpiredCount, &ageSeconds); err != nil {
			return nil, err
		}
		item.Age = time.Duration(ageSeconds * float64(time.Second))
		result = append(result, item)
	}
	return result, rows.Err()
}
