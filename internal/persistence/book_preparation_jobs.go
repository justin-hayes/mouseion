package persistence

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListBookPreparationJobs returns the owner's Book preparation rows: the one
// shape /jobs shows Classical preparation, EPUB acquisition and analysis jobs
// in, newest first. An EPUB preparation contributes its acquisition row and
// its analysis row; every row names and links to its Book.
func (s *PostgresStore) ListBookPreparationJobs(ctx context.Context, owner string) ([]domain.BookPreparationJob, error) {
	analysisJobs, err := s.ListAnalysisJobs(ctx, owner)
	if err != nil {
		return nil, err
	}
	classicalJobs, err := s.listClassicalPreparationJobs(ctx, owner)
	if err != nil {
		return nil, err
	}
	acquisitionJobs, err := s.listEPUBAcquisitionJobs(ctx, owner)
	if err != nil {
		return nil, err
	}
	jobs := make([]domain.BookPreparationJob, 0, len(analysisJobs)+len(classicalJobs)+len(acquisitionJobs))
	jobs = append(jobs, classicalJobs...)
	jobs = append(jobs, acquisitionJobs...)
	for _, job := range analysisJobs {
		progress := job.Progress
		jobs = append(jobs, domain.BookPreparationJob{
			Kind:          domain.BookPreparationKindAnalysis,
			JobID:         job.ID,
			DisplayNumber: job.DisplayNumber,
			BookID:        job.BookID,
			BookTitle:     job.BookTitle,
			Lifecycle:     job.Lifecycle,
			Stage:         job.Stage,
			Code:          job.FailureCode,
			Progress:      &progress,
			CreatedAt:     job.CreatedAt,
		})
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	return jobs, nil
}

// preparationJobRow is one River-sourced preparation row before the domain
// lifecycle mapping. Stage and Code are empty unless the job settled with a
// recorded Book preparation failure.
type preparationJobRow struct {
	ID        int64
	BookID    string
	BookTitle string
	State     string
	Stage     string
	Code      string
	CreatedAt time.Time
}

// preparationJobFromRow maps one River-sourced row onto the shared shape. The
// stage and code are the recorded failure's own values, so a failed row states
// the same evidence My Books explains for that Book.
func preparationJobFromRow(row preparationJobRow, kind domain.BookPreparationKind) domain.BookPreparationJob {
	return domain.BookPreparationJob{
		Kind:      kind,
		JobID:     row.ID,
		BookID:    row.BookID,
		BookTitle: row.BookTitle,
		Lifecycle: domain.JobLifecycleForRiverState(row.State),
		Stage:     domain.BookPreparationStage(row.Stage),
		Code:      domain.BookPreparationCode(row.Code),
		CreatedAt: row.CreatedAt,
	}
}

// queryPreparationJobs runs one River-sourced preparation query, newest first,
// and maps its rows onto the shared Book preparation shape.
func (s *PostgresStore) queryPreparationJobs(ctx context.Context, kind domain.BookPreparationKind, label, query string, args ...any) ([]domain.BookPreparationJob, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", label, err)
	}
	defer rows.Close()
	jobs := make([]domain.BookPreparationJob, 0)
	for rows.Next() {
		var row preparationJobRow
		if err := rows.Scan(&row.ID, &row.BookID, &row.BookTitle, &row.State, &row.Stage, &row.Code, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan %s: %w", label, err)
		}
		jobs = append(jobs, preparationJobFromRow(row, kind))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list %s: %w", label, err)
	}
	return jobs, nil
}

