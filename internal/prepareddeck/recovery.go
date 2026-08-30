package prepareddeck

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
)

type RecoveryJobArgs struct{}

func (RecoveryJobArgs) Kind() string { return "prepared_deck_reconcile" }

type RecoveryWorker struct {
	river.WorkerDefaults[RecoveryJobArgs]
	Store      *persistence.PostgresStore
	Client     riverClient
	Interval   time.Duration
	Limit      int
	StuckAfter time.Duration
	Metrics    BatchMetrics
}

// Work reconstructs missing short jobs only from persisted identities. River
// uniqueness makes an already-live delivery an idempotent repair.
func (w *RecoveryWorker) Work(ctx context.Context, _ *river.Job[RecoveryJobArgs]) error {
	if w == nil || w.Store == nil || w.Client == nil {
		return ErrInvalidInput
	}
	limit := w.Limit
	if limit < 1 {
		limit = 100
	}
	work, err := w.Store.ListPreparedDeckRecoveryWork(ctx, limit)
	if err != nil {
		return err
	}
	stuckAfter := w.StuckAfter
	if stuckAfter <= 0 {
		stuckAfter = 24 * time.Hour
	}
	stuck, err := w.Store.ListPreparedDeckStuckBatches(ctx, stuckAfter, limit)
	if err != nil {
		return err
	}
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "batch", Name: MetricBatchStuckBatches, Phase: "waiting", State: "stuck", Provider: "openai", Value: float64(len(stuck))})
	for _, item := range work {
		if err = w.repair(ctx, item); err != nil && !errors.Is(err, persistence.ErrPreparedDeckClaimLost) && !errors.Is(err, persistence.ErrInvalidTransition) {
			return err
		}
	}
	interval := w.Interval
	if interval <= 0 {
		interval = DefaultBatchPollInterval
	}
	return river.JobSnooze(interval)
}

func (w *RecoveryWorker) repair(ctx context.Context, item domain.PreparedDeckRecoveryWork) error {
	if item.Kind == "outcome" {
		run, err := w.Store.GetPreparedDeckRun(ctx, item.OwnerID, item.PreparationID, item.RunID)
		if err != nil {
			return err
		}
		// The persisted execution mode is the only authority during recovery.
		// Batch outcomes are repaired by their durable chunk jobs.
		if run.ExecutionMode != domain.PreparedDeckExecutionStandard {
			return nil
		}
		tx, err := w.Store.Pool().Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		args := StandardTranslationJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, Ordinal: item.Ordinal, Generation: item.Generation}
		inserted, err := w.Client.InsertTx(ctx, tx, args, durableInsertOptsForQueue(TranslationQueue))
		if err != nil {
			return err
		}
		if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
			return errors.New("River did not return live standard translation recovery work")
		}
		if err = w.Store.SetPreparedDeckTranslationJobTx(ctx, tx, item.OwnerID, item.PreparationID, item.RunID, item.Ordinal, item.Generation, inserted.Job.ID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if item.Kind == "translation_completion" {
		_, err := w.Store.AdvancePreparedDeckRunIfTerminal(ctx, item.OwnerID, item.PreparationID, item.RunID, w.insertFinalizer)
		return err
	}
	tx, err := w.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var args river.JobArgs
	switch item.Kind {
	case "batch_submission":
		args = BatchSubmitJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, ChunkID: item.ChunkID, Generation: item.Generation}
	case "batch_reconciliation":
		args = BatchPollJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, ChunkID: item.ChunkID, Generation: item.Generation}
	case "finalizer":
		args = FinalizeJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, Generation: item.Generation}
	case "batch_cleanup":
		args = BatchCleanupJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, ChunkID: item.ChunkID, Generation: item.Generation}
	default:
		return nil
	}
	inserted, err := w.Client.InsertTx(ctx, tx, args, durableInsertOpts())
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("River did not return live durable recovery work")
	}
	switch item.Kind {
	case "batch_submission":
		err = w.Store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, item.OwnerID, item.PreparationID, item.RunID, item.ChunkID, item.Generation, inserted.Job.ID)
	case "batch_reconciliation":
		err = w.Store.SetPreparedDeckBatchReconciliationJobTx(ctx, tx, item.OwnerID, item.PreparationID, item.RunID, item.ChunkID, item.Generation, inserted.Job.ID)
	case "finalizer":
		err = w.Store.SetPreparedDeckFinalizationJobTx(ctx, tx, item.OwnerID, item.PreparationID, item.RunID, item.Generation, inserted.Job.ID)
	case "batch_cleanup":
		// Cleanup jobs are idempotent and claim their own short lease. There is
		// no provider ID in the River args or recovery projection.
		err = nil
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func durableInsertOptsForQueue(queue string) *river.InsertOpts {
	opts := durableInsertOpts()
	opts.Queue = queue
	return opts
}

func (w *RecoveryWorker) insertFinalizer(ctx context.Context, tx pgx.Tx, run domain.PreparedDeckRun) error {
	inserted, err := w.Client.InsertTx(ctx, tx, FinalizeJobArgs{OwnerID: run.OwnerID, PreparationID: run.PreparationID, RunID: run.ID, Generation: run.FinalizationDispatchGeneration}, durableInsertOpts())
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("River did not return a live finalizer job")
	}
	return w.Store.SetPreparedDeckFinalizationJobTx(ctx, tx, run.OwnerID, run.PreparationID, run.ID, run.FinalizationDispatchGeneration, inserted.Job.ID)
}

func EnsureRecoveryJob(ctx context.Context, store *persistence.PostgresStore, client riverClient) error {
	if store == nil || client == nil {
		return ErrInvalidInput
	}
	tx, err := store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	inserted, err := client.InsertTx(ctx, tx, RecoveryJobArgs{}, durableInsertOpts())
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("River did not return a live prepared-deck recovery job")
	}
	return tx.Commit(ctx)
}

func AddRecoveryWorker(workers *river.Workers, store *persistence.PostgresStore, client riverClient, interval time.Duration) {
	AddRecoveryWorkerWithMetrics(workers, store, client, interval, nil)
}

func AddRecoveryWorkerWithMetrics(workers *river.Workers, store *persistence.PostgresStore, client riverClient, interval time.Duration, metrics BatchMetrics) {
	river.AddWorker(workers, &RecoveryWorker{Store: store, Client: client, Interval: interval, Metrics: metrics})
}
