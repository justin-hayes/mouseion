package prepareddeck

import (
	"context"
	"errors"
	"log"
	"net/http"
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
	if !w.validCleanupInput(args) {
		return ErrInvalidInput
	}
	claimed, token, proceed, err := w.claimCleanup(ctx, args)
	if err != nil || !proceed {
		return err
	}
	update := w.cleanupFiles(ctx, args, claimed)
	_, err = w.Store.FinishPreparedDeckBatchCleanup(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, token, update)
	if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	return err
}

func (w *BatchCleanupWorker) validCleanupInput(args BatchCleanupJobArgs) bool {
	return w != nil && w.Store != nil && w.Provider != nil && strings.TrimSpace(args.OwnerID) != "" && strings.TrimSpace(args.PreparationID) != "" && strings.TrimSpace(args.RunID) != "" && strings.TrimSpace(args.ChunkID) != ""
}

// claimCleanup loads the settled chunk and leases its cleanup projection. It
// reports proceed=false when the chunk is not settled or another owner holds
// the claim, so cleanup never runs on an unsettled or already-claimed chunk.
func (w *BatchCleanupWorker) claimCleanup(ctx context.Context, args BatchCleanupJobArgs) (domain.PreparedDeckBatchChunk, string, bool, error) {
	chunk, err := w.Store.GetPreparedDeckBatchChunk(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID)
	if err != nil {
		return domain.PreparedDeckBatchChunk{}, "", false, err
	}
	if chunk.State != domain.PreparedDeckBatchCompleted && chunk.State != domain.PreparedDeckBatchCancelled {
		return domain.PreparedDeckBatchChunk{}, "", false, nil
	}
	token := uuid.NewString()
	claimed, err := w.Store.ClaimPreparedDeckBatchCleanup(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, token, w.now().Add(batchCleanupLease))
	if err != nil {
		if errors.Is(err, persistence.ErrInvalidTransition) || errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
			return domain.PreparedDeckBatchChunk{}, "", false, nil
		}
		return domain.PreparedDeckBatchChunk{}, "", false, err
	}
	return claimed, token, true, nil
}

// cleanupFileOutcome is the durable result of one provider file deletion.
type cleanupFileOutcome struct {
	state    string
	attempts int
	failed   bool
}

// cleanupFiles deletes each provider file independently and returns the
// cleanup projection to persist. Each file has its own durable state, so a
// retry never makes a successful deletion depend on another file.
func (w *BatchCleanupWorker) cleanupFiles(ctx context.Context, args BatchCleanupJobArgs, claimed domain.PreparedDeckBatchChunk) persistence.PreparedDeckBatchCleanupUpdate {
	input := w.cleanupFile(ctx, args, "input", claimed.InputFileID, claimed.InputFileCleanupState, claimed.InputFileCleanupAttempts)
	output := w.cleanupFile(ctx, args, "output", claimed.OutputFileID, claimed.OutputFileCleanupState, claimed.OutputFileCleanupAttempts)
	errorFile := w.cleanupFile(ctx, args, "error", claimed.ErrorFileID, claimed.ErrorFileCleanupState, claimed.ErrorFileCleanupAttempts)
	update := persistence.PreparedDeckBatchCleanupUpdate{
		InputFileState: input.state, OutputFileState: output.state, ErrorFileState: errorFile.state,
		InputFileAttempts: input.attempts, OutputFileAttempts: output.attempts, ErrorFileAttempts: errorFile.attempts,
	}
	if input.failed || output.failed || errorFile.failed {
		update.ErrorClass, update.ErrorCode = "provider", "delete_file"
	}
	return update
}

// cleanupFile deletes one provider file at most once per attempt. A file that
// is absent, already settled, or out of attempts keeps its prior state; a
// successful or already-gone deletion becomes "deleted". Cleanup is best
// effort, so a provider failure is recorded for durable recovery to retry.
func (w *BatchCleanupWorker) cleanupFile(ctx context.Context, args BatchCleanupJobArgs, role, fileID, state string, attempts int) cleanupFileOutcome {
	if fileID == "" {
		return cleanupFileOutcome{state: "not_needed", attempts: attempts}
	}
	if state == "deleted" || state == "not_needed" || attempts >= 3 {
		return cleanupFileOutcome{state: state, attempts: attempts}
	}
	attempts++
	_, deleteErr := w.Provider.DeleteFile(ctx, fileID)
	if deleteErr == nil || providerFileGone(deleteErr) {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "batch", Name: MetricBatchFileCleanup, Phase: "cleanup", State: "deleted", Provider: "openai", Value: 1})
		return cleanupFileOutcome{state: "deleted", attempts: attempts}
	}
	log.Printf("prepared deck batch cleanup warning: provider file deletion failed; durable recovery will retry while attempts remain: owner_id=%s preparation_id=%s run_id=%s chunk_id=%s generation=%d file_role=%s: %v", args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, role, deleteErr)
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "batch", Name: MetricBatchFileCleanup, Phase: "cleanup", State: "failed", ErrorClass: "provider", Provider: "openai", Value: 1})
	return cleanupFileOutcome{state: "failed", attempts: attempts, failed: true}
}

// logImmediateBatchCleanupFailure reports an immediate cleanup attempt that
// returned an error. The cleanup claim is recovered durably, so the chunk is
// retried by recovery rather than by the caller.
func logImmediateBatchCleanupFailure(args BatchCleanupJobArgs, err error) {
	log.Printf("prepared deck batch cleanup warning: immediate cleanup failed; durable recovery will retry: owner_id=%s preparation_id=%s run_id=%s chunk_id=%s generation=%d: %v", args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, err)
}

func providerFileGone(err error) bool {
	var providerErr *enrichment.ProviderError
	return errors.As(err, &providerErr) && providerErr.StatusCode == http.StatusNotFound
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
