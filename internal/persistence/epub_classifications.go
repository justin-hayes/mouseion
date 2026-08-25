package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ReplaceEPUBUnitClassifications atomically replaces one classifier's complete
// result for the exact current extracted-unit snapshot.
func (s *PostgresStore) ReplaceEPUBUnitClassifications(ctx context.Context, owner, sourceID string, classifications []domain.EPUBUnitClassification) (err error) {
	if len(classifications) == 0 {
		return errors.New("persist epub classifications: complete classification set is required")
	}
	first := classifications[0]
	seen := make(map[string]struct{}, len(classifications))
	for i := range classifications {
		c := classifications[i]
		if err := c.Validate(); err != nil {
			return fmt.Errorf("persist epub classification %d: %w", i, err)
		}
		if c.Classifier != first.Classifier || c.SourceUnitSnapshot.SnapshotID != first.SourceUnitSnapshot.SnapshotID || c.SourceUnitSnapshot.ExtractedUnitsSchemaVersion != first.SourceUnitSnapshot.ExtractedUnitsSchemaVersion {
			return errors.New("persist epub classifications: mixed snapshot or classifier identity")
		}
		if _, exists := seen[c.SourceUnitSnapshot.UnitID]; exists {
			return fmt.Errorf("persist epub classifications: duplicate unit identity %q", c.SourceUnitSnapshot.UnitID)
		}
		seen[c.SourceUnitSnapshot.UnitID] = struct{}{}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var snapshotID string
	var schemaVersion, unitCount int
	err = tx.QueryRow(ctx, `SELECT snapshot_id,schema_version FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2 FOR UPDATE`, owner, sourceID).Scan(&snapshotID, &schemaVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrEPUBClassificationsUnavailable
	}
	if err != nil {
		return err
	}
	if snapshotID != first.SourceUnitSnapshot.SnapshotID || schemaVersion != first.SourceUnitSnapshot.ExtractedUnitsSchemaVersion {
		return domain.ErrEPUBClassificationsUnavailable
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2`, owner, sourceID).Scan(&unitCount); err != nil {
		return err
	}
	if unitCount != len(classifications) {
		return fmt.Errorf("persist epub classifications: got %d classifications for %d units", len(classifications), unitCount)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM source_material_unit_classification_reasons WHERE owner_id=$1 AND source_material_id=$2 AND snapshot_id=$3 AND classifier_name=$4 AND classifier_version=$5`, owner, sourceID, snapshotID, first.Classifier.Name, first.Classifier.Version); err != nil {
		return err
	}
	for _, c := range classifications {
		_, err = tx.Exec(ctx, `INSERT INTO source_material_unit_classifications(owner_id,source_material_id,snapshot_id,unit_id,schema_version,extracted_units_schema_version,classifier_name,classifier_version,category,confidence,recommended_inclusion) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(owner_id,source_material_id,snapshot_id,classifier_name,classifier_version,unit_id) DO UPDATE SET schema_version=excluded.schema_version,extracted_units_schema_version=excluded.extracted_units_schema_version,category=excluded.category,confidence=excluded.confidence,recommended_inclusion=excluded.recommended_inclusion,updated_at=CASE WHEN (source_material_unit_classifications.schema_version,source_material_unit_classifications.extracted_units_schema_version,source_material_unit_classifications.category,source_material_unit_classifications.confidence,source_material_unit_classifications.recommended_inclusion) IS DISTINCT FROM (excluded.schema_version,excluded.extracted_units_schema_version,excluded.category,excluded.confidence,excluded.recommended_inclusion) THEN now() ELSE source_material_unit_classifications.updated_at END`, owner, sourceID, snapshotID, c.SourceUnitSnapshot.UnitID, c.SchemaVersion, c.SourceUnitSnapshot.ExtractedUnitsSchemaVersion, c.Classifier.Name, c.Classifier.Version, c.Category, c.Confidence, c.RecommendedInclusion)
		if err != nil {
			return err
		}
		for order, reason := range c.Reasons {
			if _, err = tx.Exec(ctx, `INSERT INTO source_material_unit_classification_reasons(owner_id,source_material_id,snapshot_id,classifier_name,classifier_version,unit_id,reason_order,signal,message) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, owner, sourceID, snapshotID, c.Classifier.Name, c.Classifier.Version, c.SourceUnitSnapshot.UnitID, order, reason.Signal, reason.Message); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// GetEPUBUnitClassifications returns a classifier run in extracted-unit order.
func (s *PostgresStore) GetEPUBUnitClassifications(ctx context.Context, owner, sourceID, classifierName, classifierVersion string) ([]domain.EPUBUnitClassification, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.schema_version,c.snapshot_id,c.extracted_units_schema_version,c.unit_id,c.category,c.confidence,c.recommended_inclusion,c.created_at,c.updated_at,r.reason_order,r.signal,r.message FROM source_material_unit_classifications c JOIN source_material_unit_snapshots s ON s.owner_id=c.owner_id AND s.source_material_id=c.source_material_id AND s.snapshot_id=c.snapshot_id JOIN source_material_units u ON u.owner_id=c.owner_id AND u.source_material_id=c.source_material_id AND u.unit_id=c.unit_id JOIN source_material_unit_classification_reasons r ON r.owner_id=c.owner_id AND r.source_material_id=c.source_material_id AND r.snapshot_id=c.snapshot_id AND r.classifier_name=c.classifier_name AND r.classifier_version=c.classifier_version AND r.unit_id=c.unit_id WHERE c.owner_id=$1 AND c.source_material_id=$2 AND c.classifier_name=$3 AND c.classifier_version=$4 ORDER BY u.unit_order,r.reason_order`, owner, sourceID, classifierName, classifierVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.EPUBUnitClassification
	for rows.Next() {
		var c domain.EPUBUnitClassification
		var reason domain.EPUBClassificationReason
		var reasonOrder int
		c.Classifier = domain.EPUBClassifierIdentity{Name: classifierName, Version: classifierVersion}
		if err = rows.Scan(&c.SchemaVersion, &c.SourceUnitSnapshot.SnapshotID, &c.SourceUnitSnapshot.ExtractedUnitsSchemaVersion, &c.SourceUnitSnapshot.UnitID, &c.Category, &c.Confidence, &c.RecommendedInclusion, &c.CreatedAt, &c.UpdatedAt, &reasonOrder, &reason.Signal, &reason.Message); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].SourceUnitSnapshot.UnitID != c.SourceUnitSnapshot.UnitID {
			if reasonOrder != 0 {
				return nil, errors.New("persisted epub classification reasons are not contiguous")
			}
			c.Reasons = []domain.EPUBClassificationReason{reason}
			out = append(out, c)
		} else {
			current := &out[len(out)-1]
			if reasonOrder != len(current.Reasons) {
				return nil, errors.New("persisted epub classification reasons are not contiguous")
			}
			current.Reasons = append(current.Reasons, reason)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, domain.ErrEPUBClassificationsUnavailable
	}
	for i := range out {
		if err = out[i].Validate(); err != nil {
			return nil, fmt.Errorf("validate persisted epub classification: %w", err)
		}
	}
	return out, nil
}
