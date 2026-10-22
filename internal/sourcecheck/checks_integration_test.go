//go:build integration

package sourcecheck

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openStore(t *testing.T, ctx context.Context) (*persistence.PostgresStore, *pgxpool.Pool) {
	t.Helper()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "sourcecheck store", store.Close)
	return store, pool
}

func newOwner(t *testing.T, ctx context.Context, store *persistence.PostgresStore, name string) domain.User {
	t.Helper()
	owner, err := store.CreateUser(ctx, name, false)
	require.NoError(t, err)
	return owner
}

func checkTarget() Target {
	return Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet}
}

func putEPUBSource(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, identifier string, data []byte, mediaType string, content []byte) (domain.SourceMaterial, domain.ExtractedSnapshot) {
	t.Helper()
	extraction, err := epub.ExtractV1(data)
	require.NoError(t, err)
	book := extraction.Book
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: ownerID, Language: "de", SourceIdentifier: identifier, Title: book.Title,
		MediaType: mediaType, Content: content, ContentHash: "legacy:" + identifier, FullText: book.FullText,
	}, book.ExtractedUnits)
	require.NoError(t, err)
	bookID, err := store.ResolveOrCreateBookForAcquisition(ctx, ownerID, book.SourceIdentifier, "de", book.Title)
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, ownerID, bookID, source.ID))
	snapshot, err := store.GetCurrentExtractedSnapshot(ctx, ownerID, source.ID)
	require.NoError(t, err)
	return source, snapshot
}

func putV2Source(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, identifier string, data []byte) (domain.SourceMaterial, domain.ExtractedSnapshot) {
	t.Helper()
	extraction, err := epub.ExtractV2(data)
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedSnapshot(ctx, domain.SourceMaterial{
		OwnerID: ownerID, Language: "de", SourceIdentifier: identifier, Title: extraction.Book.Title,
		MediaType: epub.MediaType(), Content: data, FullText: extraction.Book.FullText,
	}, extraction.Book.ExtractedUnits, extraction.Structure)
	require.NoError(t, err)
	bookID, err := store.ResolveOrCreateBookForAcquisition(ctx, ownerID, extraction.Book.SourceIdentifier, "de", extraction.Book.Title)
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, ownerID, bookID, source.ID))
	snapshot, err := store.GetCurrentExtractedSnapshot(ctx, ownerID, source.ID)
	require.NoError(t, err)
	return source, snapshot
}

func argsFor(source domain.SourceMaterial, snapshot domain.ExtractedSnapshot) CheckArgs {
	return CheckArgs{
		OwnerID: source.OwnerID, SourceMaterialID: source.ID,
		ContentRevisionID: snapshot.ContentRevisionID, SnapshotID: snapshot.SnapshotID,
		TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet,
	}
}

func runCheck(t *testing.T, ctx context.Context, worker *Worker, args CheckArgs) error {
	t.Helper()
	return worker.Work(ctx, &river.Job[CheckArgs]{Args: args})
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx, query, args...).Scan(&count))
	return count
}

func countJobs(t *testing.T, ctx context.Context, pool *pgxpool.Pool, liveOnly bool) int {
	t.Helper()
	if liveOnly {
		return countRows(t, ctx, pool, `SELECT count(*) FROM river_job WHERE kind=$1 AND state IN ('available','pending','running','retryable','scheduled')`, (CheckArgs{}).Kind())
	}
	return countRows(t, ctx, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, (CheckArgs{}).Kind())
}

func newCheckClient(t *testing.T, pool *pgxpool.Pool) JobInserter {
	t.Helper()
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	return client
}

