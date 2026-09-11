-- source_material_evidence materializes the per-source library evidence that
-- both My Books and the source library read: current content revision, the
-- current-analysis identity chain (via current_analysis_identity), and the
-- analysis status/state classification. The status logic lives here once and
-- is reused by my_books_evidence; later widening or correction is a new
-- migration per ADR 0038.
CREATE VIEW source_material_evidence AS
SELECT s.id::text AS source_id,
       s.owner_id::text AS source_owner_id,
       s.language AS source_language,
       s.source_identifier AS source_identifier,
       s.title AS source_title,
       s.media_type AS source_media_type,
       (COALESCE(s.book_id::text, ''))::text AS book_id,
       (COALESCE(b.title, ''))::text AS book_title,
       (CASE WHEN r.digest_version = 1 THEN r.content_digest ELSE s.content_hash END)::text AS content_hash,
       (COALESCE(r.content_digest, ''))::text AS content_digest,
       (COALESCE(r.revision_id::text, ''))::text AS content_revision_id,
       (COALESCE(s.current_snapshot_id::text, ''))::text AS content_snapshot_id,
       COALESCE(r.digest_version, 0) AS digest_version,
       s.created_at AS source_created_at,
       (CASE WHEN ar.state IN ('queued', 'running') THEN 'analyzing'
            WHEN ar.state = 'failed' THEN 'analysis failed'
            WHEN ar.state = 'cancelled' THEN 'analysis cancelled'
            WHEN p.source_material_id IS NOT NULL AND ca.analysis_run_id IS NULL THEN 'stale'
            WHEN ca.analysis_run_id IS NOT NULL THEN 'analyzed'
            WHEN j.river_job_id IS NOT NULL AND j.error = '' THEN 'analyzing'
            ELSE 'not analyzed' END)::text AS analysis_status,
       (CASE WHEN ar.state IS NOT NULL THEN ar.state
            WHEN ca.analysis_run_id IS NOT NULL THEN 'completed'
            WHEN j.river_job_id IS NOT NULL AND j.error <> '' THEN 'failed'
            WHEN j.river_job_id IS NOT NULL THEN 'queued'
            ELSE '' END)::text AS analysis_state,
       (COALESCE(ca.analysis_run_id::text, ''))::text AS analysis_run_id,
       (COALESCE(ca.corpus_id::text, ''))::text AS corpus_id,
       COALESCE(j.river_job_id, 0) AS analysis_job_id,
       (p.source_material_id IS NOT NULL)::boolean AS is_current_analysis
FROM source_materials s
LEFT JOIN books b ON b.owner_id = s.owner_id AND b.id = s.book_id
LEFT JOIN source_content_revisions r ON r.owner_id = s.owner_id AND r.revision_id = s.current_content_revision_id
LEFT JOIN book_current_analyses p ON p.owner_id = s.owner_id AND p.source_material_id = s.id
LEFT JOIN current_analysis_identity ca ON ca.owner_id = p.owner_id AND ca.source_material_id = p.source_material_id
LEFT JOIN LATERAL (
  SELECT river_job_id, error, analysis_run_id FROM analysis_jobs
  WHERE owner_id = s.owner_id AND source_material_id = s.id
  ORDER BY created_at DESC, river_job_id DESC LIMIT 1
) j ON true
LEFT JOIN analysis_runs ar ON ar.owner_id = s.owner_id AND ar.id = j.analysis_run_id;