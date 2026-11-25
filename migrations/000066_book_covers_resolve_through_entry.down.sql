-- Development convenience only; not a supported production rollback (ADR 0070).
-- Per-Book image bytes are not restored.
-- my_books_evidence is rebuilt from its own current definition with
-- book_cover_states swapped for book_covers (which regains state, width, and
-- height below), so the long view body is not restated here. Both views are
-- dropped so older down migrations can drop book_covers.
CREATE TEMPORARY TABLE my_books_evidence_def AS
    SELECT replace(pg_get_viewdef('public.my_books_evidence'::regclass), 'book_cover_states', 'book_covers') AS body;
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

DO $$
DECLARE
    body text;
BEGIN
    SELECT t.body INTO body FROM my_books_evidence_def t;
    EXECUTE 'CREATE VIEW public.my_books_evidence AS ' || body;
END $$;
DROP TABLE my_books_evidence_def;
