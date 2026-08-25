package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// PutSourceMaterialWithExtractedUnits atomically replaces an EPUB source and
// its current extracted-unit snapshot. Existing corpora are deliberately not
// changed; FullText remains the analysis compatibility path.
func (s *PostgresStore) PutSourceMaterialWithExtractedUnits(ctx context.Context, v domain.SourceMaterial, units domain.ExtractedUnits) (out domain.SourceMaterial, err error) {
	if err = units.ValidateOffsets(v.FullText); err != nil {
		return out, fmt.Errorf("validate extracted units: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(owner_id,source_identifier) DO UPDATE SET title=excluded.title,media_type=excluded.media_type,content_hash=excluded.content_hash,content=excluded.content,full_text=excluded.full_text RETURNING id,owner_id,language,source_identifier,title,media_type,content_hash,content,full_text,created_at`, v.OwnerID, v.Language, v.SourceIdentifier, v.Title, v.MediaType, v.ContentHash, v.Content, v.FullText).Scan(&out.ID, &out.OwnerID, &out.Language, &out.SourceIdentifier, &out.Title, &out.MediaType, &out.ContentHash, &out.Content, &out.FullText, &out.CreatedAt)
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, out.OwnerID, out.ID); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO source_material_unit_snapshots(owner_id,source_material_id,schema_version) VALUES($1,$2,$3)`, out.OwnerID, out.ID, units.SchemaVersion); err != nil {
		return out, err
	}
	for _, unit := range units.Units {
		properties, marshalErr := json.Marshal(unit.Properties)
		if marshalErr != nil {
			return out, marshalErr
		}
		navigationLabels, marshalErr := json.Marshal(unit.NavigationLabels)
		if marshalErr != nil {
			return out, marshalErr
		}
		landmarkTypes, marshalErr := json.Marshal(unit.LandmarkTypes)
		if marshalErr != nil {
			return out, marshalErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO source_material_units(owner_id,source_material_id,unit_id,unit_order,spine_index,title,title_source,text,start_offset,end_offset,package_path,manifest_id,source_href,resolved_href,media_type,properties,linear,navigation_labels,landmark_types,selected) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,true)`, out.OwnerID, out.ID, unit.ID, unit.Order, unit.SpineIndex, unit.Title, unit.TitleSource, unit.Text, unit.StartOffset, unit.EndOffset, unit.PackagePath, unit.ManifestID, unit.SourceHref, unit.ResolvedHref, unit.MediaType, properties, unit.Linear, navigationLabels, landmarkTypes)
		if err != nil {
			return out, err
		}
	}
	err = tx.Commit(ctx)
	return out, err
}

// GetExtractedUnits returns the current owner-scoped snapshot in unit order.
// Legacy source materials intentionally return ErrExtractedUnitsUnavailable.
func (s *PostgresStore) GetExtractedUnits(ctx context.Context, owner, sourceID string) (domain.ExtractedUnits, error) {
	var out domain.ExtractedUnits
	var fullText string
	err := s.pool.QueryRow(ctx, `SELECT u.schema_version,s.full_text FROM source_material_unit_snapshots u JOIN source_materials s ON s.owner_id=u.owner_id AND s.id=u.source_material_id WHERE u.owner_id=$1 AND u.source_material_id=$2`, owner, sourceID).Scan(&out.SchemaVersion, &fullText)
	if err == pgx.ErrNoRows {
		return out, domain.ErrExtractedUnitsUnavailable
	}
	if err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT unit_id,unit_order,spine_index,title,title_source,text,start_offset,end_offset,package_path,manifest_id,source_href,resolved_href,media_type,properties,linear,navigation_labels,landmark_types FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2 ORDER BY unit_order`, owner, sourceID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var unit domain.ExtractedUnit
		var properties, navigationLabels, landmarkTypes []byte
		if err = rows.Scan(&unit.ID, &unit.Order, &unit.SpineIndex, &unit.Title, &unit.TitleSource, &unit.Text, &unit.StartOffset, &unit.EndOffset, &unit.PackagePath, &unit.ManifestID, &unit.SourceHref, &unit.ResolvedHref, &unit.MediaType, &properties, &unit.Linear, &navigationLabels, &landmarkTypes); err != nil {
			return out, err
		}
		if err = json.Unmarshal(properties, &unit.Properties); err != nil {
			return out, err
		}
		if err = json.Unmarshal(navigationLabels, &unit.NavigationLabels); err != nil {
			return out, err
		}
		if err = json.Unmarshal(landmarkTypes, &unit.LandmarkTypes); err != nil {
			return out, err
		}
		out.Units = append(out.Units, unit)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if err = out.ValidateOffsets(fullText); err != nil {
		return out, fmt.Errorf("validate persisted extracted units: %w", err)
	}
	return out, nil
}
