-- Durable, owner- and study-language-scoped reading completions. Rows are
-- append-only application facts; the composite key makes retries idempotent.
CREATE TABLE public.reading_history (
    owner_id uuid NOT NULL,
    language text NOT NULL,
    book_id uuid NOT NULL,
    completed_at timestamptz NOT NULL,
    CONSTRAINT reading_history_language_nonempty CHECK (btrim(language) <> ''),
    CONSTRAINT reading_history_pkey PRIMARY KEY (owner_id, language, book_id),
    CONSTRAINT reading_history_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT reading_history_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE
);
