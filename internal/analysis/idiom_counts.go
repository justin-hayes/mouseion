package analysis

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// IdiomCountsRebuildArgs queues the derivation of one Book's Idiom candidate
// counts (ADR 0137). The counts are their own durable job, not part of the
// publish transaction: matching reads the dictionary index, which is outside
// the database, and the derivation scales with the Book's sentences. Like the
// Browse rebuild it builds from the current decisions; DecisionRevision only
// keys uniqueness.
type IdiomCountsRebuildArgs struct {
	OwnerID          string `json:"owner_id" river:"unique"`
	BookID           string `json:"book_id" river:"unique"`
	RunID            string `json:"run_id" river:"unique"`
	DecisionRevision string `json:"decision_revision" river:"unique"`
}

func (IdiomCountsRebuildArgs) Kind() string { return "rebuild_idiom_candidate_counts" }

// InsertOpts makes a queued rebuild unique per Book, run and decision revision
// while it waits or runs.
func (IdiomCountsRebuildArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: Queue, MaxAttempts: 10, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveJobStates}}
}

// IdiomCountsRebuildWorker derives a Book's Idiom candidate counts against the
// dictionary index. IndexVersion is the version the counts are stamped with.
type IdiomCountsRebuildWorker struct {
	river.WorkerDefaults[IdiomCountsRebuildArgs]
	Pool         *pgxpool.Pool
	Index        idiom.Index
	IndexVersion string
}

// AddIdiomCountsWorker registers the Idiom count rebuild worker.
func AddIdiomCountsWorker(workers *river.Workers, pool *pgxpool.Pool, index idiom.Index, indexVersion string) *IdiomCountsRebuildWorker {
	worker := &IdiomCountsRebuildWorker{Pool: pool, Index: index, IndexVersion: indexVersion}
	river.AddWorker(workers, worker)
	return worker
}

func (w *IdiomCountsRebuildWorker) Work(ctx context.Context, job *river.Job[IdiomCountsRebuildArgs]) (err error) {
	a := job.Args
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Idiom count rebuild: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	built, err := persistence.BuildIdiomCandidateCountsTx(ctx, tx, a.OwnerID, a.BookID, w.Index, w.IndexVersion)
	if err != nil {
		return err
	}
	if !built {
		return nil // the Book has no current analysis any more
	}
	var stillCurrent bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM idiom_candidate_count_readiness r
		JOIN current_analysis_identity cai ON cai.owner_id=r.owner_id AND cai.book_id=r.book_id
		 AND cai.analysis_run_id=r.analysis_run_id AND cai.corpus_id=r.corpus_id
		WHERE r.owner_id=$1 AND r.book_id=$2)`, a.OwnerID, a.BookID).Scan(&stillCurrent); err != nil {
		return fmt.Errorf("verify Idiom count identity: %w", err)
	}
	if !stillCurrent {
		return nil // roll back the candidate; reconciliation queues the current analysis
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("publish Idiom counts: %w", err)
	}
	return nil
}

// idiomCountsInserter is the insert-only River client that queues a rebuild
// from inside another transaction. It has no workers and never connects.
var idiomCountsInserter = sync.OnceValues(func() (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(nil), &river.Config{})
})

// EnqueueIdiomCountsTx queues the Idiom count rebuild for a Book's analysis run
// in the caller's transaction, so it is durable exactly when the analysis is
// published. An identical waiting or running rebuild absorbs the insert.
func EnqueueIdiomCountsTx(ctx context.Context, tx pgx.Tx, owner, book, run string) error {
	revision, err := persistence.IdiomCountDecisionRevision(ctx, tx, owner, book, run)
	if err != nil {
		return fmt.Errorf("read decision revision for Idiom counts of Book %s: %w", book, err)
	}
	client, err := idiomCountsInserter()
	if err != nil {
		return fmt.Errorf("create Idiom count inserter: %w", err)
	}
	if _, err := client.InsertTx(ctx, tx, IdiomCountsRebuildArgs{OwnerID: owner, BookID: book, RunID: run, DecisionRevision: revision}, nil); err != nil {
		return fmt.Errorf("enqueue Idiom count rebuild for Book %s: %w", book, err)
	}
	return nil
}

// EnqueueMissingIdiomCountRebuilds queues one job per current Book analysis
// whose Idiom counts are not ready for this builder and idiom index version:
// after an upgrade, a dictionary or reviewed idiom list change, or a decision
// that withdrew readiness. Repeating it is safe.
func (s *Service) EnqueueMissingIdiomCountRebuilds(ctx context.Context, indexVersion string) (err error) {
	if s == nil || s.pool == nil || s.client == nil {
		return errors.New("analysis: Idiom count rebuild service is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Idiom count scheduling: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	rows, err := tx.Query(ctx, `SELECT cai.owner_id::text,cai.book_id::text,cai.analysis_run_id::text
		FROM current_analysis_identity cai
		JOIN source_materials source ON source.owner_id=cai.owner_id AND source.id=cai.source_material_id
		LEFT JOIN idiom_candidate_count_readiness ready ON ready.owner_id=cai.owner_id AND ready.book_id=cai.book_id
		 AND ready.analysis_run_id=cai.analysis_run_id AND ready.corpus_id=cai.corpus_id
		 AND ready.language=source.language AND ready.builder_version=$1 AND ready.index_version=$2
		WHERE ready.owner_id IS NULL ORDER BY cai.owner_id,cai.book_id`, persistence.IdiomCountBuilderVersion, indexVersion)
	if err != nil {
		return fmt.Errorf("find Books missing ready Idiom counts: %w", err)
	}
	type target struct{ owner, book, run string }
	var targets []target
	for rows.Next() {
		var t target
		if err = rows.Scan(&t.owner, &t.book, &t.run); err != nil {
			rows.Close()
			return fmt.Errorf("read Books missing Idiom counts: %w", err)
		}
		targets = append(targets, t)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate Books missing Idiom counts: %w", err)
	}
	rows.Close()
	for _, t := range targets {
		if err := EnqueueIdiomCountsTx(ctx, tx, t.owner, t.book, t.run); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Idiom count scheduling: %w", err)
	}
	return nil
}
