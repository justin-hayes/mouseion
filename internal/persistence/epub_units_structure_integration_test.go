//go:build integration

package persistence

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func structureUnitsFixture() domain.ExtractedUnits {
	first, second := "Erster Satz.", "Zweiter Satz."
	firstEnd := uint64(len([]rune(first)))
	secondStart := firstEnd + 2
	return domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "one"), Order: 0, SpineIndex: 0, ManifestID: "one", Text: first, StartOffset: 0, EndOffset: firstEnd},
		{ID: domain.EPUBUnitID(1, "two"), Order: 1, SpineIndex: 1, ManifestID: "two", Text: second, StartOffset: secondStart, EndOffset: secondStart + uint64(len([]rune(second)))},
	}}
}

func putStructureSource(t *testing.T, ctx context.Context, store *PostgresStore, owner, identifier string) domain.SourceMaterial {
	t.Helper()
	units := structureUnitsFixture()
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: "de", SourceIdentifier: identifier, Title: "Structure",
		MediaType: "application/epub+zip", Content: []byte(identifier), FullText: units.ReconstructText(),
	}, units)
	require.NoError(t, err)
	require.NotEmpty(t, source.ContentRevisionID)
	return source
}

func currentSnapshotID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, sourceID string) string {
	t.Helper()
	var snapshotID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, sourceID).Scan(&snapshotID))
	return snapshotID
}

// insertStructureSnapshot writes one v2 snapshot directly, because no
// application writer emits v2 yet. The units must be self-consistent with their
// own reconstructed text.
func insertStructureSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, sourceID, revisionID string, convention string, units []domain.ExtractedUnit) string {
	t.Helper()
	var snapshotID string
	require.NoError(t, pool.QueryRow(ctx, `
INSERT INTO source_material_unit_snapshots(owner_id, source_material_id, content_revision_id, schema_version, structure_convention_set)
VALUES ($1, $2, $3, $4, $5)
RETURNING snapshot_id::text`, owner, sourceID, revisionID, domain.ExtractedUnitsSchemaVersionV2, convention).Scan(&snapshotID))
	for _, unit := range units {
		_, err := pool.Exec(ctx, `
INSERT INTO source_material_units(owner_id, source_material_id, snapshot_id, unit_id, unit_order, spine_index, title, title_source, text,
                                  start_offset, end_offset, package_path, manifest_id, source_href, resolved_href, media_type,
                                  properties, linear, navigation_labels, landmark_types)
VALUES ($1, $2, $3, $4, $5, $6, '', 'heading', $7, $8, $9, '', $10, '', '', 'application/xhtml+xml',
        '[]'::jsonb, true, '[]'::jsonb, '[]'::jsonb)`,
			owner, sourceID, snapshotID, unit.ID, unit.Order, unit.SpineIndex, unit.Text, unit.StartOffset, unit.EndOffset, unit.ManifestID)
		require.NoError(t, err)
	}
	return snapshotID
}

func insertRoleSpan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, sourceID, snapshotID, unitID, role string, start, end int) error {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO source_material_unit_role_spans(owner_id, source_material_id, snapshot_id, unit_id, role, start_offset, end_offset)
VALUES ($1, $2, $3, $4, $5, $6, $7)`, owner, sourceID, snapshotID, unitID, role, start, end)
	return err
}

func insertHeadingReset(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, sourceID, snapshotID, unitID string, offset int) error {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO source_material_unit_heading_resets(owner_id, source_material_id, snapshot_id, unit_id, "offset")
VALUES ($1, $2, $3, $4, $5)`, owner, sourceID, snapshotID, unitID, offset)
	return err
}

