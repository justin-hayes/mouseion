package analysis

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
)

type BrowseCountsRebuildArgs struct {
	OwnerID string `json:"owner_id" river:"unique"`
	BookID  string `json:"book_id" river:"unique"`
	RunID   string `json:"run_id" river:"unique"`
}

func (BrowseCountsRebuildArgs) Kind() string { return "rebuild_vocabulary_browse_counts" }

type BrowseCountsRebuildWorker struct {
	river.WorkerDefaults[BrowseCountsRebuildArgs]
	Pool *pgxpool.Pool
}

func (w *BrowseCountsRebuildWorker) Work(ctx context.Context, job *river.Job[BrowseCountsRebuildArgs]) (err error) {
	a := job.Args
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Browse count rebuild: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	var sourceID, runID, corpusID, language string
	err = tx.QueryRow(ctx, `SELECT cai.source_material_id::text,cai.analysis_run_id::text,cai.corpus_id::text,s.language
		FROM current_analysis_identity cai
		JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id
		WHERE cai.owner_id=$1 AND cai.book_id=$2 AND cai.analysis_run_id=$3`, a.OwnerID, a.BookID, a.RunID).
		Scan(&sourceID, &runID, &corpusID, &language)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // the queued projection became obsolete before it started
	}
	if err != nil {
		return fmt.Errorf("load current analysis for Browse rebuild: %w", err)
	}
	if err = persistence.BuildVocabularyBrowseCountsTx(ctx, tx, a.OwnerID, a.BookID, sourceID, runID, corpusID, language); err != nil {
		return err
	}
	var stillCurrent bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND corpus_id=$4)`, a.OwnerID, a.BookID, runID, corpusID).Scan(&stillCurrent); err != nil {
		return fmt.Errorf("verify Browse rebuild identity: %w", err)
	}
	if !stillCurrent {
		return nil // rollback the candidate; a later reconciliation queues the current identity
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("publish rebuilt Browse counts: %w", err)
	}
	return nil
}

// EnqueueMissingBrowseCountRebuilds places one durable job per current Book
// analysis whose exact projection is not ready. The uniqueness fields make
// startup reconciliation safe to repeat.
func (s *Service) EnqueueMissingBrowseCountRebuilds(ctx context.Context) (err error) {
	if s == nil || s.pool == nil || s.client == nil {
		return errors.New("Browse rebuild service is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Browse rebuild scheduling: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	rows, err := tx.Query(ctx, `SELECT cai.owner_id::text,cai.book_id::text,cai.analysis_run_id::text
		FROM current_analysis_identity cai
		JOIN source_materials source ON source.owner_id=cai.owner_id AND source.id=cai.source_material_id
		LEFT JOIN vocabulary_browse_count_readiness ready ON ready.owner_id=cai.owner_id AND ready.book_id=cai.book_id
			AND ready.analysis_run_id=cai.analysis_run_id AND ready.corpus_id=cai.corpus_id
			AND ready.language=source.language AND ready.builder_version=1
		WHERE ready.owner_id IS NULL ORDER BY cai.owner_id,cai.book_id`)
	if err != nil {
		return fmt.Errorf("find Books missing ready Browse counts: %w", err)
	}
	var args []BrowseCountsRebuildArgs
	for rows.Next() {
		var a BrowseCountsRebuildArgs
		if err = rows.Scan(&a.OwnerID, &a.BookID, &a.RunID); err != nil {
			rows.Close()
			return fmt.Errorf("read Books missing Browse counts: %w", err)
		}
		args = append(args, a)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate Books missing Browse counts: %w", err)
	}
	rows.Close()
	for _, a := range args {
		if _, err = s.client.InsertTx(ctx, tx, a, &river.InsertOpts{Queue: Queue, MaxAttempts: 10}); err != nil {
			return fmt.Errorf("enqueue Browse rebuild for Book %s: %w", a.BookID, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Browse rebuild scheduling: %w", err)
	}
	return nil
}

// EnqueueBrowseCountRebuild schedules the exact current analysis for a Book.
// It is used after an occurrence decision invalidates that Book's readiness.
func (s *Service) EnqueueBrowseCountRebuild(ctx context.Context, ownerID, bookID string) (err error) {
	if s == nil || s.pool == nil || s.client == nil {
		return errors.New("Browse rebuild service is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Browse rebuild scheduling: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	var runID string
	err = tx.QueryRow(ctx, `SELECT cai.analysis_run_id::text FROM current_analysis_identity cai
		WHERE cai.owner_id=$1 AND cai.book_id=$2`, ownerID, bookID).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load current analysis for Browse rebuild: %w", err)
	}
	var ready bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM current_analysis_identity cai
		JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id
		JOIN vocabulary_browse_count_readiness r ON r.owner_id=cai.owner_id AND r.book_id=cai.book_id
		 AND r.analysis_run_id=cai.analysis_run_id AND r.corpus_id=cai.corpus_id
		 AND r.language=s.language AND r.builder_version=1
		WHERE cai.owner_id=$1 AND cai.book_id=$2)`, ownerID, bookID).Scan(&ready); err != nil {
		return fmt.Errorf("check Browse rebuild readiness: %w", err)
	}
	if ready {
		return nil
	}
	args := BrowseCountsRebuildArgs{OwnerID: ownerID, BookID: bookID, RunID: runID}
	if _, err = s.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: Queue, MaxAttempts: 10}); err != nil {
		return fmt.Errorf("enqueue Browse rebuild for Book %s: %w", bookID, err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Browse rebuild scheduling: %w", err)
	}
	return nil
}
