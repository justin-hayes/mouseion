package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// LoadClassicalPreparationTarget reads the Book, its Reading-unit association
// and the reconciled definition a preparation job works from. It returns
// ErrNotFound when the owner has no such Book.
func (s *PostgresStore) LoadClassicalPreparationTarget(ctx context.Context, owner, bookID string) (domain.ClassicalPreparationTarget, error) {
	var target domain.ClassicalPreparationTarget
	err := s.pool.QueryRow(ctx, `
		SELECT b.owner_id::text, b.id::text, ru.definition_key, w.title, w.author, e.label,
		       ru.citation_start, ru.citation_end,
		       ru.source_layer, ru.source_file_name, ru.source_file_size, ru.source_file_sha256,
		       ru.source_format, ru.interpretation_version, ru.revision, ru.withdrawn
		FROM books b
		JOIN book_reading_units bru ON bru.owner_id = b.owner_id AND bru.book_id = b.id
		JOIN reading_units ru ON ru.id = bru.reading_unit_id
		JOIN editions e ON e.id = ru.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE b.owner_id = $1 AND b.id = $2`, owner, bookID).
		Scan(&target.OwnerID, &target.BookID, &target.ReadingUnitKey, &target.WorkTitle, &target.WorkAuthor, &target.EditionLabel,
			&target.CitationStart, &target.CitationEnd,
			&target.SourceLayer, &target.SourceFileName, &target.SourceFileSize, &target.SourceFileSHA256,
			&target.SourceFormat, &target.InterpretationVersion, &target.DefinitionRevision, &target.Withdrawn)
	if err != nil {
		return domain.ClassicalPreparationTarget{}, missing(err)
	}
	return target, nil
}

// ClassicalPreparationComplete reports whether the Book already published an
// analysis for exactly this definition revision, source pin and interpretation
// version. It is the ensure-once check.
func (s *PostgresStore) ClassicalPreparationComplete(ctx context.Context, target domain.ClassicalPreparationTarget) (bool, error) {
	var complete bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM classical_prepared_definitions p
			JOIN analysis_runs r ON r.owner_id = p.owner_id AND r.id = p.analysis_run_id AND r.state = 'completed'
			WHERE p.owner_id = $1 AND p.book_id = $2
			  AND p.reading_unit_key = $3
			  AND p.definition_revision = $4
			  AND p.source_file_sha256 = $5
			  AND p.interpretation_version = $6
		)`, target.OwnerID, target.BookID, target.ReadingUnitKey, target.DefinitionRevision, target.SourceFileSHA256, target.InterpretationVersion).Scan(&complete)
	if err != nil {
		return false, fmt.Errorf("check classical prepared definition: %w", err)
	}
	return complete, nil
}

// PutClassicalPreparedDefinition records the definition and published run a
// Book's classical analysis came from. A later preparation for the same Book
// rewrites it; the previous published analysis is not touched.
func (s *PostgresStore) PutClassicalPreparedDefinition(ctx context.Context, record domain.ClassicalPreparedDefinition) error {
	if err := validatePreparedDefinition(record); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO classical_prepared_definitions(
			owner_id, book_id, reading_unit_key, definition_revision,
			source_file_sha256, interpretation_version, analysis_run_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (owner_id, book_id) DO UPDATE SET
			reading_unit_key = EXCLUDED.reading_unit_key,
			definition_revision = EXCLUDED.definition_revision,
			source_file_sha256 = EXCLUDED.source_file_sha256,
			interpretation_version = EXCLUDED.interpretation_version,
			analysis_run_id = EXCLUDED.analysis_run_id,
			prepared_at = now()`,
		record.OwnerID, record.BookID, record.ReadingUnitKey, record.DefinitionRevision,
		record.SourceFileSHA256, record.InterpretationVersion, record.AnalysisRunID)
	if err != nil {
		return fmt.Errorf("write classical prepared definition: %w", err)
	}
	return nil
}

