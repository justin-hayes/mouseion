DROP VIEW IF EXISTS my_books_evidence;

-- Restore the original membership-driven definition; it is superseded by
-- 000068 for new installs and left here only for reversible history.
CREATE VIEW my_books_evidence AS
SELECT b.id::text AS book_id,
       b.owner_id::text AS book_owner_id,
       b.title AS book_title,
       b.metadata_provenance AS book_metadata_provenance,
       b.language_state AS book_language_state,
       COALESCE(b.language_tag, '') AS book_language_tag,
       b.created_at AS book_created_at,
       b.updated_at AS book_updated_at,
       COALESCE(s.id::text, '') AS source_id,
       COALESCE(s.owner_id::text, '') AS source_owner_id,
       COALESCE(s.language, '') AS source_language,
       COALESCE(s.source_identifier, '') AS source_identifier,
       COALESCE(s.title, '') AS source_title,
       COALESCE(s.media_type, '') AS source_media_type,
       COALESCE(CASE WHEN r.digest_version = 1 THEN r.content_digest ELSE s.content_hash END, '') AS source_content_hash,
       COALESCE(r.content_digest, '') AS source_content_digest,
       COALESCE(r.revision_id::text, '') AS source_content_revision_id,
       COALESCE(s.current_snapshot_id::text, '') AS source_content_snapshot_id,
       COALESCE(r.digest_version, 0) AS source_digest_version,
       s.created_at AS source_created_at,
       s.id IS NOT NULL AS acquired,
       CASE WHEN ar.state IN ('queued', 'running') THEN 'analyzing'
            WHEN ar.state = 'failed' THEN 'analysis failed'
            WHEN ar.state = 'cancelled' THEN 'analysis cancelled'
            WHEN p.source_material_id IS NOT NULL AND ca.analysis_run_id IS NULL THEN 'stale'
            WHEN ca.analysis_run_id IS NOT NULL THEN 'analyzed'
            WHEN j.river_job_id IS NOT NULL AND j.error = '' THEN 'analyzing'
            ELSE 'not analyzed' END AS analysis_status,
       CASE WHEN ar.state IS NOT NULL THEN ar.state
            WHEN ca.analysis_run_id IS NOT NULL THEN 'completed'
            WHEN j.river_job_id IS NOT NULL AND j.error <> '' THEN 'failed'
            WHEN j.river_job_id IS NOT NULL THEN 'queued'
            ELSE '' END AS analysis_state,
       COALESCE(ca.analysis_run_id::text, '') AS analysis_run_id,
       COALESCE(ca.corpus_id::text, '') AS corpus_id,
       COALESCE(j.river_job_id, 0) AS analysis_job_id
FROM books b
JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id AND m.state = 'active'
LEFT JOIN book_current_analyses p ON p.owner_id = b.owner_id AND p.book_id = b.id
LEFT JOIN (
  SELECT pc.owner_id,
         pc.book_id,
         pc.source_material_id,
         pc.analysis_run_id,
         c.id AS corpus_id
  FROM book_current_analyses pc
  JOIN source_materials source
    ON source.owner_id = pc.owner_id AND source.id = pc.source_material_id
   AND source.book_id = pc.book_id
  JOIN analysis_runs rr
    ON rr.owner_id = pc.owner_id AND rr.id = pc.analysis_run_id
   AND rr.source_material_id = pc.source_material_id AND rr.state = 'completed'
  JOIN corpora c
    ON c.owner_id = rr.owner_id AND c.id = rr.corpus_id
   AND c.source_material_id = rr.source_material_id
   AND c.analysis_run_id = rr.id AND c.status = 'complete'
  WHERE source.current_content_revision_id = rr.content_revision_id
    AND source.current_snapshot_id = rr.snapshot_id
) ca ON ca.owner_id = b.owner_id AND ca.book_id = b.id
LEFT JOIN (
  SELECT src.id,
         src.owner_id,
         src.book_id,
         src.language,
         src.source_identifier,
         src.title,
         src.media_type,
         src.content_hash,
         src.current_content_revision_id,
         src.current_snapshot_id,
         src.created_at,
         ROW_NUMBER() OVER (
           PARTITION BY src.owner_id, src.book_id
           ORDER BY CASE WHEN p2.source_material_id IS NOT NULL AND src.id = p2.source_material_id THEN 0 ELSE 1 END,
                    src.created_at DESC, src.id DESC
         ) AS source_rn
  FROM source_materials src
  LEFT JOIN book_current_analyses p2 ON p2.owner_id = src.owner_id AND p2.book_id = src.book_id
) s ON s.owner_id = b.owner_id AND s.book_id = b.id AND s.source_rn = 1
LEFT JOIN source_content_revisions r ON r.owner_id = s.owner_id AND r.revision_id = s.current_content_revision_id
LEFT JOIN (
  SELECT j2.owner_id,
         j2.source_material_id,
         j2.river_job_id,
         j2.error,
         j2.analysis_run_id,
         ROW_NUMBER() OVER (
           PARTITION BY j2.owner_id, j2.source_material_id
           ORDER BY j2.created_at DESC, j2.river_job_id DESC
         ) AS job_rn
  FROM analysis_jobs j2
) j ON j.owner_id = s.owner_id AND j.source_material_id = s.id AND j.job_rn = 1
LEFT JOIN analysis_runs ar ON ar.owner_id = s.owner_id AND ar.id = j.analysis_run_id;