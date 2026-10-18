package prepareddeck

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
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
	limit := w.limit()
	if err := w.repairRecoveryWork(ctx, limit); err != nil {
		return err
	}
	if err := w.enqueueStaleRerenders(ctx, limit); err != nil {
		return err
	}
	return river.JobSnooze(w.interval())
}

// limit bounds the work one recovery pass claims at once.
func (w *RecoveryWorker) limit() int {
	if w.Limit < 1 {
		return 100
	}
	return w.Limit
}

// interval is the snooze between recovery passes.
func (w *RecoveryWorker) interval() time.Duration {
	if w.Interval <= 0 {
		return DefaultBatchPollInterval
	}
	return w.Interval
}

// repairRecoveryWork reconstructs every persisted recovery identity and records
// how many Batch runs have stalled.
func (w *RecoveryWorker) repairRecoveryWork(ctx context.Context, limit int) error {
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
		if err := w.repair(ctx, item); err != nil && !ignorableRecoveryError(err) {
			return err
		}
	}
	return nil
}

// enqueueStaleRerenders queues a presentation rebuild for every stale deck.
func (w *RecoveryWorker) enqueueStaleRerenders(ctx context.Context, limit int) error {
	stale, err := w.Store.ListStalePreparedDecks(ctx, cardexport.PresentationVersion, limit)
	if err != nil {
		return err
	}
	for _, item := range stale {
		if err := w.enqueueRerender(ctx, item); err != nil && !ignorableRecoveryError(err) {
			return err
		}
	}
	return nil
}

// ignorableRecoveryError reports the concurrent-repair outcomes a later pass
// will observe again.
func ignorableRecoveryError(err error) bool {
	return errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition)
}

func (w *RecoveryWorker) enqueueRerender(ctx context.Context, item domain.PreparedDeckRerenderWork) (err error) {
	tx, err := w.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	var currentRunID string
	var artifactRenderInputVersion int
	err = tx.QueryRow(ctx, `SELECT current_run_id::text, render_input_version FROM deck_preparations WHERE owner_id=$1 AND id=$2 AND state='ready' AND retired_at IS NULL AND current_run_id=$3::uuid FOR UPDATE`, item.OwnerID, item.PreparationID, item.RunID).Scan(&currentRunID, &artifactRenderInputVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	// The run keeps the render-input version its inputs were frozen at; a
	// rerendered artifact records the version it was rendered at on the
	// preparation.
	var runPresentationVersion, runRenderInputVersion int
	err = tx.QueryRow(ctx, `SELECT presentation_version, render_input_version FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3::uuid AND state='completed'`, item.OwnerID, item.PreparationID, currentRunID).Scan(&runPresentationVersion, &runRenderInputVersion)
	if errors.Is(err, pgx.ErrNoRows) || (runPresentationVersion >= item.PresentationVersion && max(runRenderInputVersion, artifactRenderInputVersion) >= cardexport.RenderInputVersion) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	args := RerenderJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: currentRunID, PresentationVersion: item.PresentationVersion}
	inserted, err := w.Client.InsertTx(ctx, tx, args, durableInsertOpts())
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		if _, confirmErr := liveRerenderJobID(ctx, tx, item.OwnerID, item.PreparationID, currentRunID, item.PresentationVersion); confirmErr != nil {
			return confirmErr
		}
	}
	return tx.Commit(ctx)
}

// repair dispatches one persisted recovery identity to the reconstruction step
// for its stuck state.
func (w *RecoveryWorker) repair(ctx context.Context, item domain.PreparedDeckRecoveryWork) error {
	switch item.Kind {
	case "outcome":
		return w.repairOutcome(ctx, item)
	case "translation_completion":
		return w.repairTranslationCompletion(ctx, item)
	default:
		return w.repairDurableJob(ctx, item)
	}
}

