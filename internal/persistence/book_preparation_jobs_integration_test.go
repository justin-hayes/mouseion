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

// TestListBookPreparationJobsPlacesAcquisitionFailureOnTheLatestJob pins the
// My Books rule on /jobs: only a Book's latest acquisition job may state the
// failure record, a live latest job hides the record entirely, and a record no
// job row states — because the latest job completed, or no job exists — stays
// visible as its own failed row exactly once.
func TestListBookPreparationJobsPlacesAcquisitionFailureOnTheLatestJob(t *testing.T) {
	ctx := context.Background()
	f := newBookPreparationFailureFixture(t)

	newBook := func(title string) string {
		book, err := f.store.CreateBook(ctx, domain.Book{
			OwnerID:            f.owner,
			Title:              title,
			MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
			LanguageState:      domain.LanguageChosen,
			LanguageTag:        "de",
		})
		require.NoError(t, err)
		return book.ID
	}
	recordFailure := func(bookID, entry string) {
		require.NoError(t, f.store.RecordEPUBAcquisitionFailure(ctx, domain.EPUBAcquisitionFailureRecord{
			OwnerID: f.owner, BookID: bookID, ConnectionID: f.connectionID, EntryIdentifier: entry,
			Stage: domain.BookPreparationStageFetch,
			Code:  domain.BookPreparationCodeSourceDownloadFailed,
		}))
	}

	// Two settled attempts for one Book; only the newest row may state the
	// record.
	twoAttempts := newBook("Two discarded attempts")
	older := insertPreparationRiverJob(t, ctx, f.pool, "epub_acquisition", f.owner, twoAttempts, nil,
		"discarded", time.Date(2026, time.November, 10, 9, 0, 0, 0, time.UTC))
	newest := insertPreparationRiverJob(t, ctx, f.pool, "epub_acquisition", f.owner, twoAttempts, nil,
		"discarded", time.Date(2026, time.November, 11, 9, 0, 0, 0, time.UTC))
	recordFailure(twoAttempts, "entry-a")

	// A cancelled older attempt under a live newest one: My Books shows the
	// live job and hides the record, so no row states it.
	cancelledUnderLive := newBook("Cancelled under a live attempt")
	insertPreparationRiverJob(t, ctx, f.pool, "epub_acquisition", f.owner, cancelledUnderLive, nil,
		"cancelled", time.Date(2026, time.November, 10, 10, 0, 0, 0, time.UTC))
	insertPreparationRiverJob(t, ctx, f.pool, "epub_acquisition", f.owner, cancelledUnderLive, nil,
		"running", time.Date(2026, time.November, 11, 10, 0, 0, 0, time.UTC))
	recordFailure(cancelledUnderLive, "entry-b")

	// The latest attempt completed while the record still exists: the completed
	// row states nothing, and the record stays visible as its own failed row,
	// exactly as My Books reads the same evidence.
	completedWithRecord := newBook("Completed with a failure record")
	completed := insertPreparationRiverJob(t, ctx, f.pool, "epub_acquisition", f.owner, completedWithRecord, nil,
		"completed", time.Date(2026, time.November, 12, 10, 0, 0, 0, time.UTC))
	recordFailure(completedWithRecord, "entry-c")

	jobs, err := f.store.ListBookPreparationJobs(ctx, f.owner)
	require.NoError(t, err)
	require.Len(t, jobs, 6, "two rows per Book: %+v", jobs)

	t.Run("only the newest settled attempt carries the failure", func(t *testing.T) {
		rows := preparationRowsFor(t, jobs, twoAttempts)
		require.Len(t, rows, 2, "both attempts stay in the history: %+v", rows)
		byID := make(map[int64]domain.BookPreparationJob, len(rows))
		for _, row := range rows {
			byID[row.JobID] = row
		}
		assert.Equal(t, domain.BookPreparationStageFetch, byID[newest].Stage)
		assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, byID[newest].Code)
		assert.Equal(t, domain.BookPreparationStageNone, byID[older].Stage)
		assert.Equal(t, domain.BookPreparationCodeNone, byID[older].Code)
	})

	t.Run("a cancelled older attempt under a live job states nothing", func(t *testing.T) {
		rows := preparationRowsFor(t, jobs, cancelledUnderLive)
		require.Len(t, rows, 2, "the live job and its cancelled predecessor: %+v", rows)
		assert.Equal(t, []domain.JobLifecycleState{domain.JobLifecycleRunning, domain.JobLifecycleCancelled},
			[]domain.JobLifecycleState{rows[0].Lifecycle, rows[1].Lifecycle})
		for _, row := range rows {
			assert.Equal(t, domain.BookPreparationStageNone, row.Stage)
			assert.Equal(t, domain.BookPreparationCodeNone, row.Code)
		}
	})

	t.Run("a completed latest attempt keeps the record visible exactly once", func(t *testing.T) {
		rows := preparationRowsFor(t, jobs, completedWithRecord)
		require.Len(t, rows, 2, "the completed attempt plus the record's own failed row: %+v", rows)
		stating := 0
		var completedRow, failedRow domain.BookPreparationJob
		for _, row := range rows {
			if row.Stage != domain.BookPreparationStageNone {
				stating++
				failedRow = row
				continue
			}
			completedRow = row
		}
		assert.Equal(t, 1, stating, "the record is stated exactly once: %+v", rows)
		assert.Equal(t, completed, completedRow.JobID)
		assert.Equal(t, domain.JobLifecycleCompleted, completedRow.Lifecycle)
		assert.Equal(t, int64(0), failedRow.JobID, "the record's own row has no job identity")
		assert.Equal(t, domain.JobLifecycleFailed, failedRow.Lifecycle)
		assert.Equal(t, domain.BookPreparationStageFetch, failedRow.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, failedRow.Code)
	})

	t.Run("My Books reads the same three Books the same way", func(t *testing.T) {
		status := func(bookID string) *domain.EPUBAcquisitionStatus {
			detail, err := f.store.GetBookDetail(ctx, f.owner, bookID)
			require.NoError(t, err)
			return detail.EPUBAcquisition
		}
		settled := status(twoAttempts)
		require.NotNil(t, settled)
		assert.Equal(t, domain.BookPreparationStageFetch, settled.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, settled.Code)

		live := status(cancelledUnderLive)
		require.NotNil(t, live)
		assert.Equal(t, domain.JobLifecycleRunning, live.State)
		assert.Equal(t, domain.BookPreparationStageNone, live.Stage)

		completedStatus := status(completedWithRecord)
		require.NotNil(t, completedStatus)
		assert.Equal(t, domain.BookPreparationStageFetch, completedStatus.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, completedStatus.Code)
	})
}

