package persistence

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// bookPreparationJobsLimit is the one window the merged /jobs Book preparations
// table shows. Every source query reads at least this many rows (the River
// preparation queries below and sqlc's ListAnalysisJobs both cap at 100), so
// merging the sources and truncating once leaves one consistent window.
const bookPreparationJobsLimit = 100

// ListBookPreparationJobs returns the owner's Book preparation rows: the one
// shape /jobs shows Classical preparation, EPUB acquisition and analysis jobs
// in, newest first. An EPUB preparation contributes its acquisition row and
// its analysis row; every row names and links to its Book, and reports whether
// that Book is still a My Books member, so the row's link resolves exactly
// when My Books would open the Book.
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
	if err := s.markMyBooksMembership(ctx, owner, jobs); err != nil {
		return nil, err
	}
	if err := s.attachDecisionCarryOvers(ctx, owner, jobs); err != nil {
		return nil, err
	}
	sort.SliceStable(jobs, func(i, j int) bool { return bookPreparationJobBefore(jobs[i], jobs[j]) })
	if len(jobs) > bookPreparationJobsLimit {
		jobs = jobs[:bookPreparationJobsLimit]
	}
	return jobs, nil
}

// bookPreparationJobBefore orders two Book preparation rows: newest first,
// then Kind, then the higher job id, so identical timestamps still produce one
// deterministic order.
func bookPreparationJobBefore(a, b domain.BookPreparationJob) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.JobID > b.JobID
}

