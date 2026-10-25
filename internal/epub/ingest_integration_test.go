//go:build integration

package epub

import (
	"bytes"
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
	testutil.Cleanup(t, "epub store", store.Close)
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
	t.Run("bob cannot read Alice's extracted units", func(t *testing.T) {
		require.ErrorIs(t, err, ErrExtractedUnitsUnavailable)
	})
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
	t.Run("cross-owner unit insert is rejected", func(t *testing.T) {
		require.Error(t, err)
	})
	_, err = store.GetSourceMaterial(ctx, bob.ID, result.Source.ID)
	t.Run("bob cannot read Alice's source", func(t *testing.T) {
		require.ErrorIs(t, err, persistence.ErrNotFound)
	})
	legacy, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "legacy-epub", Title: "Legacy", MediaType: MediaType(), ContentHash: "sha256:legacy", Content: []byte("retained epub"), FullText: "Legacy full text."})
	require.NoError(t, err)
	_, err = store.GetExtractedUnits(ctx, alice.ID, legacy.ID)
	t.Run("legacy source units are unavailable", func(t *testing.T) {
		require.ErrorIs(t, err, ErrExtractedUnitsUnavailable)
	})
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

func TestImportReimportRestoresPreviousRevision(t *testing.T) {
	ctx := context.Background()
	url, admin := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "epub store", store.Close)
	owner, err := store.CreateUser(ctx, "epub-revision-return", false)
	require.NoError(t, err)

	first, err := NewService(store).Import(ctx, owner.ID, "de", fixtureDirectory(t, "testfixtures/epub3-edge-cases"))
	require.NoError(t, err)
	require.NotEmpty(t, first.Source.ContentRevisionID)
	firstSnapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, first.Source.ID)
	require.NoError(t, err)

	changed, err := NewService(store).Import(ctx, owner.ID, "de", fixtureDirectory(t, "testfixtures/epub3-reimport"))
	require.NoError(t, err)
	changedSnapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, first.Source.ID)
	require.NoError(t, err)
	t.Run("an import with changed content records a new revision and snapshot", func(t *testing.T) {
		require.Equal(t, first.Source.ID, changed.Source.ID)
		require.NotEqual(t, first.Source.ContentRevisionID, changed.Source.ContentRevisionID)
		require.NotEqual(t, firstSnapshotID, changedSnapshotID)
	})

	restored, err := NewService(store).Import(ctx, owner.ID, "de", fixtureDirectory(t, "testfixtures/epub3-edge-cases"))
	require.NoError(t, err)
	restoredUnits, err := store.GetExtractedUnits(ctx, owner.ID, first.Source.ID)
	require.NoError(t, err)
	restoredSnapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, first.Source.ID)
	require.NoError(t, err)
	t.Run("re-importing a known revision makes it current again", func(t *testing.T) {
		require.Equal(t, first.Source.ID, restored.Source.ID)
		require.Equal(t, first.Source.ContentHash, restored.Source.ContentHash)
		require.Equal(t, first.Source.ContentRevisionID, restored.Source.ContentRevisionID)
		require.Equal(t, first.Book.ExtractedUnits, restoredUnits)
	})
	t.Run("restoring a revision reuses its exact snapshot and adds no revision row", func(t *testing.T) {
		require.Equal(t, firstSnapshotID, restoredSnapshotID, "the same revision and extraction reuse one immutable snapshot")
		require.NotEqual(t, changedSnapshotID, restoredSnapshotID)
		var revisionCount int
		err := admin.QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, first.Source.ID).Scan(&revisionCount)
		require.NoError(t, err)
		require.Equal(t, 2, revisionCount)
	})
}

func TestImportMainTextFixturePreservesAllDeclaredUnits(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "epub store", store.Close)
	owner, err := store.CreateUser(ctx, "main-text-import", false)
	require.NoError(t, err)

	result, err := NewService(store).Import(ctx, owner.ID, "de", fixtureDirectory(t, "testfixtures/epub3-main-text"))
	require.NoError(t, err)
	assert.Equal(t, "Vorwort\n\nDies ist Vorwort.\n\nKapitel eins\n\nDies ist Haupttext eins.\n\nKapitel zwei\n\nDies ist Haupttext zwei.\n\nBibliographie\n\nDies ist Zusatztext.", result.Book.FullText)
	assert.Equal(t, result.Book.ExtractedUnits, mustExtractedUnits(t, store, owner.ID, result.Source.ID))
	assert.Equal(t, []string{"front", "chapter-one", "chapter-two", "bibliography"}, []string{
		result.Book.ExtractedUnits.Units[0].ManifestID,
		result.Book.ExtractedUnits.Units[1].ManifestID,
		result.Book.ExtractedUnits.Units[2].ManifestID,
		result.Book.ExtractedUnits.Units[3].ManifestID,
	})
}

