package analysis

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// ConstructionRebuildArgs names one analysis run whose Construction occurrences
// are rebuilt from its stored syntax.
type ConstructionRebuildArgs struct {
	OwnerID string `json:"owner_id" river:"unique"`
	RunID   string `json:"run_id" river:"unique"`
	// DetectorVersion is the version the rebuild targets. A new version is a new
	// unique key, so a bump queues work even when an older job was retained.
	DetectorVersion int `json:"detector_version" river:"unique"`
}

func (ConstructionRebuildArgs) Kind() string { return "rebuild_construction_occurrences" }

// InsertOpts makes a queued rebuild of one owner's run unique, so repeated
// startup scans never queue it twice while it is waiting or running.
func (ConstructionRebuildArgs) InsertOpts() river.InsertOpts {
	// Finished jobs are not part of the key: only waiting or running work dedupes.
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
		rivertype.JobStateRetryable, rivertype.JobStateScheduled,
	}}}
}

// ConstructionRebuildWorker rebuilds one run's Construction occurrences after a
// detector-version bump. It changes nothing but construction_occurrences and
// construction_run_markers: the frozen parse, Browse counts, decks and decisions
// are never read for writing.
type ConstructionRebuildWorker struct {
	river.WorkerDefaults[ConstructionRebuildArgs]
	Pool *pgxpool.Pool
}

func (w *ConstructionRebuildWorker) Work(ctx context.Context, job *river.Job[ConstructionRebuildArgs]) (err error) {
	a := job.Args
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Construction rebuild: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	// The row lock serializes concurrent rebuilds of one run.
	var version *int
	err = tx.QueryRow(ctx, `SELECT m.detector_version
		FROM analysis_runs r
		LEFT JOIN construction_run_markers m ON m.analysis_run_id=r.id
		WHERE r.owner_id=$1 AND r.id=$2 AND r.state='completed' AND r.syntax_state='native_agdt'
		FOR UPDATE OF r`, a.OwnerID, a.RunID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // the run is gone, unfinished, or stores no AGDT syntax
	}
	if err != nil {
		return fmt.Errorf("load run for Construction rebuild: %w", err)
	}
	if version != nil && *version >= constructions.AGDTDetectorVersion {
		return nil // already current: a repeat changes nothing
	}
	if err = persistence.BuildConstructionOccurrencesTx(ctx, tx, a.OwnerID, a.RunID); err != nil {
		return fmt.Errorf("rebuild Construction occurrences: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("publish rebuilt Construction occurrences: %w", err)
	}
	return nil
}

// EnqueueMissingConstructionRebuilds places one durable job per completed
// native_agdt run, of any owner and whether current, pinned or superseded,
// whose marker is missing or older than the shipped detector version. The
// uniqueness fields make startup reconciliation safe to repeat.
func (s *Service) EnqueueMissingConstructionRebuilds(ctx context.Context) (err error) {
	if s == nil || s.pool == nil || s.client == nil {
		return errors.New("analysis: Construction rebuild service is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Construction rebuild scheduling: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	rows, err := tx.Query(ctx, `SELECT r.owner_id::text, r.id::text
		FROM analysis_runs r
		LEFT JOIN construction_run_markers m ON m.analysis_run_id=r.id
		WHERE r.state='completed' AND r.syntax_state='native_agdt'
		  AND (m.analysis_run_id IS NULL OR m.detector_version < $1)
		ORDER BY r.owner_id, r.id`, constructions.AGDTDetectorVersion)
	if err != nil {
		return fmt.Errorf("find runs missing current Construction occurrences: %w", err)
	}
	var args []ConstructionRebuildArgs
	for rows.Next() {
		a := ConstructionRebuildArgs{DetectorVersion: constructions.AGDTDetectorVersion}
		if err = rows.Scan(&a.OwnerID, &a.RunID); err != nil {
			rows.Close()
			return fmt.Errorf("read runs missing Construction occurrences: %w", err)
		}
		args = append(args, a)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate runs missing Construction occurrences: %w", err)
	}
	rows.Close()
	for _, a := range args {
		if _, err = s.client.InsertTx(ctx, tx, a, &river.InsertOpts{Queue: ClassicalPreparationQueue, MaxAttempts: 10}); err != nil {
			return fmt.Errorf("enqueue Construction rebuild for run %s: %w", a.RunID, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Construction rebuild scheduling: %w", err)
	}
	return nil
}
