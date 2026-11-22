//go:build integration

package persistence

import (
	"context"
	"encoding/json"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListBookPreparationJobsReturnsEveryKindInOneShapeNewestFirst proves the
// one list seam: Classical preparation, EPUB acquisition and analysis rows come
// back as one shape, newest first, with each row naming and linking to its
// Book, and another owner's rows never leak in.
func TestListBookPreparationJobsReturnsEveryKindInOneShapeNewestFirst(t *testing.T) {
	ctx := context.Background()
	f := newBookPreparationFailureFixture(t)

	classicalAt := time.Date(2026, time.November, 1, 10, 0, 0, 0, time.UTC)
	acquisitionAt := time.Date(2026, time.November, 2, 10, 0, 0, 0, time.UTC)
	analysisAt := time.Date(2026, time.November, 3, 10, 0, 0, 0, time.UTC)

	classicalJobID := insertPreparationRiverJob(t, ctx, f.pool, "classical_preparation", f.owner, f.classicalBookID,
		map[string]any{"reading_unit_key": f.readingUnitKey, "definition_revision": f.revision},
		"discarded", classicalAt)
	acquisitionJobID := insertPreparationRiverJob(t, ctx, f.pool, "epub_acquisition", f.owner, f.epubBookID,
		nil, "discarded", acquisitionAt)
	require.NoError(t, f.store.PutClassicalPreparationFailure(ctx, domain.ClassicalPreparationFailureRecord{
		OwnerID: f.owner, BookID: f.classicalBookID, ReadingUnitKey: f.readingUnitKey,
		DefinitionRevision: f.revision,
		Stage:              domain.BookPreparationStageSource,
		Code:               domain.BookPreparationCodeSourcePinMismatch,
	}))
	require.NoError(t, f.store.RecordEPUBAcquisitionFailure(ctx, domain.EPUBAcquisitionFailureRecord{
		OwnerID: f.owner, BookID: f.epubBookID, ConnectionID: f.connectionID, EntryIdentifier: "entry-1",
		Stage: domain.BookPreparationStageFetch,
		Code:  domain.BookPreparationCodeSourceDownloadFailed,
	}))
	analysis := insertFailedAnalysisJob(t, ctx, f.pool, f.store, f.owner, "Analysis book", 42, analysisAt)

	jobs, err := f.store.ListBookPreparationJobs(ctx, f.owner)
	require.NoError(t, err)
	require.Len(t, jobs, 3, "one row per Book preparation kind: %+v", jobs)

	t.Run("newest first across every kind", func(t *testing.T) {
		assert.Equal(t, []domain.BookPreparationKind{
			domain.BookPreparationKindAnalysis,
			domain.BookPreparationKindEPUBAcquisition,
			domain.BookPreparationKindClassical,
		}, []domain.BookPreparationKind{jobs[0].Kind, jobs[1].Kind, jobs[2].Kind})
	})

	t.Run("the analysis row carries its run identity, Book and progress", func(t *testing.T) {
		row := jobs[0]
		assert.Equal(t, analysis.jobID, row.JobID)
		assert.Equal(t, int64(1), row.DisplayNumber)
		assert.Equal(t, analysis.bookID, row.BookID)
		assert.Equal(t, "Analysis book", row.BookTitle)
		assert.Equal(t, domain.JobLifecycleFailed, row.Lifecycle)
		assert.Equal(t, domain.BookPreparationStageAnalysis, row.Stage)
		assert.Equal(t, domain.BookPreparationCodeDependencyParsingUnavailable, row.Code)
		require.NotNil(t, row.Progress)
		assert.Equal(t, 42, *row.Progress)
	})

	t.Run("the acquisition row shows the failure My Books explains", func(t *testing.T) {
		row := jobs[1]
		assert.Equal(t, acquisitionJobID, row.JobID)
		assert.Equal(t, f.epubBookID, row.BookID)
		assert.Equal(t, "An EPUB book", row.BookTitle)
		assert.Equal(t, domain.JobLifecycleFailed, row.Lifecycle)
		assert.Equal(t, domain.BookPreparationStageFetch, row.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, row.Code)
		assert.Nil(t, row.Progress, "only analysis reports progress")
	})

	t.Run("the classical row shows the failure My Books explains", func(t *testing.T) {
		row := jobs[2]
		assert.Equal(t, classicalJobID, row.JobID)
		assert.Equal(t, f.classicalBookID, row.BookID)
		assert.Equal(t, domain.JobLifecycleFailed, row.Lifecycle)
		assert.Equal(t, domain.BookPreparationStageSource, row.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourcePinMismatch, row.Code)
		assert.Nil(t, row.Progress, "only analysis reports progress")
	})

	t.Run("another owner sees no rows", func(t *testing.T) {
		other, err := f.store.CreateUser(ctx, "preparation-list-other-owner", false)
		require.NoError(t, err)
		otherJobs, err := f.store.ListBookPreparationJobs(ctx, other.ID)
		require.NoError(t, err)
		assert.Empty(t, otherJobs)
	})
}

// TestListBookPreparationJobsKeepsAcquisitionEvidenceHonest covers the two
// acquisition states beyond a plain failed job: a live job supersedes the
// Book's older failure record exactly as My Books reads it, and a failure
// recorded by the inline path, with no River job at all, still appears as its
// own failed row. A Classical failure whose revision has no job appears the
// same way.
func TestListBookPreparationJobsKeepsAcquisitionEvidenceHonest(t *testing.T) {
	ctx := context.Background()
	f := newBookPreparationFailureFixture(t)

	legacyBook, err := f.store.CreateBook(ctx, domain.Book{
		OwnerID:            f.owner,
		Title:              "Legacy acquisition book",
		MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        "de",
	})
	require.NoError(t, err)

	// A live acquisition job for the fixture's EPUB Book, whose failure record
	// is therefore superseded while the job runs.
	insertPreparationRiverJob(t, ctx, f.pool, "epub_acquisition", f.owner, f.epubBookID, nil, "running", time.Date(2026, time.November, 2, 9, 0, 0, 0, time.UTC))
	require.NoError(t, f.store.RecordEPUBAcquisitionFailure(ctx, domain.EPUBAcquisitionFailureRecord{
		OwnerID: f.owner, BookID: f.epubBookID, ConnectionID: f.connectionID, EntryIdentifier: "entry-1",
		Stage: domain.BookPreparationStageFetch,
		Code:  domain.BookPreparationCodeSourceDownloadFailed,
	}))
	// A second EPUB Book failed inline, before acquisition became durable, so
	// its failure record has no River job.
	require.NoError(t, f.store.RecordEPUBAcquisitionFailure(ctx, domain.EPUBAcquisitionFailureRecord{
		OwnerID: f.owner, BookID: legacyBook.ID, ConnectionID: f.connectionID, EntryIdentifier: "entry-2",
		Stage: domain.BookPreparationStageImport,
		Code:  domain.BookPreparationCodeEPUBImportFailed,
	}))
	// A second Classical Book whose failure record has no matching River job.
	report, err := f.store.AddClassicalReadingUnits(ctx, f.owner, []string{"work-2"})
	require.NoError(t, err)
	require.NotEmpty(t, report.Books)
	classicalLegacy := report.Books[0]
	require.NoError(t, f.store.PutClassicalPreparationFailure(ctx, domain.ClassicalPreparationFailureRecord{
		OwnerID: f.owner, BookID: classicalLegacy.BookID, ReadingUnitKey: "work-2",
		DefinitionRevision: 1,
		Stage:              domain.BookPreparationStageFetch,
		Code:               domain.BookPreparationCodeSourceDownloadFailed,
	}))

	jobs, err := f.store.ListBookPreparationJobs(ctx, f.owner)
	require.NoError(t, err)
	require.Len(t, jobs, 3, "live row, legacy EPUB row, legacy Classical row: %+v", jobs)

	byBook := make(map[string]domain.BookPreparationJob, len(jobs))
	for _, job := range jobs {
		byBook[job.BookID] = job
	}

	t.Run("a live acquisition job supersedes its Book's failure record", func(t *testing.T) {
		row := byBook[f.epubBookID]
		assert.Equal(t, domain.BookPreparationKindEPUBAcquisition, row.Kind)
		assert.Equal(t, domain.JobLifecycleRunning, row.Lifecycle)
		assert.Equal(t, domain.BookPreparationStageNone, row.Stage)
		assert.Equal(t, domain.BookPreparationCodeNone, row.Code)
	})

	t.Run("an inline-era acquisition failure stays visible without a job", func(t *testing.T) {
		row := byBook[legacyBook.ID]
		assert.Equal(t, domain.BookPreparationKindEPUBAcquisition, row.Kind)
		assert.Equal(t, domain.JobLifecycleFailed, row.Lifecycle)
		assert.Equal(t, domain.BookPreparationStageImport, row.Stage)
		assert.Equal(t, domain.BookPreparationCodeEPUBImportFailed, row.Code)
	})

	t.Run("a Classical failure with no matching job stays visible", func(t *testing.T) {
		row := byBook[classicalLegacy.BookID]
		assert.Equal(t, domain.BookPreparationKindClassical, row.Kind)
		assert.Equal(t, domain.JobLifecycleFailed, row.Lifecycle)
		assert.Equal(t, domain.BookPreparationStageFetch, row.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, row.Code)
	})
}

// insertPreparationRiverJob inserts one settled or live preparation job the way
// River records it. args may carry extra kind-specific arguments.
func insertPreparationRiverJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, kind, owner, bookID string, args map[string]any, state string, createdAt time.Time) int64 {
	t.Helper()
	jobArgs := map[string]any{"owner_id": owner, "book_id": bookID}
	maps.Copy(jobArgs, args)
	encoded, err := json.Marshal(jobArgs)
	require.NoError(t, err)
	var id int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO river_job(kind, args, state, queue, max_attempts, priority, attempt, created_at, finalized_at)
		VALUES ($1, $2::jsonb, $3::river_job_state, $1, 1, 1, 0, $4::timestamptz,
		        CASE WHEN $3 IN ('discarded', 'cancelled', 'completed') THEN $4::timestamptz ELSE NULL END)
		RETURNING id`,
		kind, encoded, state, createdAt).Scan(&id))
	return id
}

// insertFailedAnalysisJob seeds one failed analysis job with its Book and
// source, returning the identities the /jobs row must carry.
func insertFailedAnalysisJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *PostgresStore, owner, title string, progress int, createdAt time.Time) (job struct {
	jobID, displayNumber int64
	bookID, sourceID     string
}) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner, Title: title,
		MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState:      domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: "de", SourceIdentifier: "preparation-list-" + book.ID,
		Title: title, MediaType: "application/epub+zip", ContentHash: "sha256:preparation-list-" + book.ID,
		Content: []byte("Haus."), FullText: "Haus.",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: []domain.ExtractedUnit{{
		ID: domain.EPUBUnitID(0, book.ID), Order: 0, SpineIndex: 0, ManifestID: book.ID,
		SourceHref: book.ID, ResolvedHref: book.ID, Text: "Haus.",
		EndOffset: 5, MediaType: "application/xhtml+xml", Linear: true,
	}}})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE source_materials SET book_id=$1 WHERE id=$2`, book.ID, source.ID)
	require.NoError(t, err)

	var revisionID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO source_content_revisions(owner_id, source_material_id, digest_version, content_digest, content, full_text)
		VALUES ($1, $2, 0, $3, $4, $5)
		RETURNING revision_id::text`,
		owner, source.ID, "legacy:preparation-list-"+book.ID, []byte("Haus."), "Haus.").Scan(&revisionID))
	var runID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO analysis_runs(owner_id, source_material_id, content_revision_id, snapshot_id,
			analyzer_name, analyzer_version, config_identity, state, failure_code, created_at)
		VALUES ($1, $2, $3, gen_random_uuid(), 'fake', '1', 'fake-identity', 'failed', 'dependency_parsing_unavailable', $4)
		RETURNING id::text`, owner, source.ID, revisionID, createdAt).Scan(&runID))
	const displayNumber int64 = 1
	var jobID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO analysis_jobs(river_job_id, owner_id, source_material_id, content_hash, progress, display_number, analysis_run_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING river_job_id`,
		91001, owner, source.ID, "sha256:preparation-list-"+book.ID, progress, displayNumber, runID, createdAt).Scan(&jobID))
	return struct {
		jobID, displayNumber int64
		bookID, sourceID     string
	}{jobID, displayNumber, book.ID, source.ID}
}
