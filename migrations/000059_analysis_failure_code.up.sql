-- An analysis failure is described by a stable code, not a stored sentence
-- (ADR 0121). The run and its attempt carry the same vocabulary the other Book
-- preparation kinds use; the learner-facing sentence is derived from the code
-- where it is shown. The old error-text columns stay in place, unused, until the
-- reviewed cleanup migration.
ALTER TABLE public.analysis_runs
    ADD COLUMN failure_code text,
    ADD CONSTRAINT analysis_runs_failure_code_check CHECK ((failure_code IS NULL OR failure_code = ANY (ARRAY['analysis_failed'::text, 'dependency_parsing_unavailable'::text, 'analysis_interrupted'::text])));

ALTER TABLE public.analysis_run_attempts
    ADD COLUMN failure_code text,
    ADD CONSTRAINT analysis_run_attempts_failure_code_check CHECK ((failure_code IS NULL OR failure_code = ANY (ARRAY['analysis_failed'::text, 'dependency_parsing_unavailable'::text, 'analysis_interrupted'::text])));

-- The Book evidence read models project the failure code from the latest
-- analysis job for the Book's source. A failed run stores the code on the run;
-- a completed run whose finalization failed stores it on its latest attempt.
CREATE VIEW public.latest_analysis_failure AS
 SELECT DISTINCT ON (j.owner_id, j.source_material_id)
    j.owner_id,
    j.source_material_id,
    COALESCE(r.failure_code, ( SELECT a.failure_code
           FROM public.analysis_run_attempts a
          WHERE (a.run_id = r.id)
          ORDER BY a.attempt_number DESC
         LIMIT 1), ''::text) AS failure_code
   FROM (public.analysis_jobs j
     LEFT JOIN public.analysis_runs r ON (((r.owner_id = j.owner_id) AND (r.id = j.analysis_run_id))))
  ORDER BY j.owner_id, j.source_material_id, j.created_at DESC, j.river_job_id DESC;
