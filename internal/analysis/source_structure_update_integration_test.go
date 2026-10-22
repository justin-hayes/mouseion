//go:build integration

package analysis

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/sourcecheck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type structureAdoptionHarness struct {
	store   *persistence.PostgresStore
	pool    *pgxpool.Pool
	service *Service
	owner   domain.User
}

func newStructureAdoptionHarness(t *testing.T, ctx context.Context, name string) structureAdoptionHarness {
	t.Helper()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "structure adoption store", store.Close)
	owner, err := store.CreateUser(ctx, name, false)
	require.NoError(t, err)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	return structureAdoptionHarness{store: store, pool: pool, service: NewService(pool, client), owner: owner}
}

func (h structureAdoptionHarness) putV1BookSource(t *testing.T, ctx context.Context, identifier string, data, retained []byte) (domain.SourceMaterial, domain.Book) {
	t.Helper()
	extraction, err := epub.ExtractV1(data)
	require.NoError(t, err)
	book := extraction.Book
	material := domain.SourceMaterial{
		OwnerID: h.owner.ID, Language: "de", SourceIdentifier: identifier, Title: book.Title,
		MediaType: epub.MediaType(), Content: retained, FullText: book.FullText,
	}
	if retained == nil {
		material.ContentHash = "legacy:" + identifier
	}
	source, err := h.store.PutSourceMaterialWithExtractedUnits(ctx, material, book.ExtractedUnits)
	require.NoError(t, err)
	bookID, err := h.store.ResolveOrCreateBookForAcquisition(ctx, h.owner.ID, identifier, "de", book.Title)
	require.NoError(t, err)
	require.NoError(t, h.store.LinkSourceToBook(ctx, h.owner.ID, bookID, source.ID))
	domainBook, err := h.store.GetBook(ctx, h.owner.ID, bookID)
	require.NoError(t, err)
	return source, domainBook
}

func (h structureAdoptionHarness) check(t *testing.T, ctx context.Context, source domain.SourceMaterial) sourcecheck.CheckArgs {
	t.Helper()
	snapshot, err := h.store.GetCurrentExtractedSnapshot(ctx, h.owner.ID, source.ID)
	require.NoError(t, err)
	args := sourcecheck.CheckArgs{
		OwnerID: h.owner.ID, SourceMaterialID: source.ID,
		ContentRevisionID: source.ContentRevisionID, SnapshotID: snapshot.SnapshotID,
		TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet,
	}
	require.NoError(t, (&sourcecheck.Worker{Store: h.store}).Work(ctx, &river.Job[sourcecheck.CheckArgs]{Args: args}))
	return args
}

func (h structureAdoptionHarness) candidate(t *testing.T, ctx context.Context, source domain.SourceMaterial) domain.SourceStructureUpdateCandidate {
	t.Helper()
	candidates, err := h.service.SourceStructureUpdateCandidates(ctx, h.owner.ID, []string{source.ID})
	require.NoError(t, err)
	candidate, ok := candidates[source.ID]
	require.True(t, ok, "the checked source has no structure update offer")
	return candidate
}

func structureSnapshotCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, sourceID string) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, owner, sourceID).Scan(&count))
	return count
}

func structureRunCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, sourceID string) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, owner, sourceID).Scan(&count))
	return count
}

func currentSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, sourceID string) string {
	t.Helper()
	var snapshotID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT COALESCE(current_snapshot_id::text,'') FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, sourceID).Scan(&snapshotID))
	return snapshotID
}

func TestSourceStructureUpdateCandidateFollowsCurrentCheckedEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-candidate")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, _ := h.putV1BookSource(t, ctx, "candidate-faust", data, data)

	candidates, err := h.service.SourceStructureUpdateCandidates(ctx, h.owner.ID, []string{source.ID})
	require.NoError(t, err)
	assert.Empty(t, candidates, "a check is required before an offer exists")

	h.check(t, ctx, source)
	candidate := h.candidate(t, ctx, source)
	assert.Equal(t, source.ContentRevisionID, candidate.ContentRevisionID)
	assert.Equal(t, domain.ExtractedUnitsSchemaVersionV2, candidate.TargetContractVersion)
	assert.Equal(t, domain.StructureConventionSet, candidate.ConventionSet)
	assert.NotEmpty(t, candidate.ResultIdentity)
}

