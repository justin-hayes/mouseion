//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestLegacyFailureCleanupDropsOldTablesAndErrorText pins the cleanup migration
// (ADR 0121's final slice): the two old failure tables and the three analysis
// error-text columns no longer exist at the latest schema, catalogue sync keeps
// its own error text, and the evidence read models still classify analysis
// through the run instead of stored sentences.
func TestLegacyFailureCleanupDropsOldTablesAndErrorText(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "legacy-failure-cleanup", false)
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "de", "German")
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: "cleanup-1", Title: "Cleanup",
		MediaType: "application/epub+zip", ContentHash: "sha256:cleanup-1", Content: []byte("cleanup sentence"), FullText: "cleanup sentence",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: []domain.ExtractedUnit{{
		ID: domain.EPUBUnitID(0, "cleanup-1"), Order: 0, SpineIndex: 0, ManifestID: "cleanup-1",
		SourceHref: "cleanup-1", ResolvedHref: "cleanup-1", Text: "cleanup sentence",
		EndOffset: uint64(len([]rune("cleanup sentence"))), MediaType: "application/xhtml+xml", Linear: true,
	}}})
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT current_content_revision_id::text, current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`,
		owner.ID, source.ID).Scan(&source.ContentRevisionID, &source.ContentSnapshotID))

	for _, tc := range []struct {
		name  string
		table string
	}{
		{name: "the Classical failure table is gone", table: "classical_preparation_failures"},
		{name: "the EPUB failure table is gone", table: "epub_acquisition_failures"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var regclass string
			require.NoError(t, pool.QueryRow(ctx, `SELECT COALESCE(to_regclass('public.`+tc.table+`')::text, '')`).Scan(&regclass))
			require.Empty(t, regclass, tc.table+" still exists")
		})
	}
	t.Run("the analysis error-text columns are gone", func(t *testing.T) {
		var dropped int
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT count(*) FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND ((table_name = 'analysis_jobs' AND column_name = 'error')
			    OR (table_name = 'analysis_runs' AND column_name = 'last_error')
			    OR (table_name = 'analysis_run_attempts' AND column_name = 'error'))`).Scan(&dropped))
		require.Zero(t, dropped, "analysis failure text is a code, not a stored sentence (ADR 0121)")
	})
	t.Run("catalogue sync keeps its error text", func(t *testing.T) {
		var kept int
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT count(*) FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = 'catalogue_sync_status' AND column_name = 'last_error'`).Scan(&kept))
		require.Equal(t, 1, kept, "catalogue sync is not Book preparation and keeps its error column")
	})
	// The failed-run step links the job this sequence inserts first, so both
	// classifications stay in one dependent flow.
	t.Run("the evidence view classifies through the run, not stored text", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO analysis_jobs(river_job_id, owner_id, source_material_id, content_hash, display_number)
			VALUES(86001, $1, $2, $3, 1)`, owner.ID, source.ID, source.ContentHash)
		require.NoError(t, err)
		var status, state string
		require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_status, analysis_state FROM source_material_evidence WHERE source_owner_id = $1`, owner.ID).Scan(&status, &state))
		require.Equal(t, "analyzing", status)
		require.Equal(t, "queued", state, "a job without a run can no longer carry failure text")

		var runID string
		require.NoError(t, pool.QueryRow(ctx, `
			INSERT INTO analysis_runs(owner_id, source_material_id, content_revision_id, snapshot_id,
				analyzer_name, analyzer_version, config_identity, state, failure_code)
			VALUES($1, $2, $3, $4, 'test', '1', 'cleanup-identity', 'failed', 'analysis_failed')
			RETURNING id::text`, owner.ID, source.ID, source.ContentRevisionID, source.ContentSnapshotID).Scan(&runID))
		_, err = pool.Exec(ctx, `
			UPDATE analysis_jobs SET analysis_run_id = $3
			WHERE owner_id = $1 AND source_material_id = $2`, owner.ID, source.ID, runID)
		require.NoError(t, err)
		require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_status, analysis_state FROM source_material_evidence WHERE source_owner_id = $1`, owner.ID).Scan(&status, &state))
		require.Equal(t, "analysis failed", status)
		require.Equal(t, "failed", state, "failure classification reads the run, not a stored sentence")
	})
}