// repairOutcome reconstructs the short standard-translation job for one outcome,
// redispatching an expired claim to a new generation first. The persisted
// execution mode is the only authority during recovery: Batch outcomes are
// repaired by their durable chunk jobs.
func (w *RecoveryWorker) repairOutcome(ctx context.Context, item domain.PreparedDeckRecoveryWork) (err error) {
	run, err := w.Store.GetPreparedDeckRun(ctx, item.OwnerID, item.PreparationID, item.RunID)
	if err != nil {
		return err
	}
	if run.ExecutionMode != domain.PreparedDeckExecutionStandard {
		return nil
	}
	generation, err := w.outcomeGeneration(ctx, item)
	if err != nil {
		return err
	}
	tx, err := w.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	args := StandardTranslationJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, Ordinal: item.Ordinal, Generation: generation}
	inserted, err := w.Client.InsertTx(ctx, tx, args, durableInsertOptsForQueue(TranslationQueue))
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("prepareddeck: River did not return live standard translation recovery work")
	}
	if err := w.Store.SetPreparedDeckTranslationJobTx(ctx, tx, item.OwnerID, item.PreparationID, item.RunID, item.Ordinal, generation, inserted.Job.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// outcomeGeneration redispatches an expired standard claim, returning the
// generation its replacement job must carry.
func (w *RecoveryWorker) outcomeGeneration(ctx context.Context, item domain.PreparedDeckRecoveryWork) (int, error) {
	if !item.LeaseExpired {
		return item.Generation, nil
	}
	redispatched, err := w.Store.RedispatchPreparedDeckTranslationOutcome(ctx, item.OwnerID, item.PreparationID, item.RunID, item.Ordinal, item.Generation)
	if err != nil {
		return 0, err
	}
	return redispatched.DispatchGeneration, nil
}

// repairTranslationCompletion advances a run whose standard outcomes are all
// terminal, dispatching its finalizer when it needs one.
func (w *RecoveryWorker) repairTranslationCompletion(ctx context.Context, item domain.PreparedDeckRecoveryWork) error {
	_, err := w.Store.AdvancePreparedDeckRunIfTerminal(ctx, item.OwnerID, item.PreparationID, item.RunID, w.insertFinalizer)
	return err
}

// repairDurableJob reconstructs the missing durable job for a chunk or
// finalizer identity, storing the new River job id on the preparation.
func (w *RecoveryWorker) repairDurableJob(ctx context.Context, item domain.PreparedDeckRecoveryWork) (err error) {
	tx, err := w.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	args := durableRecoveryJobArgs(item)
	if args == nil {
		return nil
	}
	inserted, err := w.Client.InsertTx(ctx, tx, args, durableInsertOpts())
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("prepareddeck: River did not return live durable recovery work")
	}
	if err := w.setDurableRecoveryJob(ctx, tx, item, inserted.Job.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// durableRecoveryJobArgs maps a stuck-state kind to the River arguments that
// reconstruct its job, or nil when the kind has no reconstructable job.
func durableRecoveryJobArgs(item domain.PreparedDeckRecoveryWork) river.JobArgs {
	switch item.Kind {
	case "batch_submission":
		return BatchSubmitJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, ChunkID: item.ChunkID, Generation: item.Generation}
	case "batch_reconciliation":
		return BatchPollJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, ChunkID: item.ChunkID, Generation: item.Generation}
	case "finalizer":
		return FinalizeJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, Generation: item.Generation}
	case "batch_cleanup":
		return BatchCleanupJobArgs{OwnerID: item.OwnerID, PreparationID: item.PreparationID, RunID: item.RunID, ChunkID: item.ChunkID, Generation: item.Generation}
	default:
		return nil
	}
}

// setDurableRecoveryJob records the reconstructed River job id on the
// preparation. Cleanup jobs are idempotent and claim their own short lease, so
// there is no provider id to store.
func (w *RecoveryWorker) setDurableRecoveryJob(ctx context.Context, tx pgx.Tx, item domain.PreparedDeckRecoveryWork, jobID int64) error {
	switch item.Kind {
	case "batch_submission":
		return w.Store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, item.OwnerID, item.PreparationID, item.RunID, item.ChunkID, item.Generation, jobID)
	case "batch_reconciliation":
		return w.Store.SetPreparedDeckBatchReconciliationJobTx(ctx, tx, item.OwnerID, item.PreparationID, item.RunID, item.ChunkID, item.Generation, jobID)
	case "finalizer":
		return w.Store.SetPreparedDeckFinalizationJobTx(ctx, tx, item.OwnerID, item.PreparationID, item.RunID, item.Generation, jobID)
	default:
		return nil
	}
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
		return errors.New("prepareddeck: River did not return a live finalizer job")
	}
	return w.Store.SetPreparedDeckFinalizationJobTx(ctx, tx, run.OwnerID, run.PreparationID, run.ID, run.FinalizationDispatchGeneration, inserted.Job.ID)
}

func EnsureRecoveryJob(ctx context.Context, store *persistence.PostgresStore, client riverClient) (err error) {
	if store == nil || client == nil {
		return ErrInvalidInput
	}
	tx, err := store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	inserted, err := client.InsertTx(ctx, tx, RecoveryJobArgs{}, durableInsertOpts())
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("prepareddeck: River did not return a live prepared-deck recovery job")
	}
	return tx.Commit(ctx)
}

func AddRecoveryWorker(workers *river.Workers, store *persistence.PostgresStore, client riverClient, interval time.Duration) {
	AddRecoveryWorkerWithMetrics(workers, store, client, interval, nil)
}

func AddRecoveryWorkerWithMetrics(workers *river.Workers, store *persistence.PostgresStore, client riverClient, interval time.Duration, metrics BatchMetrics) {
	river.AddWorker(workers, &RecoveryWorker{Store: store, Client: client, Interval: interval, Metrics: metrics})
}