// validatePreparedDefinition refuses an incomplete record before the database
// sees it.
func validatePreparedDefinition(record domain.ClassicalPreparedDefinition) error {
	switch {
	case record.OwnerID == "":
		return errors.New("persistence: classical prepared definition owner is required")
	case record.BookID == "":
		return errors.New("persistence: classical prepared definition Book is required")
	case record.ReadingUnitKey == "":
		return errors.New("persistence: classical prepared definition Reading unit is required")
	case record.DefinitionRevision <= 0:
		return errors.New("persistence: classical prepared definition revision must be positive")
	case len(record.SourceFileSHA256) != 64:
		return errors.New("persistence: classical prepared definition source SHA-256 is invalid")
	case record.InterpretationVersion == "":
		return errors.New("persistence: classical prepared definition interpretation version is required")
	case record.AnalysisRunID == "":
		return errors.New("persistence: classical prepared definition analysis run is required")
	}
	return nil
}

// GetClassicalSource reads one stored work file by its pin SHA-256. It reports
// found=false when the source is absent, which is the normal first-preparation
// state.
func (s *PostgresStore) GetClassicalSource(ctx context.Context, sha256 string) (domain.ClassicalSource, bool, error) {
	var source domain.ClassicalSource
	err := s.pool.QueryRow(ctx, `
		SELECT sha256, layer, file_name, byte_size, content
		FROM classical_sources
		WHERE sha256 = $1`, sha256).
		Scan(&source.SHA256, &source.Layer, &source.FileName, &source.Size, &source.Content)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClassicalSource{}, false, nil
	}
	if err != nil {
		return domain.ClassicalSource{}, false, fmt.Errorf("read classical source: %w", err)
	}
	return source, true, nil
}

// PutClassicalSource stores one verified work file. A concurrent insert that
// already exists is a no-op, so two preparations of the same work can race on
// the download without failing.
func (s *PostgresStore) PutClassicalSource(ctx context.Context, source domain.ClassicalSource) error {
	if err := validateClassicalSource(source); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO classical_sources(sha256, layer, file_name, byte_size, content)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (sha256) DO NOTHING`,
		source.SHA256, source.Layer, source.FileName, source.Size, source.Content)
	if err != nil {
		return fmt.Errorf("write classical source: %w", err)
	}
	return nil
}

// validateClassicalSource refuses bytes that cannot be the reviewed pin.
func validateClassicalSource(source domain.ClassicalSource) error {
	switch {
	case len(source.SHA256) != 64:
		return errors.New("persistence: classical source SHA-256 is invalid")
	case source.Layer == "":
		return errors.New("persistence: classical source layer is required")
	case source.FileName == "":
		return errors.New("persistence: classical source file name is required")
	case source.Size <= 0 || source.Size != int64(len(source.Content)):
		return errors.New("persistence: classical source size does not match its content")
	}
	return nil
}

// pruneClassicalSources deletes every stored work file that no current,
// non-withdrawn definition pins. It runs after manifest reconciliation in the
// same transactional startup path.
func pruneClassicalSources(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM classical_sources source
		WHERE NOT EXISTS (
			SELECT 1 FROM reading_units ru
			WHERE ru.source_file_sha256 = source.sha256 AND ru.withdrawn = false
		)`)
	if err != nil {
		return fmt.Errorf("prune classical sources: %w", err)
	}
	return nil
}

// ListClassicalPreparationJobs returns the owner's classical preparation jobs
// with their Book, attempt and River state, newest first.
func (s *PostgresStore) ListClassicalPreparationJobs(ctx context.Context, owner string) ([]domain.ClassicalPreparationJob, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT job.id, book.id::text, book.title, job.attempt, job.state::text, job.created_at
		FROM river_job job
		JOIN books book ON book.owner_id = $1 AND book.id = (job.args->>'book_id')::uuid
		WHERE job.kind = 'classical_preparation' AND job.args->>'owner_id' = $1
		ORDER BY job.created_at DESC, job.id DESC
		LIMIT 100`, owner)
	if err != nil {
		return nil, fmt.Errorf("list classical preparation jobs: %w", err)
	}
	defer rows.Close()
	jobs := make([]domain.ClassicalPreparationJob, 0)
	for rows.Next() {
		var job domain.ClassicalPreparationJob
		if err := rows.Scan(&job.ID, &job.BookID, &job.BookTitle, &job.Attempt, &job.State, &job.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan classical preparation job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list classical preparation jobs: %w", err)
	}
	return jobs, nil
}