// TestExtractedSnapshotTextComesFromItsOwnUnits proves that a read reconstructs
// the snapshot text from ordered unit rows and never validates against the
// content revision's historical FullText.
func TestExtractedSnapshotTextComesFromItsOwnUnits(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "source-structure-reconstruct", false)
	require.NoError(t, err)
	source := putStructureSource(t, ctx, store, owner.ID, "reconstruct")

	current, err := store.GetCurrentExtractedSnapshot(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ExtractedUnitsSchemaVersionV1, current.Units.SchemaVersion)
	assert.False(t, current.Structure.Recorded, "v1 structure is not recorded")
	assert.Empty(t, current.Structure.RoleSpans)
	assert.Equal(t, source.FullText, current.Units.ReconstructText())
	assert.Equal(t, source.ContentRevisionID, current.ContentRevisionID)

	// A v2 extraction under the same immutable revision may carry text that the
	// historical revision FullText does not contain (for example, <br> became
	// "\n"). The exact-snapshot reader must still accept and read it.
	first, second := "Erster\nSatz.", "Zweiter Satz."
	firstEnd := uint64(len([]rune(first)))
	v2Units := []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "one"), Order: 0, SpineIndex: 0, ManifestID: "one", Text: first, StartOffset: 0, EndOffset: firstEnd},
		{ID: domain.EPUBUnitID(1, "two"), Order: 1, SpineIndex: 1, ManifestID: "two", Text: second, StartOffset: firstEnd + 2, EndOffset: firstEnd + 2 + uint64(len([]rune(second)))},
	}
	snapshotID := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, v2Units)

	snapshot, err := store.GetExtractedSnapshotByID(ctx, owner.ID, source.ID, snapshotID)
	require.NoError(t, err)
	assert.Equal(t, domain.ExtractedUnitsSchemaVersionV2, snapshot.Units.SchemaVersion)
	assert.Equal(t, first+"\n\n"+second, snapshot.Units.ReconstructText())
	assert.NotContains(t, source.FullText, "\nSatz.", "the fixture must differ from the historical revision text")
	assert.True(t, snapshot.Structure.Recorded)
	assert.Equal(t, domain.StructureConventionSet, snapshot.Structure.ConventionSet)
}

// TestExtractedSnapshotReconstructionMatchesCurrentExtractedFixture checks the
// real EPUB fixture's v1 snapshot text against reconstruction from its unit
// rows, and confirms the writer still emits contract v1.
func TestExtractedSnapshotReconstructionMatchesCurrentExtractedFixture(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "source-structure-fixture", false)
	require.NoError(t, err)
	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "epub3-edge-cases"))
	book, err := epub.Extract(data)
	require.NoError(t, err)

	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: book.SourceIdentifier, Title: book.Title,
		MediaType: "application/epub+zip", Content: data, FullText: book.FullText,
	}, book.ExtractedUnits)
	require.NoError(t, err)

	var schemaVersion int
	var conventionSet *string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT snap.schema_version, snap.structure_convention_set
FROM source_material_unit_snapshots snap
JOIN source_materials source ON source.owner_id=snap.owner_id AND source.id=snap.source_material_id AND source.current_snapshot_id=snap.snapshot_id
WHERE snap.owner_id=$1 AND snap.source_material_id=$2`, owner.ID, source.ID).Scan(&schemaVersion, &conventionSet))
	assert.Equal(t, domain.ExtractedUnitsSchemaVersionV1, schemaVersion, "writer must remain v1")
	assert.Nil(t, conventionSet, "v1 must not record a convention set")

	current, err := store.GetCurrentExtractedSnapshot(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, book.ExtractedUnits, current.Units)
	assert.Equal(t, book.FullText, current.Units.ReconstructText())
	assert.False(t, current.Structure.Recorded)
}

// TestExtractedSnapshotStructureResolvesThroughTheExactSnapshot covers
// owner/source/snapshot scoping, unit IDs that recur across snapshots, role
// nesting and heading resets.
func TestExtractedSnapshotStructureResolvesThroughTheExactSnapshot(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "source-structure-scope", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "source-structure-scope-other", false)
	require.NoError(t, err)
	source := putStructureSource(t, ctx, store, owner.ID, "scope")
	otherSource := putStructureSource(t, ctx, store, other.ID, "scope-other")
	units := structureUnitsFixture()
	firstUnit := units.Units[0].ID

	// Snapshot A records a speaker label with a nested direction and a heading
	// reset. Snapshot B reuses the same unit ID under another convention set but
	// records different roles.
	snapshotA := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, units.Units)
	require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, snapshotA, firstUnit, "speaker", 0, 12))
	require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, snapshotA, firstUnit, "stage_direction", 7, 12))
	require.NoError(t, insertHeadingReset(t, ctx, pool, owner.ID, source.ID, snapshotA, firstUnit, 7))

	snapshotB := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, "epub-structure/2", units.Units)
	require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, snapshotB, firstUnit, "speaker", 0, 6))

	readA, err := store.GetExtractedSnapshotByID(ctx, owner.ID, source.ID, snapshotA)
	require.NoError(t, err)
	require.True(t, readA.Structure.Recorded)
	assert.Equal(t, []domain.UnitRoleSpan{
		{UnitID: firstUnit, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 12},
		{UnitID: firstUnit, Role: domain.SourceStructureRoleDirection, StartOffset: 7, EndOffset: 12},
	}, readA.Structure.RoleSpans)
	assert.Equal(t, []domain.UnitHeadingReset{{UnitID: firstUnit, Offset: 7}}, readA.Structure.HeadingResets)

	readB, err := store.GetExtractedSnapshotByID(ctx, owner.ID, source.ID, snapshotB)
	require.NoError(t, err)
	assert.Equal(t, []domain.UnitRoleSpan{{UnitID: firstUnit, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 6}}, readB.Structure.RoleSpans)
	assert.Empty(t, readB.Structure.HeadingResets)

	_, err = store.GetExtractedSnapshotByID(ctx, other.ID, source.ID, snapshotA)
	require.ErrorIs(t, err, domain.ErrExtractedUnitsUnavailable, "another owner must not read the snapshot")
	_, err = store.GetExtractedSnapshotByID(ctx, owner.ID, otherSource.ID, snapshotA)
	require.ErrorIs(t, err, domain.ErrExtractedUnitsUnavailable, "another source must not read the snapshot")
	_, err = store.GetExtractedSnapshotByID(ctx, owner.ID, source.ID, "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, domain.ErrExtractedUnitsUnavailable)

	// The current reader follows the source pointer and reads the exact snapshot
	// it names, including structure.
	_, err = pool.Exec(ctx, `UPDATE source_materials SET current_snapshot_id=$3 WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID, snapshotB)
	require.NoError(t, err)
	current, err := store.GetCurrentExtractedSnapshot(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, snapshotB, current.SnapshotID)
	assert.Equal(t, "epub-structure/2", current.Structure.ConventionSet)
	assert.Equal(t, []domain.UnitRoleSpan{{UnitID: firstUnit, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 6}}, current.Structure.RoleSpans)
}