func minimalEPUB(t *testing.T, body string) []byte {
	t.Helper()
	const container = `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path="OPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	const opf = `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier>fixture</dc:identifier><dc:title>Fixture</dc:title></metadata><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter"/></spine></package>`
	files := []struct{ name, content string }{
		{"mimetype", epub.MediaType()},
		{"META-INF/container.xml", container},
		{"OPS/content.opf", opf},
		{"OPS/chapter.xhtml", `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Chapter</title></head><body>` + body + `</body></html>`},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, file := range files {
		writer, err := zw.Create(file.name)
		require.NoError(t, err)
		_, err = writer.Write([]byte(file.content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestSourceStructureCheckUnchangedProseIsNoOp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-unchanged")
	data := minimalEPUB(t, `<p>Ein ruhiger Prosasatz.</p>`)
	source, snapshot := putEPUBSource(t, ctx, store, owner.ID, "unchanged-prose", data, epub.MediaType(), data)
	beforeSnapshots := countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)
	beforeRuns := countRows(t, ctx, pool, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)

	args := argsFor(source, snapshot)
	require.NoError(t, runCheck(t, ctx, &Worker{Store: store}, args))

	check, found, err := store.GetSourceStructureCheck(ctx, args.Key())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, domain.SourceStructureUnchanged, check.Outcome)
	assert.NotEmpty(t, check.ResultIdentity)
	assert.Equal(t, beforeSnapshots, countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID), "a check stores no candidate snapshot")
	assert.Equal(t, beforeRuns, countRows(t, ctx, pool, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID), "a check never runs or rewrites analysis")
	current, err := store.GetCurrentExtractedSnapshot(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, snapshot.SnapshotID, current.SnapshotID)
}

func TestSourceStructureCheckDetectsChangedV1Text(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-changed")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, snapshot := putEPUBSource(t, ctx, store, owner.ID, "changed-faust", data, epub.MediaType(), data)
	beforeSnapshots := countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)

	args := argsFor(source, snapshot)
	require.NoError(t, runCheck(t, ctx, &Worker{Store: store}, args))
	check, found, err := store.GetSourceStructureCheck(ctx, args.Key())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, domain.SourceStructureUpdateAvailable, check.Outcome)
	assert.NotEmpty(t, check.ResultIdentity)
	assert.Equal(t, beforeSnapshots, countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID))
}

func TestSourceStructureCheckMissingBytesIsOriginalUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-missing")
	data := minimalEPUB(t, `<p>Ein Satz.</p>`)
	source, snapshot := putEPUBSource(t, ctx, store, owner.ID, "missing-bytes", data, epub.MediaType(), nil)
	beforeSnapshots := countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)

	args := argsFor(source, snapshot)
	require.NoError(t, runCheck(t, ctx, &Worker{Store: store}, args))
	check, found, err := store.GetSourceStructureCheck(ctx, args.Key())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, domain.SourceStructureOriginalUnavailable, check.Outcome)
	assert.NotEmpty(t, check.ResultIdentity)
	assert.Equal(t, beforeSnapshots, countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID))
	stored, err := store.GetSourceMaterial(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Empty(t, stored.Content, "a missing original is never downloaded or repaired by a check")
}

func TestSourceStructureCheckExcludesImportedAnalysis(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-imported")
	data := minimalEPUB(t, `<p>Ein Satz.</p>`)
	source, snapshot := putEPUBSource(t, ctx, store, owner.ID, "imported-source", data, domain.ImportedSourceMediaType, nil)

	service := NewService(store, newCheckClient(t, pool), checkTarget())
	require.NoError(t, service.EnqueueStartupChecks(ctx))
	require.NoError(t, service.EnqueueSourceStructureCheck(ctx, owner.ID, source.ID))
	assert.Zero(t, countJobs(t, ctx, pool, false), "imported analyses are excluded from source structure checks")

	args := argsFor(source, snapshot)
	require.NoError(t, runCheck(t, ctx, &Worker{Store: store}, args))
	_, found, err := store.GetSourceStructureCheck(ctx, args.Key())
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSourceStructureCheckRoleOnlyChangeIsUpdateAvailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, _ := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-role-only")
	data := minimalEPUB(t, `<p><span class="speaker">Sprecher.</span> Es ist ein Text.</p>`)
	v1, err := epub.ExtractV1(data)
	require.NoError(t, err)
	v2, err := epub.ExtractV2(data)
	require.NoError(t, err)
	require.Equal(t, v1.Book.FullText, v2.Book.FullText, "the fixture must keep text identical")
	require.NotEmpty(t, v2.Structure.RoleSpans, "the fixture must record a role")

	source, snapshot := putEPUBSource(t, ctx, store, owner.ID, "role-only", data, epub.MediaType(), data)
	args := argsFor(source, snapshot)
	require.NoError(t, runCheck(t, ctx, &Worker{Store: store}, args))
	check, found, err := store.GetSourceStructureCheck(ctx, args.Key())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, domain.SourceStructureUpdateAvailable, check.Outcome, "a role-only convention change offers an update")
}

