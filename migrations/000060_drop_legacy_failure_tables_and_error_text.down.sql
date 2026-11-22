-- Development convenience only (ADR 0038, ADR 0070): recreate the two dropped
-- tables empty, re-add the three error-text columns empty, and restore the
-- evidence view that classified a run-less job by its stored error text. No
-- rows and no text are restored; the pre-migration database backup is the
-- only recovery path. The opds_connections(owner_id, id) unique constraint
-- from migration 55 is never dropped, so both foreign keys resolve.
ALTER TABLE public.analysis_runs
    ADD COLUMN last_error text DEFAULT ''::text NOT NULL;
ALTER TABLE public.analysis_run_attempts
    ADD COLUMN error text DEFAULT ''::text NOT NULL;
ALTER TABLE public.analysis_jobs
    ADD COLUMN error text DEFAULT ''::text NOT NULL;

CREATE TABLE public.classical_preparation_failures (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    reading_unit_key text NOT NULL,
    definition_revision integer NOT NULL,
    stage text NOT NULL,
    code text NOT NULL,
    failed_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT classical_preparation_failures_pkey PRIMARY KEY (owner_id, book_id),
    CONSTRAINT classical_preparation_failures_revision_check CHECK ((definition_revision > 0)),
    CONSTRAINT classical_preparation_failures_stage_check CHECK ((stage = ANY (ARRAY['fetch'::text, 'import'::text, 'source'::text, 'withdrawn'::text, 'catalog'::text]))),
    CONSTRAINT classical_preparation_failures_code_check CHECK ((code ~ '^[a-z0-9_]+$'::text)),
    CONSTRAINT classical_preparation_failures_unit_key_check CHECK ((btrim(reading_unit_key) <> ''::text)),
    CONSTRAINT classical_preparation_failures_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT classical_preparation_failures_unit_key_fkey FOREIGN KEY (reading_unit_key) REFERENCES public.reading_units(definition_key) ON DELETE RESTRICT
);

CREATE TABLE public.epub_acquisition_failures (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    connection_id uuid,
    entry_identifier text,
    stage text NOT NULL,
    code text NOT NULL,
    failed_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT epub_acquisition_failures_pkey PRIMARY KEY (owner_id, book_id),
    CONSTRAINT epub_acquisition_failures_stage_check CHECK ((stage = ANY (ARRAY['entry'::text, 'fetch'::text, 'source'::text, 'import'::text]))),
    CONSTRAINT epub_acquisition_failures_code_check CHECK ((code ~ '^[a-z0-9_]+$'::text)),
    CONSTRAINT epub_acquisition_failures_entry_pair_check CHECK (((connection_id IS NULL) = (entry_identifier IS NULL))),
    CONSTRAINT epub_acquisition_failures_entry_identifier_check CHECK ((entry_identifier IS NULL OR btrim(entry_identifier) <> ''::text)),
    CONSTRAINT epub_acquisition_failures_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT epub_acquisition_failures_owner_connection_fkey FOREIGN KEY (owner_id, connection_id) REFERENCES public.opds_connections(owner_id, id) ON DELETE CASCADE
);

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
    (p.source_material_id IS NOT NULL) AS is_current_analysis,
    COALESCE(b.author, ''::text) AS book_author
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