// TestImportForAcquisitionWritesV2SnapshotAndRetainsBytes is the new-acquisition
// guard for the v2 cutover. A same-origin acquisition of the checksum-pinned
// Faust excerpt keeps the downloaded bytes unchanged, records the immutable v2
// snapshot with its role spans and heading resets, and analyses the whole
// snapshot because the edition declares no main-text landmark.
func TestImportForAcquisitionWritesV2SnapshotAndRetainsBytes(t *testing.T) {
	ctx := context.Background()
	url, admin := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "epub store", store.Close)
	owner, err := store.CreateUser(ctx, "faust-acquisition", false)
	require.NoError(t, err)

	data := fixtureDirectory(t, "testfixtures/faust-excerpt")
	extracted, err := ExtractV2(data)
	require.NoError(t, err)

	result, err := NewService(store).ImportForAcquisition(ctx, owner.ID, "de", AcquisitionContent{Content: data})
	require.NoError(t, err)
	require.False(t, result.AlreadyPresent)

	t.Run("retains the original bytes and digest unchanged", func(t *testing.T) {
		source, err := store.GetSourceMaterial(ctx, owner.ID, result.Source.ID)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(data, source.Content), "acquisition rewrote the retained original")
		assert.Equal(t, domain.EPUBContentDigest(data), source.ContentHash)
		assert.Equal(t, extracted.Book.FullText, source.FullText)
	})

	t.Run("records the v2 snapshot identity", func(t *testing.T) {
		snapshotID, units, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, result.Source.ID)
		require.NoError(t, err)
		require.NotEmpty(t, snapshotID)
		assert.Equal(t, int(domain.ExtractedUnitsSchemaVersionV2), units.SchemaVersion)
		assert.Equal(t, extracted.Book.ExtractedUnits, units, "the stored snapshot is the exact v2 extraction")
		assert.Equal(t, extracted.Book.ExtractedUnits.ReconstructText(), extracted.Book.FullText)

		var schemaVersion int
		var conventionSet string
		require.NoError(t, admin.QueryRow(ctx, `
SELECT schema_version, structure_convention_set
FROM source_material_unit_snapshots
WHERE owner_id=$1 AND source_material_id=$2 AND snapshot_id=$3`,
			owner.ID, result.Source.ID, snapshotID).Scan(&schemaVersion, &conventionSet))
		assert.Equal(t, int(domain.ExtractedUnitsSchemaVersionV2), schemaVersion)
		assert.Equal(t, domain.StructureConventionSet, conventionSet)

		tx, err := store.Pool().Begin(ctx)
		require.NoError(t, err)
		testutil.Cleanup(t, "snapshot structure read transaction", func() error { return tx.Rollback(ctx) })
		structure, err := persistence.LoadSnapshotStructure(ctx, tx, owner.ID, result.Source.ID, snapshotID)
		require.NoError(t, err)
		assert.True(t, structure.Recorded)
		assert.Equal(t, extracted.Structure.RoleSpans, structure.RoleSpans)
		assert.Equal(t, extracted.Structure.HeadingResets, structure.HeadingResets)
		require.NotEmpty(t, structure.RoleSpans, "the Director excerpts declare speaker and direction roles")
		require.NotEmpty(t, structure.HeadingResets, "the Vorspiel heading resets attribution")
	})

	t.Run("falls back to the whole snapshot as analysis scope", func(t *testing.T) {
		_, units, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, result.Source.ID)
		require.NoError(t, err)
		decision := IdentifyMainText(units.Units)
		assert.False(t, decision.Identified, "the excerpt declares no bodymatter landmark")
		assert.False(t, decision.Applies)
		selected := make([]string, 0, len(units.Units))
		for _, unit := range units.Units {
			selected = append(selected, unit.ID)
		}
		assert.Equal(t, selected, decision.SelectedUnitIDs)
		assert.Empty(t, decision.ExcludedUnitIDs)
		assert.Contains(t, result.Book.FullText, "Vorspiel", "header matter stays in the whole-snapshot fallback")
	})

	t.Run("a repeat acquisition converges on the same snapshot", func(t *testing.T) {
		firstSnapshot, _, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, result.Source.ID)
		require.NoError(t, err)
		second, err := NewService(store).ImportForAcquisition(ctx, owner.ID, "de", AcquisitionContent{Content: data})
		require.NoError(t, err)
		require.True(t, second.AlreadyPresent)
		assert.Equal(t, result.Source.ID, second.Source.ID)
		secondSnapshot, units, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, second.Source.ID)
		require.NoError(t, err)
		assert.Equal(t, firstSnapshot, secondSnapshot)
		assert.Equal(t, extracted.Book.ExtractedUnits, units)
		source, err := store.GetSourceMaterial(ctx, owner.ID, second.Source.ID)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(data, source.Content))
	})
}

func mustExtractedUnits(t *testing.T, store *persistence.PostgresStore, owner, source string) ExtractedUnits {
	t.Helper()
	units, err := store.GetExtractedUnits(context.Background(), owner, source)
	require.NoError(t, err)
	return units
}
