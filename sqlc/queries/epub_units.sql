-- Immutable source revisions and extracted EPUB-unit snapshots.

-- name: UpsertSourceMaterialForExtractedUnits :one
INSERT INTO source_materials(owner_id, language, source_identifier, title, media_type, content_hash, content, full_text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT(owner_id, source_identifier) DO UPDATE SET
  language = excluded.language,
  title = excluded.title,
  media_type = excluded.media_type
RETURNING id::text, owner_id::text, language, source_identifier, title, media_type,
          content_hash, content, full_text, created_at;

-- name: GetCurrentContentRevisionForUpdate :one
SELECT (COALESCE(current_content_revision_id::text, ''))::text AS current_revision_id
FROM source_materials
WHERE owner_id = $1 AND id = $2
FOR UPDATE;

-- name: GetSourceContentRevisionByDigest :one
SELECT revision_id::text
FROM source_content_revisions
WHERE owner_id = $1 AND source_material_id = $2 AND content_digest = $3;

-- name: InsertSourceContentRevision :one
INSERT INTO source_content_revisions(owner_id, source_material_id, digest_version, content_digest, content, full_text)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING revision_id::text;

-- name: SetCurrentContentRevision :exec
UPDATE source_materials
SET current_content_revision_id = $3
WHERE owner_id = $1 AND id = $2;

-- name: InsertSourceMaterialUnitSnapshot :one
INSERT INTO source_material_unit_snapshots(owner_id, source_material_id, content_revision_id, schema_version)
VALUES ($1, $2, $3, $4)
RETURNING snapshot_id::text;

-- name: SetCurrentSnapshot :exec
UPDATE source_materials
SET current_snapshot_id = $3
WHERE owner_id = $1 AND id = $2;

-- name: InsertSourceMaterialUnit :exec
INSERT INTO source_material_units(owner_id, source_material_id, snapshot_id, unit_id, unit_order, spine_index,
                                  title, title_source, text, start_offset, end_offset, package_path, manifest_id,
                                  source_href, resolved_href, media_type, properties, linear, navigation_labels,
                                  landmark_types, selected)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, true);

-- name: GetCurrentSourceMaterialContent :one
SELECT COALESCE(r.content, s.content) AS content,
       COALESCE(r.full_text, s.full_text) AS full_text,
       (CASE WHEN r.digest_version = 1 THEN r.content_digest ELSE s.content_hash END)::text AS content_hash,
       COALESCE(r.content_digest, '') AS content_digest,
       (COALESCE(r.revision_id::text, ''))::text AS content_revision_id,
       COALESCE(r.digest_version, 0) AS digest_version
FROM source_materials s
LEFT JOIN source_content_revisions r ON r.owner_id = s.owner_id
                                    AND r.revision_id = s.current_content_revision_id
WHERE s.owner_id = $1 AND s.id = $2;

-- name: GetCurrentExtractedUnitSnapshot :one
SELECT u.snapshot_id::text, u.schema_version, r.full_text
FROM source_material_unit_snapshots u
JOIN source_materials s ON s.owner_id = u.owner_id
                        AND s.id = u.source_material_id
                        AND s.current_snapshot_id = u.snapshot_id
JOIN source_content_revisions r ON r.owner_id = u.owner_id
                               AND r.source_material_id = u.source_material_id
                               AND r.revision_id = u.content_revision_id
WHERE u.owner_id = $1 AND u.source_material_id = $2;

-- name: ListCurrentExtractedUnits :many
SELECT u.unit_id, u.unit_order, u.spine_index, u.title, u.title_source, u.text,
       u.start_offset, u.end_offset, u.package_path, u.manifest_id, u.source_href,
       u.resolved_href, u.media_type, u.properties, u.linear, u.navigation_labels,
       u.landmark_types
FROM source_material_units u
JOIN source_materials s ON s.owner_id = u.owner_id
                        AND s.id = u.source_material_id
                        AND s.current_snapshot_id = u.snapshot_id
WHERE u.owner_id = $1 AND u.source_material_id = $2
ORDER BY u.unit_order;