func TestSourceStructureUpdateAdoptionSwitchesSnapshotAndConvergesOnOneRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-adopt")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, book := h.putV1BookSource(t, ctx, "adopt-faust", data, data)
	check := h.check(t, ctx, source)
	candidate := h.candidate(t, ctx, source)
	assert.Equal(t, 1, structureSnapshotCount(t, ctx, h.pool, h.owner.ID, source.ID))

	result, err := h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, candidate.Expectation())
	require.NoError(t, err)
	require.NotEmpty(t, result.RunID)

	adopted := currentSnapshot(t, ctx, h.pool, h.owner.ID, source.ID)
	require.NotEqual(t, check.SnapshotID, adopted)
	assert.Equal(t, 2, structureSnapshotCount(t, ctx, h.pool, h.owner.ID, source.ID), "adoption creates one immutable v2 snapshot")
	snapshot, err := h.store.GetExtractedSnapshotByID(ctx, h.owner.ID, source.ID, adopted)
	require.NoError(t, err)
	assert.Equal(t, domain.ExtractedUnitsSchemaVersionV2, snapshot.Units.SchemaVersion)
	assert.Equal(t, domain.StructureConventionSet, snapshot.Structure.ConventionSet)
	assert.True(t, snapshot.Structure.Recorded)
	assert.Equal(t, source.ContentRevisionID, snapshot.ContentRevisionID, "adoption keeps the same content revision")

	assert.Equal(t, 1, structureRunCount(t, ctx, h.pool, h.owner.ID, source.ID))
	var runSnapshot, state string
	require.NoError(t, h.pool.QueryRow(ctx, `SELECT snapshot_id::text,state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, h.owner.ID, result.RunID).Scan(&runSnapshot, &state))
	assert.Equal(t, adopted, runSnapshot, "the queued run is bound to the adopted snapshot")
	assert.Equal(t, "queued", state)

	candidates, err := h.service.SourceStructureUpdateCandidates(ctx, h.owner.ID, []string{source.ID})
	require.NoError(t, err)
	assert.Empty(t, candidates, "the old offer no longer names the current snapshot")

	again, err := h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, candidate.Expectation())
	require.NoError(t, err, "a retry after the first commit converges on the same checked extraction")
	assert.Equal(t, result.RunID, again.RunID)
	assert.Equal(t, 2, structureSnapshotCount(t, ctx, h.pool, h.owner.ID, source.ID))
	assert.Equal(t, 1, structureRunCount(t, ctx, h.pool, h.owner.ID, source.ID))
}

func TestSourceStructureUpdateAdoptionRejectsConcurrentSourceChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-race")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, book := h.putV1BookSource(t, ctx, "race-faust", data, data)
	h.check(t, ctx, source)
	candidate := h.candidate(t, ctx, source)

	replacement := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "epub3-reimport"))
	extraction, err := epub.ExtractV1(replacement)
	require.NoError(t, err)
	_, err = h.store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: h.owner.ID, Language: "de", SourceIdentifier: source.SourceIdentifier, Title: extraction.Book.Title,
		MediaType: epub.MediaType(), Content: replacement, FullText: extraction.Book.FullText,
	}, extraction.Book.ExtractedUnits)
	require.NoError(t, err)
	before := currentSnapshot(t, ctx, h.pool, h.owner.ID, source.ID)
	snapshots := structureSnapshotCount(t, ctx, h.pool, h.owner.ID, source.ID)

	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, candidate.Expectation())
	require.ErrorIs(t, err, ErrSourceStructureUpdateChanged)
	assert.Equal(t, before, currentSnapshot(t, ctx, h.pool, h.owner.ID, source.ID), "a changed source is never adopted")
	assert.Equal(t, snapshots, structureSnapshotCount(t, ctx, h.pool, h.owner.ID, source.ID))
	assert.Zero(t, structureRunCount(t, ctx, h.pool, h.owner.ID, source.ID))
}

type failingStructureClient struct {
	riverClient
}

func (failingStructureClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, errors.New("simulated crash before commit")
}

func TestSourceStructureUpdateAdoptionRollsBackBeforeCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-rollback")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, book := h.putV1BookSource(t, ctx, "rollback-faust", data, data)
	check := h.check(t, ctx, source)
	candidate := h.candidate(t, ctx, source)
	h.service = NewService(h.pool, failingStructureClient{riverClient: h.service.client})

	_, err := h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, candidate.Expectation())
	require.ErrorContains(t, err, "simulated crash")
	assert.Equal(t, check.SnapshotID, currentSnapshot(t, ctx, h.pool, h.owner.ID, source.ID), "a failed enqueue leaves the old snapshot current")
	assert.Equal(t, 1, structureSnapshotCount(t, ctx, h.pool, h.owner.ID, source.ID), "a failed enqueue leaves no partial snapshot")
	assert.Zero(t, structureRunCount(t, ctx, h.pool, h.owner.ID, source.ID))
	h.candidate(t, ctx, source)
}

func TestSourceStructureUpdateAdoptionRoleOnlyChangeKeepsRevision(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-role-only")
	data := structureRoleOnlyEPUB(t, `<p><span class="speaker">Sprecher.</span> Es ist ein Text.</p>`)
	v1, err := epub.ExtractV1(data)
	require.NoError(t, err)
	v2, err := epub.ExtractV2(data)
	require.NoError(t, err)
	require.Equal(t, v1.Book.FullText, v2.Book.FullText)
	require.NotEmpty(t, v2.Structure.RoleSpans)
	source, book := h.putV1BookSource(t, ctx, "role-only", data, data)
	h.check(t, ctx, source)
	candidate := h.candidate(t, ctx, source)

	result, err := h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, candidate.Expectation())
	require.NoError(t, err)
	assert.Equal(t, 1, structureRunCount(t, ctx, h.pool, h.owner.ID, source.ID))
	snapshot, err := h.store.GetExtractedSnapshotByID(ctx, h.owner.ID, source.ID, currentSnapshot(t, ctx, h.pool, h.owner.ID, source.ID))
	require.NoError(t, err)
	assert.Equal(t, source.ContentRevisionID, snapshot.ContentRevisionID)
	assert.Equal(t, v2.Structure.RoleSpans, snapshot.Structure.RoleSpans)
	assert.Equal(t, v2.Structure.HeadingResets, snapshot.Structure.HeadingResets)
	assert.Equal(t, v2.Book.FullText, snapshot.Units.ReconstructText())
	_ = result
}

func TestSourceStructureUpdateMissingBytesAndImportedSourcesOfferNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-unavailable")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))

	missing, missingBook := h.putV1BookSource(t, ctx, "missing-bytes", data, nil)
	h.check(t, ctx, missing)
	candidates, err := h.service.SourceStructureUpdateCandidates(ctx, h.owner.ID, []string{missing.ID})
	require.NoError(t, err)
	assert.Empty(t, candidates, "missing retained bytes offer no adoption")
	snapshot := currentSnapshot(t, ctx, h.pool, h.owner.ID, missing.ID)
	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, missingBook.ID, missing.ID, domain.SourceStructureUpdateExpectation{
		ContentRevisionID: missing.ContentRevisionID, SnapshotID: snapshot,
		TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet,
		ResultIdentity: sourcecheck.UnavailableIdentity(domain.SourceStructureCheckKey{
			OwnerID: h.owner.ID, SourceMaterialID: missing.ID, ContentRevisionID: missing.ContentRevisionID,
			SnapshotID: snapshot, TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2,
			ConventionSet: domain.StructureConventionSet,
		}, missing.ContentDigest),
	})
	require.ErrorIs(t, err, ErrSourceStructureUpdateChanged)
	assert.Equal(t, 1, structureSnapshotCount(t, ctx, h.pool, h.owner.ID, missing.ID))
	assert.Zero(t, structureRunCount(t, ctx, h.pool, h.owner.ID, missing.ID))

	imported, err := h.store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: h.owner.ID, Language: "de", SourceIdentifier: "imported-structure", Title: "Imported",
		MediaType: domain.ImportedSourceMediaType, Content: data, FullText: "text",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: []domain.ExtractedUnit{{
		ID: domain.ClassicalUnitID("imported-unit"), ManifestID: "imported-unit", Text: "text", EndOffset: 4,
	}}})
	require.NoError(t, err)
	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, "", imported.ID, domain.SourceStructureUpdateExpectation{
		ContentRevisionID: imported.ContentRevisionID, SnapshotID: "00000000-0000-0000-0000-000000000000",
		TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet,
	})
	require.ErrorIs(t, err, ErrEPUBRequired)
}

func TestSourceStructureUpdateAdoptionPinsActiveReadingEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-frozen-reading")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, book := h.putV1BookSource(t, ctx, "frozen-faust", data, data)
	h.check(t, ctx, source)
	candidate := h.candidate(t, ctx, source)

	var runID, corpusID, goalSnapshotID string
	artifact := "artifact-frozen-" + source.ID
	_, err := h.pool.Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version)
VALUES($1,'de','1','identity','1','fixture','1')`, artifact)
	require.NoError(t, err)
	require.NoError(t, h.pool.QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at)
VALUES($1,$2,$3,$4,'fixture','1','selection-default-v1','completed',now()) RETURNING id::text`, h.owner.ID, source.ID, source.ContentRevisionID, candidate.SnapshotID).Scan(&runID))
	require.NoError(t, h.pool.QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,status,analysis_run_id)
VALUES($1,$2,$3,'complete',$4) RETURNING id::text`, h.owner.ID, source.ID, artifact, runID).Scan(&corpusID))
	_, err = h.pool.Exec(ctx, `UPDATE analysis_runs SET corpus_id=$3 WHERE owner_id=$1 AND id=$2`, h.owner.ID, runID, corpusID)
	require.NoError(t, err)
	_, err = h.pool.Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, h.owner.ID, book.ID, source.ID, runID)
	require.NoError(t, err)
	require.NoError(t, h.pool.QueryRow(ctx, `INSERT INTO primary_goal_snapshots(owner_id,language,book_id,source_material_id,analysis_run_id,content_revision_id,content_snapshot_id,corpus_id)
VALUES($1,'de',$2,$3,$4,$5,$6,$7) RETURNING id::text`, h.owner.ID, book.ID, source.ID, runID, source.ContentRevisionID, candidate.SnapshotID, corpusID).Scan(&goalSnapshotID))
	_, err = h.pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id,snapshot_id) VALUES($1,'de',$2,$3)`, h.owner.ID, book.ID, goalSnapshotID)
	require.NoError(t, err)
	_, err = h.pool.Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version)
VALUES($1,$2,$3,$4,'unit',0,4,'wort','identity','1')`, h.owner.ID, book.ID, corpusID, runID)
	require.NoError(t, err)
	_, err = h.pool.Exec(ctx, `INSERT INTO primary_goal_snapshot_vocabulary(owner_id,snapshot_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance)
VALUES($1,$2,$3,'de','wort','NOUN',1,'[]'::jsonb,'[]'::jsonb,'{}'::jsonb)`, h.owner.ID, goalSnapshotID, corpusID)
	require.NoError(t, err)

	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, candidate.Expectation())
	require.NoError(t, err)

	var pinnedGoal, pinnedContent, currentAnalysis string
	require.NoError(t, h.pool.QueryRow(ctx, `SELECT snapshot_id::text FROM primary_goals WHERE owner_id=$1 AND language='de'`, h.owner.ID).Scan(&pinnedGoal))
	require.NoError(t, h.pool.QueryRow(ctx, `SELECT content_snapshot_id::text FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2`, h.owner.ID, goalSnapshotID).Scan(&pinnedContent))
	require.NoError(t, h.pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, h.owner.ID, book.ID).Scan(&currentAnalysis))
	assert.Equal(t, goalSnapshotID, pinnedGoal, "the active Reading keeps its reserved vocabulary snapshot")
	assert.Equal(t, candidate.SnapshotID, pinnedContent, "the active Reading keeps its exact v1 content snapshot")
	assert.Equal(t, runID, currentAnalysis, "adoption does not rewrite the published analysis before reanalysis")
	var correction string
	require.NoError(t, h.pool.QueryRow(ctx, `SELECT canonical_lemma FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, h.owner.ID, book.ID, runID).Scan(&correction))
	assert.Equal(t, "wort", correction, "confirmed corrections stay with the frozen analysis")
	var reserved int
	require.NoError(t, h.pool.QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, h.owner.ID, goalSnapshotID).Scan(&reserved))
	assert.Equal(t, 1, reserved, "the active Reading's Reserved vocabulary stays pinned")
	assert.Equal(t, 2, structureRunCount(t, ctx, h.pool, h.owner.ID, source.ID), "adoption queues a new snapshot-bound run")
	assert.NotEqual(t, candidate.SnapshotID, currentSnapshot(t, ctx, h.pool, h.owner.ID, source.ID))
}

func TestSourceStructureUpdateRejectsUncheckedOrMismatchedOffers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-guards")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, book := h.putV1BookSource(t, ctx, "guard-faust", data, data)
	h.check(t, ctx, source)
	candidate := h.candidate(t, ctx, source)

	_, err := (*Service)(nil).SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, candidate.Expectation())
	require.ErrorContains(t, err, "owner and source are required")
	_, err = h.service.SubmitSourceStructureUpdate(ctx, "", book.ID, source.ID, candidate.Expectation())
	require.ErrorContains(t, err, "owner and source are required")

	staleTarget := candidate.Expectation()
	staleTarget.TargetContractVersion = domain.ExtractedUnitsSchemaVersionV1
	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, staleTarget)
	require.ErrorIs(t, err, ErrSourceStructureUpdateChanged)

	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, "00000000-0000-0000-0000-000000000000", candidate.Expectation())
	require.ErrorIs(t, err, ErrNotFound)

	otherBook := candidate.Expectation()
	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, "00000000-0000-0000-0000-000000000000", source.ID, otherBook)
	require.ErrorIs(t, err, ErrNotFound)

	staleSnapshot := candidate.Expectation()
	staleSnapshot.SnapshotID = "00000000-0000-0000-0000-000000000000"
	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, staleSnapshot)
	require.ErrorIs(t, err, ErrSourceStructureUpdateChanged)

	wrongIdentity := candidate.Expectation()
	wrongIdentity.ResultIdentity = "sha256:unchecked"
	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, wrongIdentity)
	require.ErrorIs(t, err, ErrSourceStructureUpdateChanged)
	assert.Zero(t, structureRunCount(t, ctx, h.pool, h.owner.ID, source.ID))
}

func TestSourceStructureUpdateCandidatesGuardsAndFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-candidate-guards")

	candidates, err := (*Service)(nil).SourceStructureUpdateCandidates(ctx, h.owner.ID, []string{"00000000-0000-0000-0000-000000000000"})
	require.NoError(t, err)
	assert.Empty(t, candidates)
	candidates, err = h.service.SourceStructureUpdateCandidates(ctx, "", []string{"00000000-0000-0000-0000-000000000000"})
	require.NoError(t, err)
	assert.Empty(t, candidates)
	candidates, err = h.service.SourceStructureUpdateCandidates(ctx, h.owner.ID, []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000"})
	require.NoError(t, err)
	assert.Empty(t, candidates)

	ids := validSourceIDs([]string{" not-a-uuid ", "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000001"})
	assert.Equal(t, []string{"00000000-0000-0000-0000-000000000001"}, ids)

	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, _ := h.putV1BookSource(t, ctx, "candidate-failure", data, data)
	h.check(t, ctx, source)
	h.pool.Close()
	_, err = h.service.SourceStructureUpdateCandidates(ctx, h.owner.ID, []string{source.ID})
	require.ErrorContains(t, err, "load source structure update offers")
}

func TestSourceStructureUpdateCheckLookupAndExtractionFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newStructureAdoptionHarness(t, ctx, "structure-check-failures")
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	source, book := h.putV1BookSource(t, ctx, "check-failure-faust", data, data)
	h.check(t, ctx, source)
	candidate := h.candidate(t, ctx, source)

	// A source whose retained bytes are not an EPUB still fails extraction
	// before any snapshot or analysis write.
	broken, err := h.store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: h.owner.ID, Language: "de", SourceIdentifier: "broken-epub", Title: "Broken",
		MediaType: epub.MediaType(), Content: []byte("not an epub"), FullText: "text",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: []domain.ExtractedUnit{{
		ID: domain.EPUBUnitID(0, "broken"), ManifestID: "broken", Text: "text", EndOffset: 4,
	}}})
	require.NoError(t, err)
	snapshotID := currentSnapshot(t, ctx, h.pool, h.owner.ID, broken.ID)
	_, err = h.pool.Exec(ctx, `INSERT INTO source_structure_checks(owner_id,source_material_id,content_revision_id,snapshot_id,target_contract_version,structure_convention_set,outcome,result_identity)
VALUES($1,$2,$3,$4,$5,$6,'update_available','sha256:unchecked')`, h.owner.ID, broken.ID, broken.ContentRevisionID, snapshotID, domain.ExtractedUnitsSchemaVersionV2, domain.StructureConventionSet)
	require.NoError(t, err)
	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, "", broken.ID, domain.SourceStructureUpdateExpectation{
		ContentRevisionID: broken.ContentRevisionID, SnapshotID: snapshotID,
		TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet,
		ResultIdentity: "sha256:unchecked",
	})
	require.ErrorContains(t, err, "re-extract retained source")
	assert.Equal(t, 1, structureSnapshotCount(t, ctx, h.pool, h.owner.ID, broken.ID))
	assert.Zero(t, structureRunCount(t, ctx, h.pool, h.owner.ID, broken.ID))

	_, err = h.pool.Exec(ctx, `DROP TABLE source_structure_checks CASCADE`)
	require.NoError(t, err)
	_, err = h.service.SubmitSourceStructureUpdate(ctx, h.owner.ID, book.ID, source.ID, candidate.Expectation())
	require.ErrorContains(t, err, "load source structure check")
}

func structureRoleOnlyEPUB(t *testing.T, body string) []byte {
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
