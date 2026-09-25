CREATE TABLE public.book_dispositions (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    disposition text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT book_dispositions_pkey PRIMARY KEY (owner_id, book_id),
    CONSTRAINT book_dispositions_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT book_dispositions_disposition_check CHECK (disposition = ANY (ARRAY['inbox'::text, 'to_read'::text, 'set_aside'::text]))
);