// markMyBooksMembership populates BookInMyBooks on every row kind. The
// predicate is the one GetBookDetail resolves a Book through — an active
// membership row plus a disposition row — which is the same evidence
// /catalogs/books/{id}/in-my-books trusts before it opens a Book, so a row's
// flag is true exactly when that route would not report the Book missing.
func (s *PostgresStore) markMyBooksMembership(ctx context.Context, owner string, jobs []domain.BookPreparationJob) error {
	ids := make([]string, 0, len(jobs))
	seen := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		if job.BookID == "" || seen[job.BookID] {
			continue
		}
		seen[job.BookID] = true
		ids = append(ids, job.BookID)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT b.id::text
		FROM books b
		JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id AND m.state = 'active'
		JOIN book_dispositions d ON d.owner_id = b.owner_id AND d.book_id = b.id
		WHERE b.owner_id = $1::uuid AND b.id::text = ANY($2::text[])`, owner, ids)
	if err != nil {
		return fmt.Errorf("read My Books membership: %w", err)
	}
	defer rows.Close()
	member := make(map[string]bool, len(ids))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan My Books membership: %w", err)
		}
		member[id] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read My Books membership: %w", err)
	}
	for i := range jobs {
		jobs[i].BookInMyBooks = member[jobs[i].BookID]
	}
	return nil
}

// attachDecisionCarryOvers reports, on the row of the job that published an
// Analysis update's run, how many decisions carried over (ADR 0126). An analysis
// row is found through its run. A Classical preparation job does not name its
// run, so the carry-over row records the River job that published it, inside the
// publishing transaction. The link holds whatever the job's final state: the run
// is published before the job's last write, so a job that failed afterwards and
// was retried or discarded still published it. A run with no decisions on the replaced run has nothing
// to report and is left alone.
func (s *PostgresStore) attachDecisionCarryOvers(ctx context.Context, owner string, jobs []domain.BookPreparationJob) error {
	rows, err := s.pool.Query(ctx, `
		SELECT 'analysis', aj.river_job_id, co.carried, co.total
		FROM analysis_decision_carry_overs co
		JOIN analysis_jobs aj ON aj.owner_id = co.owner_id AND aj.analysis_run_id = co.analysis_run_id
		WHERE co.owner_id = $1 AND co.total > 0
		UNION ALL
		SELECT 'classical', co.river_job_id, co.carried, co.total
		FROM analysis_decision_carry_overs co
		WHERE co.owner_id = $1 AND co.total > 0 AND co.river_job_id IS NOT NULL`, owner)
	if err != nil {
		return fmt.Errorf("list carried-over decisions: %w", err)
	}
	defer rows.Close()
	type key struct {
		kind domain.BookPreparationKind
		id   int64
	}
	carried := map[key][2]int{}
	for rows.Next() {
		var kind string
		var id int64
		var done, total int
		if err := rows.Scan(&kind, &id, &done, &total); err != nil {
			return fmt.Errorf("scan carried-over decisions: %w", err)
		}
		row := domain.BookPreparationKindAnalysis
		if kind == "classical" {
			row = domain.BookPreparationKindClassical
		}
		carried[key{row, id}] = [2]int{done, total}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read carried-over decisions: %w", err)
	}
	for i := range jobs {
		if counts, ok := carried[key{jobs[i].Kind, jobs[i].JobID}]; ok {
			jobs[i].DecisionsCarried, jobs[i].DecisionsTotal = counts[0], counts[1]
		}
	}
	return nil
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
// newest first, with the failure evidence read the way My Books reads it:
// within one definition revision the Book's latest job carries the recorded
// stage and code when it settled discarded or cancelled; when that latest job
// completed, or the revision has no job at all, the record stays visible as
// its own synthetic failed row; while the latest job for the revision is
// queued or running the failure is hidden, exactly as
// populateClassicalPreparationPointers reads it. Older jobs for the revision
// never repeat the failure. Book ids and revisions are compared as text, so
// malformed job args can never abort the page. The query is parameterized by
// the owner's uuid ($1).
func (s *PostgresStore) listClassicalPreparationJobs(ctx context.Context, owner string) ([]domain.BookPreparationJob, error) {
	return s.queryPreparationJobs(ctx, domain.BookPreparationKindClassical, "classical preparation jobs", `
		WITH history AS (
			SELECT job.id, book.id::text AS book_id, book.title, job.state::text AS state,
			       CASE WHEN job.state IN ('discarded', 'cancelled') AND job.id = latest.id
			            THEN COALESCE(failure.stage, '') ELSE '' END AS stage,
			       CASE WHEN job.state IN ('discarded', 'cancelled') AND job.id = latest.id
			            THEN COALESCE(failure.code, '') ELSE '' END AS code,
			       job.created_at
			FROM river_job job
			JOIN books book ON book.owner_id = $1::uuid AND book.id::text = job.args->>'book_id'
			LEFT JOIN LATERAL (
			    SELECT j.id
			    FROM river_job j
			    WHERE j.kind = 'classical_preparation'
			      AND j.args->>'owner_id' = book.owner_id::text
			      AND j.args->>'book_id' = book.id::text
			      AND COALESCE(j.args->>'definition_revision', '0') = COALESCE(job.args->>'definition_revision', '0')
			    ORDER BY j.id DESC
			    LIMIT 1
			) latest ON true
			LEFT JOIN book_preparation_failures failure
			       ON failure.owner_id = book.owner_id AND failure.book_id = book.id
			      AND failure.reading_unit_key IS NOT NULL
			      AND COALESCE(job.args->>'definition_revision', '0') = failure.definition_revision::text
			WHERE job.kind = 'classical_preparation' AND job.args->>'owner_id' = $1::text
		)
		SELECT id, book_id, title, state, stage, code, created_at
		FROM history
		UNION ALL
		SELECT 0, book.id::text, book.title, 'failed', failure.stage, failure.code, failure.failed_at
		FROM book_preparation_failures failure
		JOIN books book ON book.owner_id = failure.owner_id AND book.id = failure.book_id
		LEFT JOIN LATERAL (
		    SELECT j.state
		    FROM river_job j
		    WHERE j.kind = 'classical_preparation'
		      AND j.args->>'owner_id' = book.owner_id::text
		      AND j.args->>'book_id' = book.id::text
		      AND COALESCE(j.args->>'definition_revision', '0') = failure.definition_revision::text
		    ORDER BY j.id DESC
		    LIMIT 1
		) latest ON true
		WHERE failure.owner_id = $1
		  AND failure.reading_unit_key IS NOT NULL
		  AND (latest.state IS NULL OR latest.state = 'completed')
		ORDER BY created_at DESC, id DESC
		LIMIT 100`, owner)
}

// listEPUBAcquisitionJobs returns the owner's EPUB acquisition rows, newest
// first, with the failure evidence read exactly the way My Books reads it: a
// Book's failure record is visible exactly once, and only while no live job
// supersedes it. The Book's latest acquisition job (the highest job id)
// carries the recorded stage and code when it settled discarded or cancelled;
// when that latest job completed, or the Book has no job at all, the record
// stays visible as its own synthetic failed row; while the latest job is
// queued or running the failure is hidden, exactly as
// populateEPUBAcquisitionStatuses reads it. Older jobs never repeat the
// failure. Book ids are compared as text, so malformed job args can never
// abort the page. The query is parameterized by the owner's uuid ($1).
func (s *PostgresStore) listEPUBAcquisitionJobs(ctx context.Context, owner string) ([]domain.BookPreparationJob, error) {
	return s.queryPreparationJobs(ctx, domain.BookPreparationKindEPUBAcquisition, "EPUB acquisition jobs", `
		WITH history AS (
			SELECT job.id, book.id::text AS book_id, book.title, job.state::text AS state,
			       CASE WHEN job.state IN ('discarded', 'cancelled') AND job.id = latest.id
			            THEN COALESCE(failure.stage, '') ELSE '' END AS stage,
			       CASE WHEN job.state IN ('discarded', 'cancelled') AND job.id = latest.id
			            THEN COALESCE(failure.code, '') ELSE '' END AS code,
			       job.created_at
			FROM river_job job
			JOIN books book ON book.owner_id = $1::uuid AND book.id::text = job.args->>'book_id'
			LEFT JOIN LATERAL (
			    SELECT j.id
			    FROM river_job j
			    WHERE j.kind = 'epub_acquisition'
			      AND j.args->>'owner_id' = book.owner_id::text
			      AND j.args->>'book_id' = book.id::text
			    ORDER BY j.id DESC
			    LIMIT 1
			) latest ON true
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
		LEFT JOIN LATERAL (
		    SELECT j.state
		    FROM river_job j
		    WHERE j.kind = 'epub_acquisition'
		      AND j.args->>'owner_id' = book.owner_id::text
		      AND j.args->>'book_id' = book.id::text
		    ORDER BY j.id DESC
		    LIMIT 1
		) latest ON true
		WHERE failure.owner_id = $1
		  AND failure.reading_unit_key IS NULL
		  AND (latest.state IS NULL OR latest.state = 'completed')
		ORDER BY created_at DESC, id DESC
		LIMIT 100`, owner)
}
