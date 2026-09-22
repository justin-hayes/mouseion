-- Source-material library queries. The current-analysis identity and the
-- analysis status/state classification live in the source_material_evidence
-- view (migration 000067); this query only selects from it, so the status
-- logic is not duplicated here.

-- name: ListSourceMaterials :many
SELECT source_id,
       source_owner_id,
       source_language,
       source_identifier,
       source_title,
       source_media_type,
       book_id,
       book_title,
       content_hash,
       content_digest,
       content_revision_id,
       content_snapshot_id,
       digest_version,
       source_created_at,
       analysis_status,
       analysis_state,
       analysis_run_id,
       corpus_id,
       analysis_job_id,
       book_author
FROM source_material_evidence
WHERE source_owner_id = sqlc.arg('owner')
ORDER BY source_created_at DESC, source_title, source_id;