// TestListBookPreparationJobsPlacesClassicalFailureOnTheLatestJobForTheRevision
// pins the same rule per definition revision: within a revision only the
// latest job states the record, older attempts never repeat it, and a record
// whose latest revision job completed stays visible as its own failed row,
// exactly as My Books reads the same evidence.
func TestListBookPreparationJobsPlacesClassicalFailureOnTheLatestJobForTheRevision(t *testing.T) {
	ctx := context.Background()
	f := newBookPreparationFailureFixture(t)

	// The fixture's classical Book: two settled attempts for its revision; only
	// the newest may state the record.
	older := insertPreparationRiverJob(t, ctx, f.pool, "classical_preparation", f.owner, f.classicalBookID,
		map[string]any{"reading_unit_key": f.readingUnitKey, "definition_revision": f.revision},
		"discarded", time.Date(2026, time.November, 10, 9, 0, 0, 0, time.UTC))
	newest := insertPreparationRiverJob(t, ctx, f.pool, "classical_preparation", f.owner, f.classicalBookID,
		map[string]any{"reading_unit_key": f.readingUnitKey, "definition_revision": f.revision},
		"discarded", time.Date(2026, time.November, 11, 9, 0, 0, 0, time.UTC))
	require.NoError(t, f.store.PutClassicalPreparationFailure(ctx, domain.ClassicalPreparationFailureRecord{
		OwnerID: f.owner, BookID: f.classicalBookID, ReadingUnitKey: f.readingUnitKey,
		DefinitionRevision: f.revision,
		Stage:              domain.BookPreparationStageSource,
		Code:               domain.BookPreparationCodeSourcePinMismatch,
	}))

	// A second classical Book whose only attempt for the revision completed
	// while the record still exists.
	report, err := f.store.AddClassicalReadingUnits(ctx, f.owner, []string{"work-2"})
	require.NoError(t, err)
	require.Len(t, report.Books, 1)
	second := report.Books[0]
	secondDetail, err := f.store.GetBookDetail(ctx, f.owner, second.BookID)
	require.NoError(t, err)
	require.NotNil(t, secondDetail.ReadingUnit)
	insertPreparationRiverJob(t, ctx, f.pool, "classical_preparation", f.owner, second.BookID,
		map[string]any{"reading_unit_key": second.ReadingUnitKey, "definition_revision": secondDetail.ReadingUnit.Revision},
		"completed", time.Date(2026, time.November, 12, 9, 0, 0, 0, time.UTC))
	require.NoError(t, f.store.PutClassicalPreparationFailure(ctx, domain.ClassicalPreparationFailureRecord{
		OwnerID: f.owner, BookID: second.BookID, ReadingUnitKey: second.ReadingUnitKey,
		DefinitionRevision: secondDetail.ReadingUnit.Revision,
		Stage:              domain.BookPreparationStageFetch,
		Code:               domain.BookPreparationCodeSourceDownloadFailed,
	}))

	jobs, err := f.store.ListBookPreparationJobs(ctx, f.owner)
	require.NoError(t, err)
	require.Len(t, jobs, 4, "two rows per classical Book: %+v", jobs)

	t.Run("only the newest attempt for the revision carries the failure", func(t *testing.T) {
		rows := preparationRowsFor(t, jobs, f.classicalBookID)
		require.Len(t, rows, 2, "both attempts stay in the history: %+v", rows)
		byID := make(map[int64]domain.BookPreparationJob, len(rows))
		for _, row := range rows {
			byID[row.JobID] = row
		}
		assert.Equal(t, domain.BookPreparationStageSource, byID[newest].Stage)
		assert.Equal(t, domain.BookPreparationCodeSourcePinMismatch, byID[newest].Code)
		assert.Equal(t, domain.BookPreparationStageNone, byID[older].Stage)
		assert.Equal(t, domain.BookPreparationCodeNone, byID[older].Code)
	})

	t.Run("a completed latest attempt keeps the record visible exactly once", func(t *testing.T) {
		rows := preparationRowsFor(t, jobs, second.BookID)
		require.Len(t, rows, 2, "the completed attempt plus the record's own failed row: %+v", rows)
		stating := 0
		var failedRow domain.BookPreparationJob
		for _, row := range rows {
			if row.Stage != domain.BookPreparationStageNone {
				stating++
				failedRow = row
			}
		}
		assert.Equal(t, 1, stating, "the record is stated exactly once: %+v", rows)
		assert.Equal(t, int64(0), failedRow.JobID, "the record's own row has no job identity")
		assert.Equal(t, domain.JobLifecycleFailed, failedRow.Lifecycle)
		assert.Equal(t, domain.BookPreparationStageFetch, failedRow.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, failedRow.Code)
	})

	t.Run("My Books reads both Books the same way", func(t *testing.T) {
		first, err := f.store.GetBookDetail(ctx, f.owner, f.classicalBookID)
		require.NoError(t, err)
		require.NotNil(t, first.ClassicalPreparation)
		assert.Equal(t, domain.BookPreparationStageSource, first.ClassicalPreparation.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourcePinMismatch, first.ClassicalPreparation.Code)

		secondRead, err := f.store.GetBookDetail(ctx, f.owner, second.BookID)
		require.NoError(t, err)
		require.NotNil(t, secondRead.ClassicalPreparation)
		assert.Equal(t, domain.BookPreparationStageFetch, secondRead.ClassicalPreparation.Stage)
		assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, secondRead.ClassicalPreparation.Code)
	})
}

