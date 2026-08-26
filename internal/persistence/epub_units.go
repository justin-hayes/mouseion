package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// PutSourceMaterialWithExtractedUnits atomically adds an EPUB content revision
// and extracted snapshot. Existing revisions, units, classifications, and
// scopes are never rewritten.
func (s *PostgresStore) PutSourceMaterialWithExtractedUnits(ctx context.Context, v domain.SourceMaterial, units domain.ExtractedUnits) (out domain.SourceMaterial, err error) {
	if err = units.ValidateOffsets(v.FullText); err != nil {
		return out, fmt.Errorf("validate extracted units: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	digest := v.ContentHash
	digestVersion := 0
	if v.Content != nil {
		sum := sha256.Sum256(v.Content)
		digest = "sha256:" + hex.EncodeToString(sum[:])
		digestVersion = 1
	}
	if digest == "" {
		return out, fmt.Errorf("persist EPUB source: content identity is required")
	}
	if digestVersion == 0 && !strings.HasPrefix(digest, "legacy:") {
		digest = "legacy:" + digest
	}
	storedContent := v.Content
	if storedContent == nil {
		storedContent = []byte{}
	}
	err = tx.QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(owner_id,source_identifier) DO UPDATE SET language=excluded.language,title=excluded.title,media_type=excluded.media_type RETURNING id,owner_id,language,source_identifier,title,media_type,content_hash,content,full_text,created_at`, v.OwnerID, v.Language, v.SourceIdentifier, v.Title, v.MediaType, digest, storedContent, v.FullText).Scan(&out.ID, &out.OwnerID, &out.Language, &out.SourceIdentifier, &out.Title, &out.MediaType, &out.ContentHash, &out.Content, &out.FullText, &out.CreatedAt)
	if err != nil {
		return out, err
	}
	var revisionID, currentRevisionID string
	err = tx.QueryRow(ctx, `SELECT COALESCE(current_content_revision_id::text,'') FROM source_materials WHERE owner_id=$1 AND id=$2 FOR UPDATE`, out.OwnerID, out.ID).Scan(&currentRevisionID)
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT revision_id::text FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2 AND content_digest=$3`, out.OwnerID, out.ID, digest).Scan(&revisionID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO source_content_revisions(owner_id,source_material_id,digest_version,content_digest,content,full_text) VALUES($1,$2,$3,$4,$5,$6) RETURNING revision_id::text`, out.OwnerID, out.ID, digestVersion, digest, storedContent, v.FullText).Scan(&revisionID)
	}
	if err != nil {
		return out, err
	}
	if revisionID != currentRevisionID {
		if _, err = tx.Exec(ctx, `UPDATE source_materials SET current_content_revision_id=$3 WHERE owner_id=$1 AND id=$2`, out.OwnerID, out.ID, revisionID); err != nil {
			return out, err
		}
		var snapshotID string
		err = tx.QueryRow(ctx, `INSERT INTO source_material_unit_snapshots(owner_id,source_material_id,content_revision_id,schema_version) VALUES($1,$2,$3,$4) RETURNING snapshot_id::text`, out.OwnerID, out.ID, revisionID, units.SchemaVersion).Scan(&snapshotID)
		if err != nil {
			return out, err
		}
		if _, err = tx.Exec(ctx, `UPDATE source_materials SET current_snapshot_id=$3 WHERE owner_id=$1 AND id=$2`, out.OwnerID, out.ID, snapshotID); err != nil {
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
			_, err = tx.Exec(ctx, `INSERT INTO source_material_units(owner_id,source_material_id,snapshot_id,unit_id,unit_order,spine_index,title,title_source,text,start_offset,end_offset,package_path,manifest_id,source_href,resolved_href,media_type,properties,linear,navigation_labels,landmark_types,selected) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,true)`, out.OwnerID, out.ID, snapshotID, unit.ID, unit.Order, unit.SpineIndex, unit.Title, unit.TitleSource, unit.Text, unit.StartOffset, unit.EndOffset, unit.PackagePath, unit.ManifestID, unit.SourceHref, unit.ResolvedHref, unit.MediaType, properties, unit.Linear, navigationLabels, landmarkTypes)
			if err != nil {
				return out, err
			}
		}
	}
	var currentContent []byte
	if err = tx.QueryRow(ctx, `SELECT COALESCE(r.content,s.content),COALESCE(r.full_text,s.full_text),CASE WHEN r.digest_version=1 THEN r.content_digest ELSE s.content_hash END,COALESCE(r.content_digest,''),COALESCE(r.revision_id::text,''),COALESCE(r.digest_version,0) FROM source_materials s LEFT JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.revision_id=s.current_content_revision_id WHERE s.owner_id=$1 AND s.id=$2`, out.OwnerID, out.ID).Scan(&currentContent, &out.FullText, &out.ContentHash, &out.ContentDigest, &out.ContentRevisionID, &out.ContentDigestVersion); err != nil {
		return out, err
	}
	out.Content = currentContent
	err = tx.Commit(ctx)
	return out, err
}

// GetExtractedUnits returns the current owner-scoped snapshot in unit order.
// Legacy source materials intentionally return ErrExtractedUnitsUnavailable.
func (s *PostgresStore) GetExtractedUnits(ctx context.Context, owner, sourceID string) (domain.ExtractedUnits, error) {
	_, out, err := s.GetExtractedUnitSnapshot(ctx, owner, sourceID)
	return out, err
}

// GetExtractedUnitSnapshot returns the immutable identity and contents of the
// current owner-scoped extracted-unit snapshot.
func (s *PostgresStore) GetExtractedUnitSnapshot(ctx context.Context, owner, sourceID string) (string, domain.ExtractedUnits, error) {
	var snapshotID string
	var out domain.ExtractedUnits
	var fullText string
	err := s.pool.QueryRow(ctx, `SELECT u.snapshot_id,u.schema_version,r.full_text FROM source_material_unit_snapshots u JOIN source_materials s ON s.owner_id=u.owner_id AND s.id=u.source_material_id AND s.current_snapshot_id=u.snapshot_id JOIN source_content_revisions r ON r.owner_id=u.owner_id AND r.source_material_id=u.source_material_id AND r.revision_id=u.content_revision_id WHERE u.owner_id=$1 AND u.source_material_id=$2`, owner, sourceID).Scan(&snapshotID, &out.SchemaVersion, &fullText)
	if err == pgx.ErrNoRows {
		return "", out, domain.ErrExtractedUnitsUnavailable
	}
	if err != nil {
		return "", out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT u.unit_id,u.unit_order,u.spine_index,u.title,u.title_source,u.text,u.start_offset,u.end_offset,u.package_path,u.manifest_id,u.source_href,u.resolved_href,u.media_type,u.properties,u.linear,u.navigation_labels,u.landmark_types FROM source_material_units u JOIN source_materials s ON s.owner_id=u.owner_id AND s.id=u.source_material_id AND s.current_snapshot_id=u.snapshot_id WHERE u.owner_id=$1 AND u.source_material_id=$2 ORDER BY u.unit_order`, owner, sourceID)
	if err != nil {
		return "", out, err
	}
	defer rows.Close()
	for rows.Next() {
		var unit domain.ExtractedUnit
		var properties, navigationLabels, landmarkTypes []byte
		if err = rows.Scan(&unit.ID, &unit.Order, &unit.SpineIndex, &unit.Title, &unit.TitleSource, &unit.Text, &unit.StartOffset, &unit.EndOffset, &unit.PackagePath, &unit.ManifestID, &unit.SourceHref, &unit.ResolvedHref, &unit.MediaType, &properties, &unit.Linear, &navigationLabels, &landmarkTypes); err != nil {
			return "", out, err
		}
		if err = json.Unmarshal(properties, &unit.Properties); err != nil {
			return "", out, err
		}
		if err = json.Unmarshal(navigationLabels, &unit.NavigationLabels); err != nil {
			return "", out, err
		}
		if err = json.Unmarshal(landmarkTypes, &unit.LandmarkTypes); err != nil {
			return "", out, err
		}
		out.Units = append(out.Units, unit)
	}
	if err = rows.Err(); err != nil {
		return "", out, err
	}
	if err = out.ValidateOffsets(fullText); err != nil {
		return "", out, fmt.Errorf("validate persisted extracted units: %w", err)
	}
	return snapshotID, out, nil
}
