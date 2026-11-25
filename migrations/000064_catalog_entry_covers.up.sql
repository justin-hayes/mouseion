-- Retained Catalog covers are keyed by Catalog entry, not by Book (ADR 0130,
-- ADR 0038). The identity (owner_id, connection_id, entry_identifier) is the
-- one catalogue_entries and the catalog-entry alias use (ADR 0044).
--
-- There are deliberately no foreign keys: an image a Book uses must outlive its
-- Catalog connection, so the row references neither opds_connections nor
-- catalogue_entries nor books. Removal is application-driven, in the same
-- transaction as index reconciliation, and every read is scoped by owner_id.
--
-- state is pending, available, or unavailable; no row means the entry
-- advertises no cover. advertised_at is the retrieval-generation fence: a
-- completion whose advertised_at no longer matches is dropped. Structural only:
-- the backfill from book_covers and the contract are separate migrations.
CREATE TABLE public.catalog_entry_covers (
    owner_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    entry_identifier text NOT NULL,
    state text NOT NULL,
    advertised_at timestamptz NOT NULL,
    media_type text,
    width integer,
    height integer,
    content_hash text,
    bytes bytea,
    fetched_at timestamptz,
    failure_reason text,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT catalog_entry_covers_pkey PRIMARY KEY (owner_id, connection_id, entry_identifier),
    CONSTRAINT catalog_entry_covers_identifier_nonempty CHECK ((btrim(entry_identifier) <> ''::text)),
    CONSTRAINT catalog_entry_covers_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'available'::text, 'unavailable'::text]))),
    CONSTRAINT catalog_entry_covers_state_data_check CHECK ((((state = 'available'::text) AND (bytes IS NOT NULL) AND (octet_length(bytes) > 0) AND (btrim(COALESCE(media_type, ''::text)) <> ''::text) AND (width > 0) AND (height > 0) AND (btrim(COALESCE(content_hash, ''::text)) <> ''::text)) OR ((state <> 'available'::text) AND (bytes IS NULL))))
);

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