type failingRecordStore struct {
	Store
	fail bool
}

func (s *failingRecordStore) RecordSourceStructureCheck(ctx context.Context, check domain.SourceStructureCheck) error {
	if s.fail {
		s.fail = false
		return errors.New("simulated crash before commit")
	}
	return s.Store.RecordSourceStructureCheck(ctx, check)
}

func TestSourceStructureCheckFailureRetryAndExactKeyReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-retry")
	data := minimalEPUB(t, `<p>Ein ruhiger Prosasatz.</p>`)
	source, snapshot := putEPUBSource(t, ctx, store, owner.ID, "retry-prose", data, epub.MediaType(), data)
	beforeSnapshots := countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)
	args := argsFor(source, snapshot)
	service := NewService(store, newCheckClient(t, pool), checkTarget())

	require.NoError(t, service.EnqueueSourceStructureCheck(ctx, owner.ID, source.ID))
	assert.Equal(t, 1, countJobs(t, ctx, pool, true))

	// A failure before the record commits leaves no success row and no snapshot.
	worker := &Worker{Store: &failingRecordStore{Store: store, fail: true}}
	require.ErrorContains(t, runCheck(t, ctx, worker, args), "simulated crash")
	_, found, err := store.GetSourceStructureCheck(ctx, args.Key())
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, beforeSnapshots, countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID))

	// River exhausts and discards the failed job; an explicit retry can enqueue it
	// again because uniqueness is bound to live states only.
	_, err = pool.Exec(ctx, `UPDATE river_job SET state='discarded',finalized_at=now() WHERE kind=$1`, (CheckArgs{}).Kind())
	require.NoError(t, err)
	require.NoError(t, service.EnqueueSourceStructureCheck(ctx, owner.ID, source.ID))
	assert.Equal(t, 1, countJobs(t, ctx, pool, true))
	assert.Equal(t, 2, countJobs(t, ctx, pool, false))

	require.NoError(t, runCheck(t, ctx, &Worker{Store: store}, args))
	check, found, err := store.GetSourceStructureCheck(ctx, args.Key())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, domain.SourceStructureUnchanged, check.Outcome)
	assert.Equal(t, beforeSnapshots, countRows(t, ctx, pool, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID))

	// A successful exact key is reused: evidence changes and startup scans do
	// not enqueue it, and a stray job does not extract again.
	require.NoError(t, service.EnqueueSourceStructureCheck(ctx, owner.ID, source.ID))
	require.NoError(t, service.EnqueueStartupChecks(ctx))
	assert.Equal(t, 2, countJobs(t, ctx, pool, false))
	extractions := 0
	reusingWorker := &Worker{Store: store, Extract: func(data []byte) (epub.Extraction, error) {
		extractions++
		return epub.ExtractV2(data)
	}}
	require.NoError(t, runCheck(t, ctx, reusingWorker, args))
	assert.Zero(t, extractions)
}

