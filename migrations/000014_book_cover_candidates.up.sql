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
