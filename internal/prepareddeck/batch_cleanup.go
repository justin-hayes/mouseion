package prepareddeck

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
)

const batchCleanupLease = 5 * time.Minute

type BatchCleanupJobArgs struct {
	OwnerID       string `json:"owner_id"`
	PreparationID string `json:"preparation_id"`
	RunID         string `json:"run_id"`
	ChunkID       string `json:"chunk_id" river:"unique"`
	Generation    int    `json:"generation"`
}

func (BatchCleanupJobArgs) Kind() string { return "prepared_deck_batch_cleanup" }

type batchFileDeleter interface {
	DeleteFile(context.Context, string) (enrichment.OpenAIFileDeletion, error)
}

// BatchCleanupWorker removes temporary provider files after successful
// reconciliation or cancellation. Each file has its own durable state, so a
// retry never makes a successful deletion depend on another file.
type BatchCleanupWorker struct {
	river.WorkerDefaults[BatchCleanupJobArgs]
	Store    *persistence.PostgresStore
	Provider batchFileDeleter
	Now      func() time.Time
	Metrics  BatchMetrics
}

func (w *BatchCleanupWorker) Work(ctx context.Context, job *river.Job[BatchCleanupJobArgs]) error {
	if job == nil {
		return ErrInvalidInput
	}
	return w.Cleanup(ctx, job.Args)
}

func (w *BatchCleanupWorker) Cleanup(ctx context.Context, args BatchCleanupJobArgs) error {
	if w == nil || w.Store == nil || w.Provider == nil || strings.TrimSpace(args.OwnerID) == "" || strings.TrimSpace(args.PreparationID) == "" || strings.TrimSpace(args.RunID) == "" || strings.TrimSpace(args.ChunkID) == "" {
		return ErrInvalidInput
	}
	chunk, err := w.Store.GetPreparedDeckBatchChunk(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID)
	if err != nil {
		return err
	}
	if chunk.State != domain.PreparedDeckBatchCompleted && chunk.State != domain.PreparedDeckBatchCancelled {
		return nil
	}
	token := uuid.NewString()
	claimed, err := w.Store.ClaimPreparedDeckBatchCleanup(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, token, w.now().Add(batchCleanupLease))
	if err != nil {
		if errors.Is(err, persistence.ErrInvalidTransition) || errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
			return nil
		}
		return err
	}
	update := persistence.PreparedDeckBatchCleanupUpdate{
		InputFileState: claimed.InputFileCleanupState, OutputFileState: claimed.OutputFileCleanupState, ErrorFileState: claimed.ErrorFileCleanupState,
		InputFileAttempts: claimed.InputFileCleanupAttempts, OutputFileAttempts: claimed.OutputFileCleanupAttempts, ErrorFileAttempts: claimed.ErrorFileCleanupAttempts,
	}
	cleanupErrorClass, cleanupErrorCode := "", ""
	updateFile := func(id, state string, attempts *int, next *string) {
		if id == "" {
			*next = "not_needed"
			return
		}
		if state == "deleted" || state == "not_needed" || *attempts >= 3 {
			*next = state
			return
		}
		*attempts++
		_, deleteErr := w.Provider.DeleteFile(ctx, id)
		if deleteErr == nil || providerFileGone(deleteErr) {
			*next = "deleted"
			observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchFileCleanup, Phase: "cleanup", State: "deleted", Provider: "openai", Value: 1})
			return
		}
		*next = "failed"
		cleanupErrorClass = "provider"
		cleanupErrorCode = "delete_file"
		observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchFileCleanup, Phase: "cleanup", State: "failed", ErrorClass: cleanupErrorClass, Provider: "openai", Value: 1})
	}
	updateFile(claimed.InputFileID, claimed.InputFileCleanupState, &update.InputFileAttempts, &update.InputFileState)
	updateFile(claimed.OutputFileID, claimed.OutputFileCleanupState, &update.OutputFileAttempts, &update.OutputFileState)
	updateFile(claimed.ErrorFileID, claimed.ErrorFileCleanupState, &update.ErrorFileAttempts, &update.ErrorFileState)
	update.ErrorClass, update.ErrorCode = cleanupErrorClass, cleanupErrorCode
	_, err = w.Store.FinishPreparedDeckBatchCleanup(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, token, update)
	if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	return err
}

func providerFileGone(err error) bool {
	var providerErr *enrichment.ProviderError
	return errors.As(err, &providerErr) && providerErr.StatusCode == 404
}

func (w *BatchCleanupWorker) now() time.Time {
	if w != nil && w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func AddBatchCleanupWorker(workers *river.Workers, worker *BatchCleanupWorker) {
	river.AddWorker(workers, worker)
}
