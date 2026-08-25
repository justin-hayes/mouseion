package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// GetEPUBReviewedScope reloads one immutable scope through its complete
// owner/source identity. Callers must still compare its snapshot and schema
// identities with the current extracted-unit snapshot before reusing it.
func (s *PostgresStore) GetEPUBReviewedScope(ctx context.Context, owner, sourceID, scopeID string) (domain.EPUBReviewedScopeSnapshot, error) {
	var scope domain.EPUBReviewedScopeSnapshot
	err := s.pool.QueryRow(ctx, `SELECT schema_version,scope_id::text,owner_id::text,source_material_id::text,snapshot_id,extracted_units_schema_version,classifier_name,classifier_version,selection_mode,created_at
		FROM epub_reviewed_scopes WHERE scope_id=$1 AND owner_id=$2 AND source_material_id=$3`, scopeID, owner, sourceID).Scan(
		&scope.SchemaVersion, &scope.ScopeID, &scope.OwnerID, &scope.SourceMaterialID, &scope.SourceUnitSnapshot.SnapshotID,
		&scope.SourceUnitSnapshot.ExtractedUnitsSchemaVersion, &scope.Classifier.Name, &scope.Classifier.Version, &scope.SelectionMode, &scope.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EPUBReviewedScopeSnapshot{}, ErrNotFound
	}
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT unit_id,unit_order FROM epub_reviewed_scope_units
		WHERE scope_id=$1 AND owner_id=$2 AND source_material_id=$3 ORDER BY unit_order`, scopeID, owner, sourceID)
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

// CreateEPUBReviewedScope validates browser-supplied identities against the
// current owner-scoped persisted snapshot and stores a new immutable review.
func (s *PostgresStore) CreateEPUBReviewedScope(ctx context.Context, scope domain.EPUBReviewedScopeSnapshot) (domain.EPUBReviewedScopeSnapshot, error) {
	snapshotID, units, err := s.GetExtractedUnitSnapshot(ctx, scope.OwnerID, scope.SourceMaterialID)
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) {
		return domain.EPUBReviewedScopeSnapshot{}, domain.ErrEPUBReviewedScopeUnavailable
	}
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	source := domain.EPUBScopeSourceSnapshot{OwnerID: scope.OwnerID, SourceMaterialID: scope.SourceMaterialID, SnapshotID: snapshotID, ExtractedUnits: units}
	if err = scope.ValidateAgainst(source); err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	var currentSnapshot string
	if err = tx.QueryRow(ctx, `SELECT snapshot_id FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2 FOR SHARE`, scope.OwnerID, scope.SourceMaterialID).Scan(&currentSnapshot); errors.Is(err, pgx.ErrNoRows) {
		return domain.EPUBReviewedScopeSnapshot{}, domain.ErrEPUBReviewedScopeUnavailable
	}
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	if currentSnapshot != scope.SourceUnitSnapshot.SnapshotID {
		return domain.EPUBReviewedScopeSnapshot{}, domain.ErrEPUBReviewedScopeUnavailable
	}
	err = tx.QueryRow(ctx, `INSERT INTO epub_reviewed_scopes(scope_id,owner_id,source_material_id,snapshot_id,schema_version,extracted_units_schema_version,classifier_name,classifier_version,selection_mode) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING created_at`, scope.ScopeID, scope.OwnerID, scope.SourceMaterialID, scope.SourceUnitSnapshot.SnapshotID, scope.SchemaVersion, scope.SourceUnitSnapshot.ExtractedUnitsSchemaVersion, scope.Classifier.Name, scope.Classifier.Version, scope.SelectionMode).Scan(&scope.CreatedAt)
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, fmt.Errorf("persist reviewed scope: %w", err)
	}
	for _, selected := range scope.SelectedUnits {
		if _, err = tx.Exec(ctx, `INSERT INTO epub_reviewed_scope_units(scope_id,owner_id,source_material_id,unit_id,unit_order) VALUES($1,$2,$3,$4,$5)`, scope.ScopeID, scope.OwnerID, scope.SourceMaterialID, selected.UnitID, selected.Order); err != nil {
			return domain.EPUBReviewedScopeSnapshot{}, fmt.Errorf("persist reviewed scope unit: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	return scope, nil
}
