-- The cleanup that ADR 0121 keeps apart from every feature change. Issue #2558
-- is the separately reviewed deletion: book_preparation_failures replaced
-- classical_preparation_failures and epub_acquisition_failures (migration 57,
-- with every row copied unchanged by the migration 58 backfill), and an
-- analysis failure is a stable code on the run and attempt (migration 59)
-- instead of a stored sentence.
--
-- What this destroys, and only this:
-- * every row of classical_preparation_failures and
--   epub_acquisition_failures. The backfill already copied each row into
--   book_preparation_failures, which stays; the old rows themselves are
--   destroyed.
-- * the error text in analysis_runs.last_error, analysis_run_attempts.error
--   and analysis_jobs.error. Analysis stopped writing them in #2553; the
--   learner-facing sentence is derived from the code, which the evidence read
--   models project through latest_analysis_failure.
--
-- source_material_evidence reads analysis_jobs.error to classify a legacy
-- run-less job as failed, so the view is rewritten first, in the same
-- migration the column dies in: a job without a run is queued evidence, and
-- failure reads the run's state, with the code still carried on the run and
-- attempt and projected through latest_analysis_failure. A job whose failure
-- only ever lived in the dropped text classifies as queued from now on, and
-- the evidence view no longer produces job-failed signals.
--
-- Following migration 56's precedent, the maintainer repeats these
-- deployment checks immediately before merge:
--   SELECT count(*) FROM analysis_jobs
--    WHERE error <> '' AND analysis_run_id IS NULL;
--   SELECT count(*) FROM classical_preparation_failures;
--   SELECT count(*) FROM epub_acquisition_failures;
-- The first must be 0; the last two must each match the rows the backfill
-- already copied into book_preparation_failures, which this migration keeps.
--
-- Recovery: the usual pre-migration database backup is the only path back to
-- the dropped rows and text. The down migration is a development convenience
-- that recreates the empty tables, the empty columns and the error-reading
-- view; it restores no data.
--
-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
CREATE OR REPLACE VIEW public.source_material_evidence AS
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
            WHEN (j.river_job_id IS NOT NULL) THEN 'analyzing'::text
            ELSE 'not analyzed'::text
        END AS analysis_status,
        CASE
            WHEN (ar.state IS NOT NULL) THEN ar.state
            WHEN (ca.analysis_run_id IS NOT NULL) THEN 'completed'::text
            WHEN (j.river_job_id IS NOT NULL) THEN 'queued'::text
            ELSE ''::text
        END AS analysis_state,
    COALESCE((ca.analysis_run_id)::text, ''::text) AS analysis_run_id,
    COALESCE((ca.corpus_id)::text, ''::text) AS corpus_id,
    COALESCE(j.river_job_id, (0)::bigint) AS analysis_job_id,
    (p.source_material_id IS NOT NULL) AS is_current_analysis,
    COALESCE(b.author, ''::text) AS book_author
   FROM ((((((public.source_materials s
     LEFT JOIN public.books b ON (((b.owner_id = s.owner_id) AND (b.id = s.book_id))))
     LEFT JOIN public.source_content_revisions r ON (((r.owner_id = s.owner_id) AND (r.revision_id = s.current_content_revision_id))))
     LEFT JOIN public.book_current_analyses p ON (((p.owner_id = s.owner_id) AND (p.source_material_id = s.id))))
     LEFT JOIN public.current_analysis_identity ca ON (((ca.owner_id = p.owner_id) AND (ca.source_material_id = p.source_material_id))))
     LEFT JOIN LATERAL ( SELECT analysis_jobs.river_job_id,
            analysis_jobs.analysis_run_id
           FROM public.analysis_jobs
          WHERE ((analysis_jobs.owner_id = s.owner_id) AND (analysis_jobs.source_material_id = s.id))
          ORDER BY analysis_jobs.created_at DESC, analysis_jobs.river_job_id DESC
         LIMIT 1) j ON (true))
     LEFT JOIN public.analysis_runs ar ON (((ar.owner_id = s.owner_id) AND (ar.id = j.analysis_run_id))));

DROP TABLE public.classical_preparation_failures;
DROP TABLE public.epub_acquisition_failures;

ALTER TABLE public.analysis_runs
    DROP COLUMN last_error;
ALTER TABLE public.analysis_run_attempts
    DROP COLUMN error;
ALTER TABLE public.analysis_jobs
    DROP COLUMN error;
