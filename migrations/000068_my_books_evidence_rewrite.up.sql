-- Rebuild my_books_evidence on top of source_material_evidence so the
-- analysis status/state classification lives in exactly one place. The
-- window function picks the current source per Book (preferring the current
-- analysis pointer, then the most recently created source), and the source
-- evidence columns and status come from the view.
DROP VIEW IF EXISTS my_books_evidence;

CREATE VIEW my_books_evidence AS
SELECT b.id::text AS book_id,
       b.owner_id::text AS book_owner_id,
       b.title AS book_title,
       b.metadata_provenance AS book_metadata_provenance,
       b.language_state AS book_language_state,
       COALESCE(b.language_tag, '') AS book_language_tag,
       b.created_at AS book_created_at,
       b.updated_at AS book_updated_at,
       COALESCE(sme.source_id, '') AS source_id,
       COALESCE(sme.source_owner_id, '') AS source_owner_id,
       COALESCE(sme.source_language, '') AS source_language,
       COALESCE(sme.source_identifier, '') AS source_identifier,
       COALESCE(sme.source_title, '') AS source_title,
       COALESCE(sme.source_media_type, '') AS source_media_type,
       COALESCE(sme.content_hash, '') AS source_content_hash,
       COALESCE(sme.content_digest, '') AS source_content_digest,
       COALESCE(sme.content_revision_id, '') AS source_content_revision_id,
       COALESCE(sme.content_snapshot_id, '') AS source_content_snapshot_id,
       COALESCE(sme.digest_version, 0) AS source_digest_version,
       sme.source_created_at,
       (sme.source_id IS NOT NULL)::boolean AS acquired,
       COALESCE(sme.analysis_status, '') AS analysis_status,
       COALESCE(sme.analysis_state, '') AS analysis_state,
       COALESCE(sme.analysis_run_id, '') AS analysis_run_id,
       COALESCE(sme.corpus_id, '') AS corpus_id,
       COALESCE(sme.analysis_job_id, 0) AS analysis_job_id
FROM books b
JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id AND m.state = 'active'
LEFT JOIN (
  SELECT source_material_evidence.*,
         ROW_NUMBER() OVER (
           PARTITION BY source_owner_id, book_id
           ORDER BY is_current_analysis DESC, source_created_at DESC, source_id DESC
         ) AS source_rn
  FROM source_material_evidence
) sme ON sme.source_owner_id = b.owner_id::text AND sme.book_id = b.id::text AND sme.source_rn = 1;