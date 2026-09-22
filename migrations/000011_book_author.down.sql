DROP VIEW public.my_books_evidence;
DROP VIEW public.source_material_evidence;

CREATE VIEW public.source_material_evidence AS
 SELECT (s.id)::text AS source_id,
    (s.owner_id)::text AS source_owner_id,
    s.language AS source_language,
    s.source_identifier,
    s.title AS source_title,
    s.media_type AS source_media_type,
    COALESCE((s.book_id)::text, ''::text) AS book_id,
    COALESCE(b.title, ''::text) AS book_title,
        CASE
            WHEN (r.digest_version = 1) THEN r.content_digest
            ELSE s.content_hash
        END AS content_hash,
    COALESCE(r.content_digest, ''::text) AS content_digest,
    COALESCE((r.revision_id)::text, ''::text) AS content_revision_id,
    COALESCE((s.current_snapshot_id)::text, ''::text) AS content_snapshot_id,
    COALESCE(r.digest_version, 0) AS digest_version,
    s.created_at AS source_created_at,
        CASE
            WHEN (ar.state = ANY (ARRAY['queued'::text, 'running'::text])) THEN 'analyzing'::text
            WHEN (ar.state = 'failed'::text) THEN 'analysis failed'::text
            WHEN (ar.state = 'cancelled'::text) THEN 'analysis cancelled'::text
            WHEN ((p.source_material_id IS NOT NULL) AND (ca.analysis_run_id IS NULL)) THEN 'stale'::text
            WHEN (ca.analysis_run_id IS NOT NULL) THEN 'analyzed'::text
            WHEN ((j.river_job_id IS NOT NULL) AND (j.error = ''::text)) THEN 'analyzing'::text
            ELSE 'not analyzed'::text
        END AS analysis_status,
        CASE
            WHEN (ar.state IS NOT NULL) THEN ar.state
            WHEN (ca.analysis_run_id IS NOT NULL) THEN 'completed'::text
            WHEN ((j.river_job_id IS NOT NULL) AND (j.error <> ''::text)) THEN 'failed'::text
            WHEN (j.river_job_id IS NOT NULL) THEN 'queued'::text
            ELSE ''::text
        END AS analysis_state,
    COALESCE((ca.analysis_run_id)::text, ''::text) AS analysis_run_id,
    COALESCE((ca.corpus_id)::text, ''::text) AS corpus_id,
    COALESCE(j.river_job_id, (0)::bigint) AS analysis_job_id,
    (p.source_material_id IS NOT NULL) AS is_current_analysis
   FROM ((((((public.source_materials s
     LEFT JOIN public.books b ON (((b.owner_id = s.owner_id) AND (b.id = s.book_id))))
     LEFT JOIN public.source_content_revisions r ON (((r.owner_id = s.owner_id) AND (r.revision_id = s.current_content_revision_id))))
     LEFT JOIN public.book_current_analyses p ON (((p.owner_id = s.owner_id) AND (p.source_material_id = s.id))))
     LEFT JOIN public.current_analysis_identity ca ON (((ca.owner_id = p.owner_id) AND (ca.source_material_id = p.source_material_id))))
     LEFT JOIN LATERAL ( SELECT analysis_jobs.river_job_id,
            analysis_jobs.error,
            analysis_jobs.analysis_run_id
           FROM public.analysis_jobs
          WHERE ((analysis_jobs.owner_id = s.owner_id) AND (analysis_jobs.source_material_id = s.id))
          ORDER BY analysis_jobs.created_at DESC, analysis_jobs.river_job_id DESC
         LIMIT 1) j ON (true))
     LEFT JOIN public.analysis_runs ar ON (((ar.owner_id = s.owner_id) AND (ar.id = j.analysis_run_id))));

CREATE VIEW public.my_books_evidence AS
 SELECT (b.id)::text AS book_id,
    (b.owner_id)::text AS book_owner_id,
    b.title AS book_title,
    b.metadata_provenance AS book_metadata_provenance,
    b.language_state AS book_language_state,
    COALESCE(b.language_tag, ''::text) AS book_language_tag,
    b.created_at AS book_created_at,
    b.updated_at AS book_updated_at,
    COALESCE(sme.source_id, ''::text) AS source_id,
    COALESCE(sme.source_owner_id, ''::text) AS source_owner_id,
    COALESCE(sme.source_language, ''::text) AS source_language,
    COALESCE(sme.source_identifier, ''::text) AS source_identifier,
    COALESCE(sme.source_title, ''::text) AS source_title,
    COALESCE(sme.source_media_type, ''::text) AS source_media_type,
    COALESCE(sme.content_hash, ''::text) AS source_content_hash,
    COALESCE(sme.content_digest, ''::text) AS source_content_digest,
    COALESCE(sme.content_revision_id, ''::text) AS source_content_revision_id,
    COALESCE(sme.content_snapshot_id, ''::text) AS source_content_snapshot_id,
    COALESCE(sme.digest_version, 0) AS source_digest_version,
    sme.source_created_at,
    (sme.source_id IS NOT NULL) AS acquired,
    COALESCE(sme.analysis_status, ''::text) AS analysis_status,
    COALESCE(sme.analysis_state, ''::text) AS analysis_state,
    COALESCE(sme.analysis_run_id, ''::text) AS analysis_run_id,
    COALESCE(sme.corpus_id, ''::text) AS corpus_id,
    COALESCE(sme.analysis_job_id, (0)::bigint) AS analysis_job_id
   FROM ((public.books b
     JOIN public.book_membership m ON (((m.owner_id = b.owner_id) AND (m.book_id = b.id) AND (m.state = 'active'::text))))
     LEFT JOIN ( SELECT source_material_evidence.source_id,
            source_material_evidence.source_owner_id,
            source_material_evidence.source_language,
            source_material_evidence.source_identifier,
            source_material_evidence.source_title,
            source_material_evidence.source_media_type,
            source_material_evidence.book_id,
            source_material_evidence.book_title,
            source_material_evidence.content_hash,
            source_material_evidence.content_digest,
            source_material_evidence.content_revision_id,
            source_material_evidence.content_snapshot_id,
            source_material_evidence.digest_version,
            source_material_evidence.source_created_at,
            source_material_evidence.analysis_status,
            source_material_evidence.analysis_state,
            source_material_evidence.analysis_run_id,
            source_material_evidence.corpus_id,
            source_material_evidence.analysis_job_id,
            source_material_evidence.is_current_analysis,
            row_number() OVER (PARTITION BY source_material_evidence.source_owner_id, source_material_evidence.book_id ORDER BY source_material_evidence.is_current_analysis DESC, source_material_evidence.source_created_at DESC, source_material_evidence.source_id DESC) AS source_rn
           FROM public.source_material_evidence) sme ON (((sme.source_owner_id = (b.owner_id)::text) AND (sme.book_id = (b.id)::text) AND (sme.source_rn = 1))));

ALTER TABLE public.books
    DROP COLUMN author;
