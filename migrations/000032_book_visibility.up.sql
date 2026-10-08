-- Hidden is an owner-and-Book visibility choice that is independent of the
-- Inbox/To Read disposition (ADR 0086). A missing row means the Book is visible
-- at revision 0, so catalog sync, alias reconciliation, and language correction
-- never need to create or reset visibility. The revision orders Hide/Unhide
-- requests; it is never shared with book_dispositions.revision.
-- Structural only: no data is converted or inferred here, and no existing Book
-- becomes Hidden.
CREATE TABLE public.book_visibility (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    hidden boolean NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT book_visibility_pkey PRIMARY KEY (owner_id, book_id),
    CONSTRAINT book_visibility_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT book_visibility_revision_check CHECK (revision > 0)
);