// TestSourceStructureMigrationValidatesRolesAndBounds drives the physical
// validation: unknown roles, out-of-unit bounds, improper overlap and
// contradictory nesting are rejected, while a direction nested in a speaker
// label is accepted.
func TestSourceStructureMigrationValidatesRolesAndBounds(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "source-structure-validation", false)
	require.NoError(t, err)
	source := putStructureSource(t, ctx, store, owner.ID, "validation")
	units := structureUnitsFixture()
	firstUnit := units.Units[0].ID
	v1Snapshot := currentSnapshotID(t, ctx, pool, owner.ID, source.ID)
	v2Snapshot := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, units.Units)

	t.Run("unknown role", func(t *testing.T) {
		require.ErrorContains(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, v2Snapshot, firstUnit, "heading", 0, 4), "role_check")
	})
	t.Run("empty span", func(t *testing.T) {
		require.ErrorContains(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, v2Snapshot, firstUnit, "speaker", 4, 4), "outside the unit text")
	})
	t.Run("span past unit text", func(t *testing.T) {
		require.ErrorContains(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, v2Snapshot, firstUnit, "speaker", 0, 100), "outside the unit text")
	})
	t.Run("span in a unit outside the snapshot", func(t *testing.T) {
		require.ErrorContains(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, v2Snapshot, "epub-unit-v1:9:other", "speaker", 0, 4), "does not belong to snapshot")
	})
	t.Run("v1 snapshot rejects roles", func(t *testing.T) {
		require.ErrorContains(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, v1Snapshot, firstUnit, "speaker", 0, 4), "require an extraction contract v2 snapshot")
	})
	t.Run("proper nesting inside a speaker label", func(t *testing.T) {
		require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, v2Snapshot, firstUnit, "speaker", 0, 12))
		require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, v2Snapshot, firstUnit, "stage_direction", 7, 12))
	})
	t.Run("overlap without nesting", func(t *testing.T) {
		otherSnapshot := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, "epub-structure/overlap", units.Units)
		require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, otherSnapshot, firstUnit, "speaker", 0, 8))
		require.ErrorContains(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, otherSnapshot, firstUnit, "speaker", 5, 12), "overlap without nesting")
	})
	t.Run("same bounds with contradictory roles", func(t *testing.T) {
		require.ErrorContains(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, v2Snapshot, firstUnit, "stage_direction", 0, 12), "contradict")
	})
	t.Run("speaker inside a direction", func(t *testing.T) {
		otherSnapshot := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, "epub-structure/direction", units.Units)
		require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, otherSnapshot, firstUnit, "stage_direction", 0, 12))
		require.ErrorContains(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, otherSnapshot, firstUnit, "speaker", 2, 6), "nesting a speaker inside a direction")
	})
	t.Run("heading reset past unit text", func(t *testing.T) {
		require.ErrorContains(t, insertHeadingReset(t, ctx, pool, owner.ID, source.ID, v2Snapshot, firstUnit, 100), "outside the unit text")
	})
	t.Run("v1 snapshot rejects heading resets", func(t *testing.T) {
		require.ErrorContains(t, insertHeadingReset(t, ctx, pool, owner.ID, source.ID, v1Snapshot, firstUnit, 1), "require an extraction contract v2 snapshot")
	})
	t.Run("rows are immutable", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE source_material_unit_role_spans SET role='stage_direction' WHERE owner_id=$1 AND source_material_id=$2 AND snapshot_id=$3`, owner.ID, source.ID, v2Snapshot)
		require.ErrorContains(t, err, "source structure is immutable")
	})
}

// TestSourceStructureSnapshotUniquenessAppliesOnlyToV2 keeps historical v1
// duplicate-shaped snapshots readable while enforcing one new snapshot per
// revision, contract and convention set.
func TestSourceStructureSnapshotUniquenessAppliesOnlyToV2(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "source-structure-uniqueness", false)
	require.NoError(t, err)
	source := putStructureSource(t, ctx, store, owner.ID, "uniqueness")
	units := structureUnitsFixture()

	insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, units.Units)
	_, err = pool.Exec(ctx, `
INSERT INTO source_material_unit_snapshots(owner_id, source_material_id, content_revision_id, schema_version, structure_convention_set)
VALUES ($1, $2, $3, 2, $4)`, owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet)
	require.ErrorContains(t, err, "source_material_unit_snapshots_v2_extraction_identity", "duplicate v2 extraction must be rejected")

	// A different convention set is a different extraction.
	insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, "epub-structure/2", units.Units)

	// Historical v1 snapshots may be duplicate-shaped; they are never
	// deduplicated or rewritten by the new constraint.
	for range 2 {
		_, err = pool.Exec(ctx, `
INSERT INTO source_material_unit_snapshots(owner_id, source_material_id, content_revision_id, schema_version)
VALUES ($1, $2, $3, 1)`, owner.ID, source.ID, source.ContentRevisionID)
		require.NoError(t, err, "historical v1 snapshots must coexist")
	}
}

func TestSourceStructureCheckRecord(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "source-structure-check", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "source-structure-check-other", false)
	require.NoError(t, err)
	source := putStructureSource(t, ctx, store, owner.ID, "check")
	otherSource := putStructureSource(t, ctx, store, other.ID, "check-other")
	snapshotID := currentSnapshotID(t, ctx, pool, owner.ID, source.ID)
	check := domain.SourceStructureCheck{
		OwnerID: owner.ID, SourceMaterialID: source.ID, ContentRevisionID: source.ContentRevisionID,
		SnapshotID: snapshotID, TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2,
		ConventionSet: domain.StructureConventionSet, Outcome: domain.SourceStructureUnchanged,
		ResultIdentity: "sha256:checked-result",
	}
	require.NoError(t, store.RecordSourceStructureCheck(ctx, check))

	stored, found, err := store.GetSourceStructureCheck(ctx, check.Key())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, domain.SourceStructureUnchanged, stored.Outcome)
	assert.Equal(t, "sha256:checked-result", stored.ResultIdentity)
	assert.NotZero(t, stored.CheckedAt)

	// The exact key is idempotent, but a different result for the same key is
	// reported instead of silently replacing or reusing it.
	require.NoError(t, store.RecordSourceStructureCheck(ctx, check))
	check.Outcome = domain.SourceStructureUpdateAvailable
	require.ErrorContains(t, store.RecordSourceStructureCheck(ctx, check), "already holds a different result")
	reused, found, err := store.GetSourceStructureCheck(ctx, check.Key())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, domain.SourceStructureUnchanged, reused.Outcome)

	otherKey := check.Key()
	otherKey.OwnerID, otherKey.SourceMaterialID = other.ID, otherSource.ID
	_, found, err = store.GetSourceStructureCheck(ctx, otherKey)
	require.NoError(t, err)
	assert.False(t, found, "checks are owner/source scoped")

	check.TargetContractVersion = domain.ExtractedUnitsSchemaVersionV1
	require.ErrorContains(t, store.RecordSourceStructureCheck(ctx, check), "is not v2")
	check.TargetContractVersion = domain.ExtractedUnitsSchemaVersionV2
	check.Outcome = "failed"
	require.ErrorContains(t, store.RecordSourceStructureCheck(ctx, check), "unknown outcome")

	_, err = pool.Exec(ctx, `
INSERT INTO source_structure_checks(owner_id, source_material_id, content_revision_id, snapshot_id, target_contract_version, structure_convention_set, outcome, result_identity)
VALUES ($1, $2, $3, $4, 2, $5, 'failed', 'sha256:failed')`, owner.ID, source.ID, source.ContentRevisionID, snapshotID, "epub-structure/direct")
	require.ErrorContains(t, err, "outcome_check")

	_, err = pool.Exec(ctx, `UPDATE source_structure_checks SET outcome='update_available' WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)
	require.ErrorContains(t, err, "source structure is immutable")
}

