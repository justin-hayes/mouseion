-- Development convenience only; not a supported production rollback (ADR 0070).
-- Per-Book image bytes are not restored.
DROP VIEW public.my_books_evidence;
DROP VIEW public.book_cover_states;
ALTER TABLE public.book_covers
    DROP CONSTRAINT book_covers_selected_entry_fkey,
    ALTER COLUMN selected_connection_id DROP NOT NULL,
    ALTER COLUMN selected_source_identifier DROP NOT NULL,
    ADD COLUMN state text NOT NULL DEFAULT 'none',
    ADD COLUMN media_type text,
    ADD COLUMN width integer,
    ADD COLUMN height integer,
    ADD COLUMN content_hash text,
    ADD COLUMN bytes bytea,
    ADD COLUMN advertised_at timestamptz,
    ADD COLUMN fetched_at timestamptz,
    ADD COLUMN failure_reason text;
DROP INDEX public.book_covers_selected_entry_idx;
UPDATE public.book_covers SET state = 'unavailable';
ALTER TABLE public.book_covers
    ADD CONSTRAINT book_covers_state_check CHECK (state = ANY (ARRAY['none'::text, 'pending'::text, 'available'::text, 'unavailable'::text])),
    ADD CONSTRAINT book_covers_state_data_check CHECK ((state = 'available' AND bytes IS NOT NULL) OR (state <> 'available' AND bytes IS NULL));
CREATE TABLE public.book_cover_candidates (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    source_identifier text NOT NULL,
    advertised_at timestamptz NOT NULL,
    state text NOT NULL,
    failure_reason text,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT book_cover_candidates_pkey PRIMARY KEY (owner_id, book_id, connection_id, source_identifier, advertised_at),
    CONSTRAINT book_cover_candidates_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT book_cover_candidates_state_check CHECK (state = ANY (ARRAY['pending'::text, 'succeeded'::text, 'failed'::text]))
);

CREATE INDEX book_cover_candidates_pending_idx
    ON public.book_cover_candidates (owner_id, book_id, advertised_at)
    WHERE state = 'pending';

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
    COALESCE(sme.analysis_job_id, (0)::bigint) AS analysis_job_id,
    COALESCE(b.author, ''::text) AS book_author,
    COALESCE(bc.state, 'none'::text) AS book_cover_state,
    COALESCE(bc.width, 0) AS book_cover_width,
    COALESCE(bc.height, 0) AS book_cover_height
   FROM public.books b
     JOIN public.book_membership m ON ((m.owner_id = b.owner_id) AND (m.book_id = b.id) AND (m.state = 'active'::text))
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
            source_material_evidence.book_author,
            row_number() OVER (PARTITION BY source_material_evidence.source_owner_id, source_material_evidence.book_id ORDER BY source_material_evidence.is_current_analysis DESC, source_material_evidence.source_created_at DESC, source_material_evidence.source_id DESC) AS source_rn
           FROM public.source_material_evidence) sme ON (sme.source_owner_id = (b.owner_id)::text AND sme.book_id = (b.id)::text AND sme.source_rn = 1)
     LEFT JOIN public.book_covers bc ON (bc.owner_id = b.owner_id AND bc.book_id = b.id);