func TestSourceStructureStartupScanSkipsCurrentTargetAndRepeats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-scan")
	data := minimalEPUB(t, `<p>Ein ruhiger Prosasatz.</p>`)
	putEPUBSource(t, ctx, store, owner.ID, "scan-v1-a", data, epub.MediaType(), data)
	putEPUBSource(t, ctx, store, owner.ID, "scan-v1-b", data, epub.MediaType(), data)
	putV2Source(t, ctx, store, owner.ID, "scan-v2", data)

	service := NewService(store, newCheckClient(t, pool), checkTarget())
	service.pageSize = 1
	require.NoError(t, service.EnqueueStartupChecks(ctx))
	assert.Equal(t, 2, countJobs(t, ctx, pool, true), "only sources below the target contract are enqueued")
	require.NoError(t, service.EnqueueStartupChecks(ctx))
	assert.Equal(t, 2, countJobs(t, ctx, pool, true), "live exact keys are not re-enqueued")
}

func TestSourceStructureCheckRejectsMismatchedExtractor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, _ := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-mismatch")
	data := minimalEPUB(t, `<p>Ein Satz.</p>`)
	source, snapshot := putEPUBSource(t, ctx, store, owner.ID, "mismatch", data, epub.MediaType(), data)
	args := argsFor(source, snapshot)
	args.ConventionSet = "epub-structure/other"
	worker := &Worker{Store: store}
	require.ErrorContains(t, runCheck(t, ctx, worker, args), "unsupported structure convention set")

	args = argsFor(source, snapshot)
	args.TargetContractVersion = domain.ExtractedUnitsSchemaVersionV2
	worker = &Worker{Store: store, Extract: func([]byte) (epub.Extraction, error) {
		return epub.Extraction{Book: epub.ExtractedBook{ExtractedUnits: domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1}}}, nil
	}}
	require.ErrorContains(t, runCheck(t, ctx, worker, args), "extractor produced contract")
}

func TestSourceStructureCheckObsoleteEvidenceIsNotRecorded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, _ := openStore(t, ctx)
	owner := newOwner(t, ctx, store, "structure-obsolete")
	data := minimalEPUB(t, `<p>Ein Satz.</p>`)
	source, snapshot := putEPUBSource(t, ctx, store, owner.ID, "obsolete", data, epub.MediaType(), data)
	args := argsFor(source, snapshot)
	args.SnapshotID = "00000000-0000-0000-0000-000000000000"
	require.NoError(t, runCheck(t, ctx, &Worker{Store: store}, args))
	_, found, err := store.GetSourceStructureCheck(ctx, args.Key())
	require.NoError(t, err)
	assert.False(t, found, "a job for an obsolete snapshot records nothing")
}

func TestSourceStructureServiceRequiresConfiguration(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, (*Service)(nil).EnqueueStartupChecks(ctx))
	require.NoError(t, (*Service)(nil).EnqueueSourceStructureCheck(ctx, "owner", "source"))
	inactive := NewService(nil, nil, Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV1})
	require.NoError(t, inactive.EnqueueStartupChecks(ctx))
	require.NoError(t, inactive.EnqueueSourceStructureCheck(ctx, "owner", "source"))
	active := NewService(nil, nil, checkTarget())
	require.ErrorContains(t, active.EnqueueStartupChecks(ctx), "not configured")
	require.ErrorContains(t, active.EnqueueSourceStructureCheck(ctx, "owner", "source"), "not configured")
}

func TestSourceStructureCheckCandidatesRejectInvalidLimitsAndPropagateFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, pool := openStore(t, ctx)
	zero := "00000000-0000-0000-0000-000000000000"
	_, err := store.ListSourceStructureCheckCandidates(ctx, domain.ExtractedUnitsSchemaVersionV2, domain.StructureConventionSet, zero, 0)
	require.ErrorContains(t, err, "limit must be positive")
	_, err = store.ListSourceStructureCheckCandidates(ctx, domain.ExtractedUnitsSchemaVersionV2, domain.StructureConventionSet, zero, math.MaxInt32+1)
	require.ErrorContains(t, err, "integer conversion overflow")

	_, err = pool.Exec(ctx, `DROP TABLE source_structure_checks CASCADE`)
	require.NoError(t, err)
	_, err = store.ListSourceStructureCheckCandidates(ctx, domain.ExtractedUnitsSchemaVersionV2, domain.StructureConventionSet, zero, 1)
	require.Error(t, err)
}
