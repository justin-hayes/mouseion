package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// GetEPUBReviewedScope reloads one immutable scope through its complete owner/source identity.
func (s *PostgresStore) GetEPUBReviewedScope(ctx context.Context, owner, sourceID, scopeID string) (domain.EPUBReviewedScopeSnapshot, error) {
	var scope domain.EPUBReviewedScopeSnapshot
	err := s.pool.QueryRow(ctx, `SELECT scope.schema_version,scope.scope_id::text,scope.owner_id::text,scope.source_material_id::text,scope.content_revision_id::text,revision.content_digest,revision.digest_version,scope.snapshot_id,scope.extracted_units_schema_version,scope.created_at
		FROM epub_reviewed_scopes scope JOIN source_content_revisions revision ON revision.owner_id=scope.owner_id AND revision.source_material_id=scope.source_material_id AND revision.revision_id=scope.content_revision_id WHERE scope.scope_id=$1 AND scope.owner_id=$2 AND scope.source_material_id=$3`, scopeID, owner, sourceID).Scan(
		&scope.SchemaVersion, &scope.ScopeID, &scope.OwnerID, &scope.SourceMaterialID, &scope.SourceContent.RevisionID, &scope.SourceContent.Digest, &scope.SourceContent.DigestVersion, &scope.SourceUnitSnapshot.SnapshotID,
		&scope.SourceUnitSnapshot.ExtractedUnitsSchemaVersion, &scope.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EPUBReviewedScopeSnapshot{}, ErrNotFound
	}
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT unit_id,unit_order FROM epub_reviewed_scope_units WHERE scope_id=$1 AND owner_id=$2 AND source_material_id=$3 ORDER BY unit_order`, scopeID, owner, sourceID)
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var selected domain.EPUBSelectedUnitReference
		if err = rows.Scan(&selected.UnitID, &selected.Order); err != nil {
			return domain.EPUBReviewedScopeSnapshot{}, err
		}
		scope.SelectedUnits = append(scope.SelectedUnits, selected)
	}
	if err = rows.Err(); err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	return scope, nil
}

// FindFullBookScope returns an existing immutable scope selecting every readable unit in a snapshot.
func (s *PostgresStore) FindFullBookScope(ctx context.Context, owner, sourceID, snapshotID string) (domain.EPUBReviewedScopeSnapshot, error) {
	_, units, err := s.GetExtractedUnitSnapshot(ctx, owner, sourceID)
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT scope_id::text FROM epub_reviewed_scopes WHERE owner_id=$1 AND source_material_id=$2 AND snapshot_id=$3 ORDER BY created_at`, owner, sourceID, snapshotID)
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return domain.EPUBReviewedScopeSnapshot{}, err
		}
		scope, getErr := s.GetEPUBReviewedScope(ctx, owner, sourceID, id)
		if getErr != nil {
			return domain.EPUBReviewedScopeSnapshot{}, getErr
		}
		expected := make([]domain.EPUBSelectedUnitReference, 0)
		for _, unit := range units.Units {
			if strings.TrimSpace(unit.Text) != "" {
				expected = append(expected, domain.EPUBSelectedUnitReference{UnitID: unit.ID, Order: unit.Order})
			}
		}
		if sameSelected(scope.SelectedUnits, expected) {
			return scope, nil
		}
	}
	return domain.EPUBReviewedScopeSnapshot{}, ErrNotFound
}
func sameSelected(a, b []domain.EPUBSelectedUnitReference) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// CreateEPUBReviewedScope validates trusted identities and stores an immutable scope.
func (s *PostgresStore) CreateEPUBReviewedScope(ctx context.Context, scope domain.EPUBReviewedScopeSnapshot) (domain.EPUBReviewedScopeSnapshot, error) {
	source, err := s.GetSourceMaterial(ctx, scope.OwnerID, scope.SourceMaterialID)
	if errors.Is(err, ErrNotFound) {
		return domain.EPUBReviewedScopeSnapshot{}, domain.ErrEPUBReviewedScopeUnavailable
	}
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	snapshotID, units, err := s.GetExtractedUnitSnapshot(ctx, scope.OwnerID, scope.SourceMaterialID)
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) {
		return domain.EPUBReviewedScopeSnapshot{}, domain.ErrEPUBReviewedScopeUnavailable
	}
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	sourceSnapshot := domain.EPUBScopeSourceSnapshot{OwnerID: scope.OwnerID, SourceMaterialID: scope.SourceMaterialID, SourceContent: domain.EPUBContentRevisionIdentity{RevisionID: source.ContentRevisionID, Digest: source.ContentDigest, DigestVersion: source.ContentDigestVersion}, SnapshotID: snapshotID, ExtractedUnits: units}
	if scope.SourceContent.RevisionID == "" {
		scope.SourceContent = sourceSnapshot.SourceContent
	}
	if err = scope.ValidateAgainst(sourceSnapshot); err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	var currentSnapshot, currentRevision string
	if err = tx.QueryRow(ctx, `SELECT current_snapshot_id::text,current_content_revision_id::text FROM source_materials WHERE owner_id=$1 AND id=$2 FOR SHARE`, scope.OwnerID, scope.SourceMaterialID).Scan(&currentSnapshot, &currentRevision); errors.Is(err, pgx.ErrNoRows) {
		return domain.EPUBReviewedScopeSnapshot{}, domain.ErrEPUBReviewedScopeUnavailable
	}
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	if currentSnapshot != scope.SourceUnitSnapshot.SnapshotID || currentRevision != scope.SourceContent.RevisionID {
		return domain.EPUBReviewedScopeSnapshot{}, domain.ErrEPUBReviewedScopeUnavailable
	}
	if err = tx.QueryRow(ctx, `INSERT INTO epub_reviewed_scopes(scope_id,owner_id,source_material_id,content_revision_id,snapshot_id,schema_version,extracted_units_schema_version) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`, scope.ScopeID, scope.OwnerID, scope.SourceMaterialID, scope.SourceContent.RevisionID, scope.SourceUnitSnapshot.SnapshotID, scope.SchemaVersion, scope.SourceUnitSnapshot.ExtractedUnitsSchemaVersion).Scan(&scope.CreatedAt); err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, fmt.Errorf("persist reviewed scope: %w", err)
	}
	for _, selected := range scope.SelectedUnits {
		if _, err = tx.Exec(ctx, `INSERT INTO epub_reviewed_scope_units(scope_id,owner_id,source_material_id,snapshot_id,unit_id,unit_order) VALUES($1,$2,$3,$4,$5,$6)`, scope.ScopeID, scope.OwnerID, scope.SourceMaterialID, scope.SourceUnitSnapshot.SnapshotID, selected.UnitID, selected.Order); err != nil {
			return domain.EPUBReviewedScopeSnapshot{}, fmt.Errorf("persist reviewed scope unit: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	return scope, nil
}