// TestListBookPreparationJobsReportsMyBooksMembershipOnEveryKind proves the
// row's link predicate on all three kinds: a current member reports true, and
// a Book removed from My Books keeps its history rows while reporting false,
// exactly as the My Books route reports the Book missing.
func TestListBookPreparationJobsReportsMyBooksMembershipOnEveryKind(t *testing.T) {
	ctx := context.Background()
	f := newBookPreparationFailureFixture(t)

	classicalJobID := insertPreparationRiverJob(t, ctx, f.pool, "classical_preparation", f.owner, f.classicalBookID,
		map[string]any{"reading_unit_key": f.readingUnitKey, "definition_revision": f.revision},
		"discarded", time.Date(2026, time.November, 1, 10, 0, 0, 0, time.UTC))
	acquisitionJobID := insertPreparationRiverJob(t, ctx, f.pool, "epub_acquisition", f.owner, f.epubBookID,
		nil, "discarded", time.Date(2026, time.November, 2, 10, 0, 0, 0, time.UTC))
	analysis := insertFailedAnalysisJob(t, ctx, f.pool, f.store, f.owner, "Membership analysis book", 42, time.Date(2026, time.November, 3, 10, 0, 0, 0, time.UTC))

	require.NoError(t, f.store.RemoveBookFromMyBooks(ctx, f.owner, f.epubBookID))
	require.NoError(t, f.store.RemoveBookFromMyBooks(ctx, f.owner, analysis.bookID))

	jobs, err := f.store.ListBookPreparationJobs(ctx, f.owner)
	require.NoError(t, err)
	require.Len(t, jobs, 3, "one row per Book preparation kind: %+v", jobs)
	byKind := make(map[domain.BookPreparationKind]domain.BookPreparationJob, len(jobs))
	for _, job := range jobs {
		byKind[job.Kind] = job
	}

	t.Run("a current member reports true", func(t *testing.T) {
		row := byKind[domain.BookPreparationKindClassical]
		require.Equal(t, classicalJobID, row.JobID)
		assert.Equal(t, f.classicalBookID, row.BookID)
		assert.True(t, row.BookInMyBooks)
	})

	t.Run("a Book removed from My Books keeps its rows but reports false", func(t *testing.T) {
		acquisition := byKind[domain.BookPreparationKindEPUBAcquisition]
		require.Equal(t, acquisitionJobID, acquisition.JobID)
		assert.Equal(t, f.epubBookID, acquisition.BookID)
		assert.False(t, acquisition.BookInMyBooks)

		analysisRow := byKind[domain.BookPreparationKindAnalysis]
		require.Equal(t, analysis.jobID, analysisRow.JobID)
		assert.Equal(t, analysis.bookID, analysisRow.BookID)
		assert.False(t, analysisRow.BookInMyBooks)
	})

	t.Run("the My Books route agrees with every flag", func(t *testing.T) {
		_, err := f.store.GetBookDetail(ctx, f.owner, f.classicalBookID)
		require.NoError(t, err)
		_, err = f.store.GetBookDetail(ctx, f.owner, f.epubBookID)
		require.ErrorIs(t, err, ErrNotFound)
		_, err = f.store.GetBookDetail(ctx, f.owner, analysis.bookID)
		require.ErrorIs(t, err, ErrNotFound)
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

// preparationRowsFor returns the merged list's rows for one Book.
func preparationRowsFor(t *testing.T, jobs []domain.BookPreparationJob, bookID string) []domain.BookPreparationJob {
	t.Helper()
	var rows []domain.BookPreparationJob
	for _, job := range jobs {
		if job.BookID == bookID {
			rows = append(rows, job)
		}
	}
	return rows
}

// insertFailedAnalysisJob seeds one failed analysis job with its Book and
// source, returning the identities the /jobs row must carry. The River job is
// seeded for real, so its id is unique across every seeded job.
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

	jobID := insertPreparationRiverJob(t, ctx, pool, "analysis", owner, book.ID,
		map[string]any{"source_material_id": source.ID}, "completed", createdAt)

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
	_, err = pool.Exec(ctx, `
		INSERT INTO analysis_jobs(river_job_id, owner_id, source_material_id, content_hash, progress, display_number, analysis_run_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		jobID, owner, source.ID, "sha256:preparation-list-"+book.ID, progress, displayNumber, runID, createdAt)
	require.NoError(t, err)
	return struct {
		jobID, displayNumber int64
		bookID, sourceID     string
	}{jobID, displayNumber, book.ID, source.ID}
}
