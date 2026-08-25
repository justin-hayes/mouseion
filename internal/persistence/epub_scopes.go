package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

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
