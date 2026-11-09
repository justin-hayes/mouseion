-- Classical preparation failures are recorded per Book, never per pinned file
-- (ADR 0116). A failure belongs to the Book that hit it, so one failed Book of a
-- Work does not fail its siblings, and the Book can recover on its own evidence.
-- Only stable stages and ASCII codes are stored; source text and Greek are never
-- written here. The current Reading unit's revision is compared with
-- definition_revision, so a newer definition revision clears a terminal source
-- failure without rewriting the row.
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

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
