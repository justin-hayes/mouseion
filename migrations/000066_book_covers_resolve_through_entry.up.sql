-- Contract step of the Book cover move (ADR 0130, ADR 0038): a Book's cover now
-- resolves through the Catalog entry it selected, so book_covers shrinks to the
-- selection pointer and the per-Book image bytes, retrieval state, and
-- book_cover_candidates are removed. The data was copied by 000065, which must
-- have run first (same deploy). Forward-only in production (ADR 0070); the down
-- migration is a development convenience.
--
-- Recovery: the file runs as one transaction, so a failure (including the guard
-- below) leaves the schema unchanged, but golang-migrate marks the version
-- dirty: fix the data, run `migrate force 64` (so 000065, which is idempotent,
-- replays and copies any selection a repair changed), then migrate up again.
-- After a successful
-- run the per-Book bytes and book_cover_candidates are gone; the only recovery
-- is a database backup taken before this deploy (take one first), because the
-- images now live only in catalog_entry_covers (copied by 000065). Pending
-- covers are re-retrieved by the next Catalog sync.
-- Guard: an available per-Book image with no selected entry would lose its
-- bytes (it has nowhere to move), so the migration refuses to run instead.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.book_covers
        WHERE state = 'available'
          AND (selected_connection_id IS NULL OR btrim(COALESCE(selected_source_identifier, '')) = '')
    ) THEN
        RAISE EXCEPTION 'book_covers holds available images with no selected Catalog entry; they cannot move to catalog_entry_covers';
    END IF;
    -- Per-selection completion signal of 000065: every selection has its entry row.
    IF EXISTS (
        SELECT 1 FROM public.book_covers bc
        WHERE bc.selected_connection_id IS NOT NULL
          AND btrim(COALESCE(bc.selected_source_identifier, '')) <> ''
          AND NOT EXISTS (
              SELECT 1 FROM public.catalog_entry_covers c
              WHERE c.owner_id = bc.owner_id AND c.connection_id = bc.selected_connection_id
                AND c.entry_identifier = bc.selected_source_identifier)
    ) THEN
        RAISE EXCEPTION 'a book_covers selection has no catalog_entry_covers row; 000065 did not copy it';
    END IF;
END $$;

-- Every Book's presentation state, derived from the selected entry's row. A
-- Book that has not selected an entry yet reports the state of the entries its
-- aliases advertise: pending while any is pending, unavailable when they all
-- failed, none when nothing is advertised.
CREATE VIEW public.book_cover_states AS
 SELECT b.owner_id,
    b.id AS book_id,
    COALESCE(
        CASE WHEN bc.book_id IS NOT NULL THEN sc.state END,
        ( SELECT CASE WHEN bool_or(c.state = 'pending') THEN 'pending' ELSE 'unavailable' END
            FROM public.book_aliases a
              JOIN public.catalog_entry_covers c ON c.owner_id = a.owner_id AND c.connection_id = a.connection_id AND c.entry_identifier = a.value
           WHERE bc.book_id IS NULL AND a.owner_id = b.owner_id AND a.book_id = b.id
             AND a.connection_id IS NOT NULL AND a.namespace = 'source_identifier' AND c.state IN ('pending', 'unavailable')
           HAVING count(*) > 0 ),
        'none') AS state,
    CASE WHEN sc.state = 'available' THEN COALESCE(sc.width, 0) ELSE 0 END AS width,
    CASE WHEN sc.state = 'available' THEN COALESCE(sc.height, 0) ELSE 0 END AS height
   FROM public.books b
     LEFT JOIN public.book_covers bc ON bc.owner_id = b.owner_id AND bc.book_id = b.id
     LEFT JOIN public.catalog_entry_covers sc ON sc.owner_id = bc.owner_id AND sc.connection_id = bc.selected_connection_id AND sc.entry_identifier = bc.selected_source_identifier;

CREATE OR REPLACE VIEW public.my_books_evidence AS
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
    COALESCE(bcs.state, 'none'::text) AS book_cover_state,
    COALESCE(bcs.width, 0) AS book_cover_width,
    COALESCE(bcs.height, 0) AS book_cover_height
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
     LEFT JOIN public.book_cover_states bcs ON (bcs.owner_id = b.owner_id AND bcs.book_id = b.id);

DROP TABLE public.book_cover_candidates;

-- A Book keeps a row only while it selects an entry.
DELETE FROM public.book_covers
WHERE selected_connection_id IS NULL OR btrim(COALESCE(selected_source_identifier, '')) = '';

ALTER TABLE public.book_covers
    DROP CONSTRAINT book_covers_state_data_check,
    DROP CONSTRAINT book_covers_state_check,
    DROP COLUMN state,
    DROP COLUMN media_type,
    DROP COLUMN width,
    DROP COLUMN height,
    DROP COLUMN content_hash,
    DROP COLUMN bytes,
    DROP COLUMN advertised_at,
    DROP COLUMN fetched_at,
    DROP COLUMN failure_reason,
    ALTER COLUMN selected_connection_id SET NOT NULL,
    ALTER COLUMN selected_source_identifier SET NOT NULL;

-- The database refuses to delete an entry image a Book still selects; the
-- application demotes the Book first (explicit absence) or keeps the row.
ALTER TABLE public.book_covers
    ADD CONSTRAINT book_covers_selected_entry_fkey
    FOREIGN KEY (owner_id, selected_connection_id, selected_source_identifier)
    REFERENCES public.catalog_entry_covers (owner_id, connection_id, entry_identifier);

CREATE INDEX book_covers_selected_entry_idx
    ON public.book_covers (owner_id, selected_connection_id, selected_source_identifier);
