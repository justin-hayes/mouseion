-- Position reports (#2860): where the learner was in a Book, as a fraction of
-- the whole Book, with where the report came from and when it was recorded. A
-- Book's Reading position is derived, never stored: it is the Book's latest
-- report by recorded_at, with the identity breaking a tie so equal times keep
-- insertion order. Reports belong to the Book, not to its Reading membership,
-- so removing a Book from Reading keeps them. Only the learner's own manual
-- entry exists so far; page and page_count are optional and come together.
--
-- Additive structural DDL only: a new table, no change to an existing one and
-- no data backfill. Forward-only in production (ADR 0038, ADR 0070).
CREATE TABLE public.reading_position_reports (
    id bigint GENERATED ALWAYS AS IDENTITY NOT NULL,
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    source text NOT NULL,
    fraction numeric(7,6) NOT NULL,
    page integer,
    page_count integer,
    recorded_at timestamptz NOT NULL,
    CONSTRAINT reading_position_reports_pkey PRIMARY KEY (id),
    CONSTRAINT reading_position_reports_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT reading_position_reports_source_check CHECK (source = 'manual'),
    CONSTRAINT reading_position_reports_fraction_check CHECK (fraction >= 0 AND fraction <= 1),
    CONSTRAINT reading_position_reports_page_check CHECK (
        (page IS NULL AND page_count IS NULL)
        OR (page IS NOT NULL AND page_count IS NOT NULL AND page_count >= 1 AND page >= 0 AND page <= page_count)
    )
);
CREATE INDEX reading_position_reports_latest_idx ON public.reading_position_reports (owner_id, book_id, recorded_at DESC, id DESC);
