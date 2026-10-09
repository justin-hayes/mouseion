-- ADR 0086 structural stage for retiring the Set Aside disposition. This
-- migration is structural only: it converts no data, initializes no
-- visibility, and never infers Hidden or future-reading intent.
--
-- The replacement CHECK is NOT VALID so it rejects any new or changed row that
-- is not Inbox/To Read while leaving still-legacy 'set_aside' rows readable for
-- the separate, operator-run data conversion (cmd/bookdispositioncutover).
-- That conversion validates the constraint in the same transaction that maps
-- the last legacy row, so a validated constraint proves no legacy value remains.
ALTER TABLE public.book_dispositions
    DROP CONSTRAINT book_dispositions_disposition_check;
ALTER TABLE public.book_dispositions
    ADD CONSTRAINT book_dispositions_disposition_check
    CHECK (disposition = ANY (ARRAY['inbox'::text, 'to_read'::text])) NOT VALID;

-- Checkpoint/audit of the one data-only conversion. A row exists only when the
-- conversion transaction committed, so committed data and checkpoint agree.
CREATE TABLE public.book_disposition_cutovers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    manifest_sha256 text NOT NULL,
    manifest jsonb NOT NULL,
    converted_dispositions integer NOT NULL,
    initialized_visibility integer NOT NULL,
    completed_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT book_disposition_cutovers_pkey PRIMARY KEY (id),
    CONSTRAINT book_disposition_cutovers_manifest_key UNIQUE (manifest_sha256),
    CONSTRAINT book_disposition_cutovers_counts_check CHECK (converted_dispositions >= 0 AND initialized_visibility >= 0)
);

-- Original values of every row the conversion changed, retained so guarded
-- forward correction can prove a row is unchanged since cutover.
CREATE TABLE public.book_disposition_cutover_changes (
    cutover_id uuid NOT NULL,
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    original_disposition text NOT NULL,
    original_revision bigint NOT NULL,
    original_updated_at timestamptz NOT NULL,
    converted_disposition text NOT NULL,
    converted_revision bigint NOT NULL,
    history_category text NOT NULL,
    corrected_disposition text,
    corrected_at timestamptz,
    CONSTRAINT book_disposition_cutover_changes_pkey PRIMARY KEY (cutover_id, owner_id, book_id),
    CONSTRAINT book_disposition_cutover_changes_cutover_fkey FOREIGN KEY (cutover_id) REFERENCES public.book_disposition_cutovers(id) ON DELETE CASCADE,
    CONSTRAINT book_disposition_cutover_changes_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT book_disposition_cutover_changes_category_check CHECK (history_category = ANY (ARRAY['unread'::text, 'assertion'::text, 'completion'::text, 'mixed'::text])),
    CONSTRAINT book_disposition_cutover_changes_correction_check CHECK ((corrected_disposition IS NULL) = (corrected_at IS NULL))
);

-- A database with no legacy value (a fresh install, for example) needs no data
-- conversion, so its constraint is validated here. This changes no data: when a
-- legacy row exists the constraint stays NOT VALID for the operator-run stage.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM public.book_dispositions WHERE disposition NOT IN ('inbox', 'to_read')) THEN
        ALTER TABLE public.book_dispositions VALIDATE CONSTRAINT book_dispositions_disposition_check;
    END IF;
END
$$;
