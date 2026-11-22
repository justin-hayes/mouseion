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
       book_author,
       COALESCE(f.failure_code, '')::text AS analysis_failure_code
FROM source_material_evidence e
LEFT JOIN latest_analysis_failure f ON f.owner_id::text = e.source_owner_id AND f.source_material_id::text = e.source_id
WHERE source_owner_id = sqlc.arg('owner')
ORDER BY source_created_at DESC, source_title, source_id;
