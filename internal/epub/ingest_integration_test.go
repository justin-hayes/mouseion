//go:build integration

package epub

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportPostgresOwnerIsolationHistoryAndDeletion(t *testing.T) {
	ctx := context.Background()
	url, admin := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()
	alice, err := store.CreateUser(ctx, "epub-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "epub-bob", false)
	require.NoError(t, err)
	result, err := NewService(store).Import(ctx, alice.ID, "de", fixtureDirectory(t, "testfixtures/epub3-edge-cases"))
	require.NoError(t, err)
	got, err := store.GetSourceMaterial(ctx, alice.ID, result.Source.ID)
	require.NoError(t, err)
	assert.Equal(t, alice.ID, got.OwnerID)
	assert.Equal(t, result.Book.FullText, got.FullText)
	assert.NotEmpty(t, got.Content)
	units, err := store.GetExtractedUnits(ctx, alice.ID, result.Source.ID)
	require.NoError(t, err)
	assert.Equal(t, result.Book.ExtractedUnits, units)
	snapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, result.Source.ID)
	require.NoError(t, err)
	assert.Equal(t, result.Book.FullText, got.FullText)
	assert.Contains(t, got.FullText, "Bibliographie.")
	_, err = store.GetExtractedUnits(ctx, bob.ID, result.Source.ID)
	assert.ErrorIs(t, err, ErrExtractedUnitsUnavailable, "bob read alice units")
	second, err := NewService(store).Import(ctx, alice.ID, "de", fixtureDirectory(t, "testfixtures/epub3-edge-cases"))
	require.NoError(t, err)
	assert.Equal(t, result.Source.ID, second.Source.ID)
	var snapshotCount, unitCount int
	err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, result.Source.ID).Scan(&snapshotCount)
	require.NoError(t, err)
	err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, result.Source.ID).Scan(&unitCount)
	require.NoError(t, err)
	assert.Equal(t, 1, snapshotCount)
	assert.Equal(t, len(result.Book.ExtractedUnits.Units), unitCount)
	reimportSnapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, result.Source.ID)
	require.NoError(t, err)
	assert.Equal(t, snapshotID, reimportSnapshotID)
	replacement, err := NewService(store).Import(ctx, alice.ID, "de", fixtureDirectory(t, "testfixtures/epub3-reimport"))
	require.NoError(t, err)
	assert.Equal(t, result.Source.ID, replacement.Source.ID)
	units, err = store.GetExtractedUnits(ctx, alice.ID, result.Source.ID)
	require.NoError(t, err)
	assert.Equal(t, replacement.Book.ExtractedUnits, units)
	got, err = store.GetSourceMaterial(ctx, alice.ID, result.Source.ID)
	require.NoError(t, err)
	assert.Equal(t, replacement.Book.FullText, got.FullText)
	assert.Equal(t, replacement.Source.ContentHash, got.ContentHash)
	err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, result.Source.ID).Scan(&unitCount)
	require.NoError(t, err)
	assert.Equal(t, len(result.Book.ExtractedUnits.Units)+len(replacement.Book.ExtractedUnits.Units), unitCount)
	replacementSnapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, result.Source.ID)
	require.NoError(t, err)
	assert.NotEqual(t, snapshotID, replacementSnapshotID)
	_, err = admin.Exec(ctx, `INSERT INTO source_material_units(owner_id,source_material_id,snapshot_id,unit_id,unit_order,spine_index,title,title_source,text,start_offset,end_offset,package_path,manifest_id,source_href,resolved_href,media_type,linear) SELECT $1,source_material_id,snapshot_id,'cross-owner',99,99,'x','heading','x',0,1,'x','x','x','x','application/xhtml+xml',true FROM source_material_unit_snapshots WHERE owner_id=$2 AND source_material_id=$3`, bob.ID, alice.ID, result.Source.ID)
	assert.Error(t, err, "cross-owner unit insert succeeded")
	_, err = store.GetSourceMaterial(ctx, bob.ID, result.Source.ID)
	assert.ErrorIs(t, err, persistence.ErrNotFound, "bob read alice source")
	legacy, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "legacy-epub", Title: "Legacy", MediaType: MediaType(), ContentHash: "sha256:legacy", Content: []byte("retained epub"), FullText: "Legacy full text."})
	require.NoError(t, err)
	_, err = store.GetExtractedUnits(ctx, alice.ID, legacy.ID)
	assert.ErrorIs(t, err, ErrExtractedUnitsUnavailable, "legacy source units")
	var historyCount int
	err = admin.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='epub.import' AND status='complete' AND details->>'source_material_id'=$2`, alice.ID, result.Source.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 3, historyCount)
	_, err = admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, alice.ID)
	require.NoError(t, err)
	var sourceCount, remainingHistory, remainingUnits int
	err = admin.QueryRow(ctx, `SELECT count(*) FROM source_materials WHERE id=$1`, result.Source.ID).Scan(&sourceCount)
	require.NoError(t, err)
	err = admin.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1`, alice.ID).Scan(&remainingHistory)
	require.NoError(t, err)
	err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1`, alice.ID).Scan(&remainingUnits)
	require.NoError(t, err)
	assert.Equal(t, 0, sourceCount)
	assert.Equal(t, 0, remainingHistory)
	assert.Equal(t, 0, remainingUnits)
}