// TestExtractedSnapshotReadsMissingSourcesAndPropagatesFailures covers the
// absent-snapshot sentinel and each new read path's database failure.
func TestExtractedSnapshotReadsMissingSourcesAndPropagatesFailures(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "source-structure-read-failure", false)
	require.NoError(t, err)

	legacy, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: "legacy-no-snapshot",
		Title: "Legacy", MediaType: "application/epub+zip", ContentHash: "legacy", FullText: "text",
	})
	require.NoError(t, err)
	_, err = store.GetCurrentExtractedSnapshot(ctx, owner.ID, legacy.ID)
	require.ErrorIs(t, err, domain.ErrExtractedUnitsUnavailable)

	source := putStructureSource(t, ctx, store, owner.ID, "read-failure")
	units := structureUnitsFixture()
	snapshotID := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, units.Units)
	require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, snapshotID, units.Units[0].ID, "speaker", 0, 5))

	for name, statement := range map[string]string{
		"snapshot identity": `DROP TABLE source_material_unit_snapshots CASCADE`,
		"units":             `DROP TABLE source_material_units CASCADE`,
		"role spans":        `DROP TABLE source_material_unit_role_spans CASCADE`,
		"heading resets":    `DROP TABLE source_material_unit_heading_resets CASCADE`,
		"structure checks":  `DROP TABLE source_structure_checks CASCADE`,
	} {
		t.Run(name+" failure", func(t *testing.T) {
			newURL, _ := testutil.Postgres(t, ctx, Migrate)
			newStore := openIntegrationStore(t, ctx, newURL)
			newOwner, err := newStore.CreateUser(ctx, "source-structure-read-failure-"+name, false)
			require.NoError(t, err)
			newSource := putStructureSource(t, ctx, newStore, newOwner.ID, "read-failure-"+name)
			newSnapshot := insertStructureSnapshot(t, ctx, newStore.Pool(), newOwner.ID, newSource.ID, newSource.ContentRevisionID, domain.StructureConventionSet, units.Units)
			_, err = newStore.Pool().Exec(ctx, statement)
			require.NoError(t, err)
			if name == "structure checks" {
				_, _, err = newStore.GetSourceStructureCheck(ctx, domain.SourceStructureCheckKey{
					OwnerID: newOwner.ID, SourceMaterialID: newSource.ID, ContentRevisionID: newSource.ContentRevisionID,
					SnapshotID: newSnapshot, TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2,
					ConventionSet: domain.StructureConventionSet,
				})
			} else {
				_, err = newStore.GetExtractedSnapshotByID(ctx, newOwner.ID, newSource.ID, newSnapshot)
			}
			require.Error(t, err)
		})
	}
}

// TestSourceStructureV2MigrationReversesCleanly runs the successor migration
// down and up again, so the down script is part of the tested contract.
func TestSourceStructureV2MigrationReversesCleanly(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)

	moveApplicationMigrationsTo(t, url, 44)
	var present bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.source_material_unit_role_spans') IS NOT NULL`).Scan(&present))
	assert.False(t, present, "role span table survived the down migration")
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.source_material_unit_heading_resets') IS NOT NULL`).Scan(&present))
	assert.False(t, present, "heading reset table survived the down migration")
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.source_structure_checks') IS NOT NULL`).Scan(&present))
	assert.False(t, present, "structure check table survived the down migration")
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema='public' AND table_name='source_material_unit_snapshots' AND column_name='structure_convention_set')`).Scan(&present))
	assert.False(t, present, "convention-set column survived the down migration")

	moveApplicationMigrationsTo(t, url, 45)
	for _, table := range []string{"source_material_unit_role_spans", "source_material_unit_heading_resets", "source_structure_checks"} {
		require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&present))
		assert.True(t, present, "table %s was not recreated", table)
	}
}