// listClassicalPreparationJobs returns the owner's classical preparation rows,
// newest first, including a failure whose revision has no River job at all.
// The query is parameterized by the owner's uuid ($1).
func (s *PostgresStore) listClassicalPreparationJobs(ctx context.Context, owner string) ([]domain.BookPreparationJob, error) {
	return s.queryPreparationJobs(ctx, domain.BookPreparationKindClassical, "classical preparation jobs", `
		WITH history AS (
			SELECT job.id, book.id::text AS book_id, book.title, job.state::text AS state,
			       CASE WHEN job.state IN ('discarded', 'cancelled') THEN COALESCE(failure.stage, '') ELSE '' END AS stage,
			       CASE WHEN job.state IN ('discarded', 'cancelled') THEN COALESCE(failure.code, '') ELSE '' END AS code,
			       job.created_at
			FROM river_job job
			JOIN books book ON book.owner_id = $1::uuid AND book.id = (job.args->>'book_id')::uuid
			LEFT JOIN book_preparation_failures failure
			       ON failure.owner_id = book.owner_id AND failure.book_id = book.id
			      AND failure.reading_unit_key IS NOT NULL
			      AND failure.definition_revision = COALESCE((job.args->>'definition_revision')::integer, 0)
			WHERE job.kind = 'classical_preparation' AND job.args->>'owner_id' = $1::text
		)
		SELECT id, book_id, title, state, stage, code, created_at
		FROM history
		UNION ALL
		SELECT 0, book.id::text, book.title, 'failed', failure.stage, failure.code, failure.failed_at
		FROM book_preparation_failures failure
		JOIN books book ON book.owner_id = failure.owner_id AND book.id = failure.book_id
		WHERE failure.owner_id = $1
		  AND failure.reading_unit_key IS NOT NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM river_job job
		      WHERE job.kind = 'classical_preparation'
		        AND job.args->>'owner_id' = $1::text
		        AND job.args->>'book_id' = book.id::text
		        AND COALESCE((job.args->>'definition_revision')::integer, 0) = failure.definition_revision
		  )
		ORDER BY created_at DESC, id DESC
		LIMIT 100`, owner)
}

// listEPUBAcquisitionJobs returns the owner's EPUB acquisition rows, newest
// first. A live job supersedes the Book's older failure record, exactly as My
// Books reads it, and a failure recorded by the inline path, with no River job
// at all, stays visible as its own failed row. The query is parameterized by
// the owner's uuid ($1).
func (s *PostgresStore) listEPUBAcquisitionJobs(ctx context.Context, owner string) ([]domain.BookPreparationJob, error) {
	return s.queryPreparationJobs(ctx, domain.BookPreparationKindEPUBAcquisition, "EPUB acquisition jobs", `
		WITH history AS (
			SELECT job.id, book.id::text AS book_id, book.title, job.state::text AS state,
			       CASE WHEN job.state IN ('discarded', 'cancelled') THEN COALESCE(failure.stage, '') ELSE '' END AS stage,
			       CASE WHEN job.state IN ('discarded', 'cancelled') THEN COALESCE(failure.code, '') ELSE '' END AS code,
			       job.created_at
			FROM river_job job
			JOIN books book ON book.owner_id = $1::uuid AND book.id = (job.args->>'book_id')::uuid
			LEFT JOIN book_preparation_failures failure
			       ON failure.owner_id = book.owner_id AND failure.book_id = book.id
			      AND failure.reading_unit_key IS NULL
			WHERE job.kind = 'epub_acquisition' AND job.args->>'owner_id' = $1::text
		)
		SELECT id, book_id, title, state, stage, code, created_at
		FROM history
		UNION ALL
		SELECT 0, book.id::text, book.title, 'failed', failure.stage, failure.code, failure.failed_at
		FROM book_preparation_failures failure
		JOIN books book ON book.owner_id = failure.owner_id AND book.id = failure.book_id
		WHERE failure.owner_id = $1
		  AND failure.reading_unit_key IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM river_job job
		      WHERE job.kind = 'epub_acquisition'
		        AND job.args->>'owner_id' = $1::text
		        AND job.args->>'book_id' = book.id::text
		  )
		ORDER BY created_at DESC, id DESC
		LIMIT 100`, owner)
}
