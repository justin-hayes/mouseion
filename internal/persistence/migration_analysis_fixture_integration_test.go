//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// insertMigrationAnalysisFixture seeds a legacy Book with a completed analysis,
// corpus, and published current analysis directly against the schema, for the
// migration tests that start from an old shape.
func insertMigrationAnalysisFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *PostgresStore, owner, language, suffix string) (bookID, sourceID, runID, corpusID string) {
	t.Helper()
	book := insertLegacyBook(t, ctx, store, owner, "Migration "+suffix, language)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner, Language: language, SourceIdentifier: "migration-" + suffix, Title: "Migration " + suffix, MediaType: "text/plain", ContentHash: "legacy:" + suffix, Content: []byte(suffix), FullText: suffix})
	require.NoError(t, err)
	err = store.LinkSourceToBook(ctx, owner, book.ID, source.ID)
	require.NoError(t, err)
	revisionID := uuid.NewString()
	snapshotID := uuid.NewString()
	_, err = pool.Exec(ctx, `INSERT INTO source_content_revisions(revision_id, owner_id, source_material_id, digest_version, content_digest, content, full_text) VALUES ($1,$2,$3,0,$4,$5,$6)`, revisionID, owner, source.ID, "legacy:"+suffix, []byte(suffix), suffix)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO source_material_unit_snapshots(owner_id, source_material_id, schema_version, snapshot_id, content_revision_id) VALUES ($1,$2,1,$3,$4)`, owner, source.ID, snapshotID, revisionID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE source_materials SET current_content_revision_id=$1, current_snapshot_id=$2 WHERE owner_id=$3 AND id=$4`, revisionID, snapshotID, owner, source.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO analysis_runs(owner_id, source_material_id, content_revision_id, snapshot_id, analyzer_name, analyzer_version, config_identity, state, completed_at) VALUES ($1,$2,$3,$4,'migration-test','1',$5,'completed',now())`, owner, source.ID, revisionID, snapshotID, suffix)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT id::text FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, owner, source.ID).Scan(&runID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash, language, schema_version, normalization_profile, normalization_version, analyzer_name, analyzer_version) VALUES ($1,$2,'1',$2,'1','migration-test','1')`, "corpus-"+suffix, language)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO corpora(owner_id, source_material_id, artifact_hash, status, analysis_run_id) VALUES ($1,$2,$3,'complete',$4)`, owner, source.ID, "corpus-"+suffix, runID)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, owner, source.ID).Scan(&corpusID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpusID, owner, runID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO book_current_analyses(owner_id, book_id, source_material_id, analysis_run_id) VALUES ($1,$2,$3,$4)`, owner, book.ID, source.ID, runID)
	require.NoError(t, err)
	return book.ID, source.ID, runID, corpusID
}
