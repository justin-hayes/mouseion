-- EPUB acquisition failures are recorded per Book, never per attempt
-- (ADR 0117). A failed acquisition keeps the learner's To Read choice and the
-- reason stays on My Books until an acquisition succeeds, so the row records
-- the latest failure only. Only stable stages and ASCII codes are stored;
-- source text and catalog content are never written here.
--
-- connection_id and entry_identifier are both null when the failing attempt
-- could not resolve a catalog entry. Otherwise they identify the owner-scoped
-- connection and entry, and the composite foreign keys keep a failure from
-- crossing owners or outliving its connection.
ALTER TABLE public.opds_connections
    ADD CONSTRAINT opds_connections_owner_id_id_key UNIQUE (owner_id, id);

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

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
