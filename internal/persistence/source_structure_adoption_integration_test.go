//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startTx(t *testing.T, ctx context.Context, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	return tx
}

// adoptRejected runs one adoption in its own transaction and rolls back, so a
// guard that returns before commit cannot hold the source row lock.
func adoptRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, adoption ExtractedSnapshotAdoption) error {
	t.Helper()
	tx := startTx(t, ctx, pool)
	_, err := AdoptExtractedSnapshot(ctx, tx, adoption)
	require.NoError(t, tx.Rollback(ctx))
	return err
}

func adoptionFixture() (domain.ExtractedUnits, domain.SnapshotStructure) {
	units := structureUnitsFixture()
	units.SchemaVersion = domain.ExtractedUnitsSchemaVersionV2
	structure := domain.SnapshotStructure{
		Recorded: true, ConventionSet: domain.StructureConventionSet,
		RoleSpans: []domain.UnitRoleSpan{{
			UnitID: units.Units[0].ID, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 6,
		}},
	}
	return units, structure
}

func TestAdoptExtractedSnapshotCreatesReusesAndGuards(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "adopt-snapshot", false)
	require.NoError(t, err)
	source := putStructureSource(t, ctx, store, owner.ID, "adopt-snapshot")
	original := currentSnapshotID(t, ctx, pool, owner.ID, source.ID)
	units, structure := adoptionFixture()
	adoption := ExtractedSnapshotAdoption{
		OwnerID: owner.ID, SourceMaterialID: source.ID,
		ContentRevisionID: source.ContentRevisionID, ExpectedSnapshotID: original,
		Units: units, Structure: structure,
	}

	t.Run("invalid identity and extraction", func(t *testing.T) {
		require.ErrorContains(t, adoptRejected(t, ctx, pool, ExtractedSnapshotAdoption{}), "identity is incomplete")
		invalid := adoption
		invalid.Units.SchemaVersion = domain.ExtractedUnitsSchemaVersionV1
		require.ErrorContains(t, adoptRejected(t, ctx, pool, invalid), "is not v2")
		invalid = adoption
		invalid.Units.Units = nil
		require.ErrorContains(t, adoptRejected(t, ctx, pool, invalid), "validate units")
		invalid = adoption
		invalid.Structure = domain.SnapshotStructure{}
		require.ErrorContains(t, adoptRejected(t, ctx, pool, invalid), "validate structure")
	})

	t.Run("missing source", func(t *testing.T) {
		missing := adoption
		missing.SourceMaterialID = "00000000-0000-0000-0000-000000000000"
		require.ErrorIs(t, adoptRejected(t, ctx, pool, missing), ErrNotFound)
	})

	t.Run("changed revision and changed current snapshot are rejected", func(t *testing.T) {
		changed := adoption
		changed.ContentRevisionID = "00000000-0000-0000-0000-000000000000"
		require.ErrorIs(t, adoptRejected(t, ctx, pool, changed), ErrSourceEvidenceChanged)
		changed = adoption
		changed.ExpectedSnapshotID = "00000000-0000-0000-0000-000000000000"
		require.ErrorIs(t, adoptRejected(t, ctx, pool, changed), ErrSourceEvidenceChanged)
	})

	t.Run("creates the exact snapshot and switches the pointer", func(t *testing.T) {
		tx := startTx(t, ctx, pool)
		adopted, err := AdoptExtractedSnapshot(ctx, tx, adoption)
		require.NoError(t, err)
		require.NoError(t, tx.Commit(ctx))
		assert.NotEqual(t, original, adopted)
		assert.Equal(t, adopted, currentSnapshotID(t, ctx, pool, owner.ID, source.ID))
		read, err := store.GetExtractedSnapshotByID(ctx, owner.ID, source.ID, adopted)
		require.NoError(t, err)
		assert.Equal(t, units, read.Units)
		assert.Equal(t, structure, read.Structure)
	})

	t.Run("a repeat with the same checked extraction reuses it", func(t *testing.T) {
		before := currentSnapshotID(t, ctx, pool, owner.ID, source.ID)
		tx := startTx(t, ctx, pool)
		adopted, err := AdoptExtractedSnapshot(ctx, tx, adoption)
		require.NoError(t, err)
		require.NoError(t, tx.Commit(ctx))
		assert.Equal(t, before, adopted)
		assert.Equal(t, before, currentSnapshotID(t, ctx, pool, owner.ID, source.ID))
	})
}

func TestAdoptExtractedSnapshotPropagatesDatabaseFailures(t *testing.T) {
	ctx := context.Background()
	for name, statement := range map[string]string{
		"source lock":            `DROP TABLE source_materials CASCADE`,
		"snapshot identity read": `DROP TABLE source_material_unit_snapshots CASCADE`,
	} {
		t.Run(name, func(t *testing.T) {
			url, pool := testutil.Postgres(t, ctx, Migrate)
			store := openIntegrationStore(t, ctx, url)
			owner, err := store.CreateUser(ctx, "adopt-failure-"+name, false)
			require.NoError(t, err)
			source := putStructureSource(t, ctx, store, owner.ID, "adopt-failure-"+name)
			units, structure := adoptionFixture()
			adoption := ExtractedSnapshotAdoption{
				OwnerID: owner.ID, SourceMaterialID: source.ID,
				ContentRevisionID: source.ContentRevisionID, ExpectedSnapshotID: currentSnapshotID(t, ctx, pool, owner.ID, source.ID),
				Units: units, Structure: structure,
			}
			_, err = pool.Exec(ctx, statement)
			require.NoError(t, err)
			require.Error(t, adoptRejected(t, ctx, pool, adoption))
		})
	}
}

func TestAdoptExtractedSnapshotRejectsAnExistingTargetBehindAnotherCurrentSnapshot(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "adopt-existing-target", false)
	require.NoError(t, err)
	source := putStructureSource(t, ctx, store, owner.ID, "adopt-existing-target")
	original := currentSnapshotID(t, ctx, pool, owner.ID, source.ID)
	units, structure := adoptionFixture()
	adoption := ExtractedSnapshotAdoption{
		OwnerID: owner.ID, SourceMaterialID: source.ID,
		ContentRevisionID: source.ContentRevisionID, ExpectedSnapshotID: original,
		Units: units, Structure: structure,
	}
	tx := startTx(t, ctx, pool)
	adopted, err := AdoptExtractedSnapshot(ctx, tx, adoption)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	// Another actor switches to a different snapshot of the same revision; the
	// exact target still exists, but the source is no longer where the learner
	// saw it. The offer must refresh rather than silently switch back.
	other := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, "epub-structure/other", units.Units)
	_, err = pool.Exec(ctx, `UPDATE source_materials SET current_snapshot_id=$3 WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID, other)
	require.NoError(t, err)
	err = adoptRejected(t, ctx, pool, adoption)
	require.ErrorIs(t, err, ErrSourceEvidenceChanged)
	assert.Equal(t, other, currentSnapshotID(t, ctx, pool, owner.ID, source.ID))
	assert.NotEqual(t, adopted, other)
}
