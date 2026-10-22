//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// extractedUnitFixture is one self-consistent unit for the text it spans.
func extractedUnitFixture(manifest string, order, spine uint64, text string, start, end uint64) domain.ExtractedUnit {
	return domain.ExtractedUnit{ID: domain.EPUBUnitID(spine, manifest), Order: order, SpineIndex: spine, ManifestID: manifest, Text: text, StartOffset: start, EndOffset: end}
}

// extractedUnitsFixture is the snapshot stored by the extracted-unit tests.
func extractedUnitsFixture() domain.ExtractedUnits {
	return domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: []domain.ExtractedUnit{
		extractedUnitFixture("unit", 0, 0, "text", 0, 4),
	}}
}

func putExtractedUnitsSource(t *testing.T, ctx context.Context, store *PostgresStore, owner, identifier string) domain.SourceMaterial {
	t.Helper()
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner, Language: "de", SourceIdentifier: identifier, Title: "Extracted units", MediaType: "application/epub+zip", Content: []byte("text"), FullText: "text"}, extractedUnitsFixture())
	require.NoError(t, err)
	return source
}

// TestPutSourceMaterialWithExtractedUnitsResolvesContentIdentity drives the
// content-identity step: unit validation, the missing-identity guard, and the
// legacy tagging of a bare content hash.
func TestPutSourceMaterialWithExtractedUnitsResolvesContentIdentity(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "epub-unit-identity", false)
	require.NoError(t, err)

	t.Run("rejects units that do not match the full text", func(t *testing.T) {
		bad := domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: []domain.ExtractedUnit{
			extractedUnitFixture("unit", 0, 0, "other", 0, 4),
		}}
		_, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "bad-units", Title: "Bad", MediaType: "application/epub+zip", Content: []byte("text"), FullText: "text"}, bad)
		require.ErrorContains(t, err, "validate extracted units")
	})

	t.Run("requires a content identity", func(t *testing.T) {
		_, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "no-identity", Title: "No identity", MediaType: "application/epub+zip", FullText: "text"}, extractedUnitsFixture())
		require.ErrorContains(t, err, "content identity is required")
	})

	t.Run("tags a bare content hash as legacy", func(t *testing.T) {
		source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "legacy-identity", Title: "Legacy", MediaType: "application/epub+zip", ContentHash: "bare-hash", FullText: "text"}, extractedUnitsFixture())
		require.NoError(t, err)
		require.Equal(t, "legacy:bare-hash", source.ContentHash)
		stored, err := store.GetExtractedUnits(ctx, owner.ID, source.ID)
		require.NoError(t, err)
		require.Equal(t, extractedUnitsFixture(), stored)
	})
}

// TestPutSourceMaterialWithExtractedUnitsPropagatesPersistenceFailures drops
// the table behind each write so a database failure reaches the caller.
func TestPutSourceMaterialWithExtractedUnitsPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	run := func(t *testing.T, statement string) error {
		t.Helper()
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "epub-unit-put-failure", false)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, statement)
		require.NoError(t, err)
		_, err = store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "put-failure", Title: "Put failure", MediaType: "application/epub+zip", Content: []byte("text"), FullText: "text"}, extractedUnitsFixture())
		return err
	}

	t.Run("the source material upsert fails", func(t *testing.T) {
		require.Error(t, run(t, `DROP TABLE source_materials CASCADE`))
	})
	t.Run("the revision lookup fails", func(t *testing.T) {
		require.Error(t, run(t, `DROP TABLE source_content_revisions CASCADE`))
	})
	t.Run("the snapshot insert fails", func(t *testing.T) {
		require.Error(t, run(t, `DROP TABLE source_material_unit_snapshots CASCADE`))
	})
}

// insertCorruptUnit adds one extra unit to the source's current snapshot. The
// units table rejects UPDATE, so corruption is seeded by inserting a row the
// loader must reject.
const insertCorruptUnit = `INSERT INTO source_material_units(owner_id, source_material_id, snapshot_id, unit_id, unit_order, spine_index, title, title_source, text, start_offset, end_offset, package_path, manifest_id, source_href, resolved_href, media_type, properties, linear, navigation_labels, landmark_types)
SELECT $1, $2, s.current_snapshot_id, 'epub-unit-v1:1:extra', 1, 1, 'Extra', 'heading', 'text', 0, 4, 'OPS', 'extra', 'Text/extra.xhtml', 'OPS/Text/extra.xhtml', 'application/xhtml+xml', $3::jsonb, true, $4::jsonb, $5::jsonb
FROM source_materials s WHERE s.owner_id = $1 AND s.id = $2`

// TestGetExtractedUnitSnapshotRejectsCorruptPersistedUnits seeds a row the
// loader must reject so each reconciliation check reaches its own branch.
func TestGetExtractedUnitSnapshotRejectsCorruptPersistedUnits(t *testing.T) {
	ctx := context.Background()
	url, admin := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "epub-unit-corrupt", false)
	require.NoError(t, err)

	tests := []struct {
		name       string
		properties string
		labels     string
		landmarks  string
		want       string
	}{
		{name: "invalid properties JSON", properties: "{}", labels: "[]", landmarks: "[]", want: "cannot unmarshal object into Go value of type []string"},
		{name: "invalid navigation_labels JSON", properties: "[]", labels: "{}", landmarks: "[]", want: "cannot unmarshal object into Go value of type []string"},
		{name: "invalid landmark_types JSON", properties: "[]", labels: "[]", landmarks: "{}", want: "cannot unmarshal object into Go value of type []string"},
		{name: "offsets that do not match the full text", properties: "[]", labels: "[]", landmarks: "[]", want: "validate persisted extracted units"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := putExtractedUnitsSource(t, ctx, store, owner.ID, "corrupt-"+tt.name)
			_, err := admin.Exec(ctx, insertCorruptUnit, owner.ID, source.ID, tt.properties, tt.labels, tt.landmarks)
			require.NoError(t, err)

			_, _, err = store.GetExtractedUnitSnapshot(ctx, owner.ID, source.ID)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// TestGetExtractedUnitSnapshotPropagatesPersistenceFailures drops the table
// behind each read so a database failure reaches the caller instead of being
// read as an absent snapshot.
func TestGetExtractedUnitSnapshotPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	run := func(t *testing.T, statement string) error {
		t.Helper()
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "epub-unit-get-failure", false)
		require.NoError(t, err)
		source := putExtractedUnitsSource(t, ctx, store, owner.ID, "get-failure")
		_, err = store.Pool().Exec(ctx, statement)
		require.NoError(t, err)
		_, _, err = store.GetExtractedUnitSnapshot(ctx, owner.ID, source.ID)
		return err
	}

	t.Run("the current snapshot lookup fails", func(t *testing.T) {
		require.Error(t, run(t, `DROP TABLE source_material_unit_snapshots CASCADE`))
	})
	t.Run("the unit list fails", func(t *testing.T) {
		require.Error(t, run(t, `DROP TABLE source_material_units CASCADE`))
	})
}
