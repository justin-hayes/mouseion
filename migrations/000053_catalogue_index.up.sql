-- The Catalog index mirrors the entries each OPDS connection offers within the
-- catalog sync scope (ADR 0117). Identity is connection-scoped, matching the
-- catalogue-entry alias identity in ADR 0044, and the row holds no learner
-- state: ownership comes through the connection, so deleting a connection
-- deletes its index.
--
-- title, author, language_tag, cover_href/cover_media_type and epub_href are
-- the metadata acquisition and Catalog browsing read. last_seen_sync lets a
-- complete sync delete entries it no longer saw; a partial or failed run never
-- deletes. An empty cover_href is explicit absence of an advertised cover.
CREATE TABLE public.catalogue_entries (
    connection_id uuid NOT NULL,
    entry_identifier text NOT NULL,
    title text NOT NULL,
    author text DEFAULT ''::text NOT NULL,
    language_tag text NOT NULL,
    cover_href text DEFAULT ''::text NOT NULL,
    cover_media_type text DEFAULT ''::text NOT NULL,
    epub_href text NOT NULL,
    last_seen_sync timestamptz NOT NULL,
    CONSTRAINT catalogue_entries_pkey PRIMARY KEY (connection_id, entry_identifier),
    CONSTRAINT catalogue_entries_connection_fkey FOREIGN KEY (connection_id) REFERENCES public.opds_connections(id) ON DELETE CASCADE,
    CONSTRAINT catalogue_entries_identifier_nonempty CHECK ((btrim(entry_identifier) <> ''::text)),
    CONSTRAINT catalogue_entries_title_nonempty CHECK ((btrim(title) <> ''::text)),
    CONSTRAINT catalogue_entries_language_nonempty CHECK ((btrim(language_tag) <> ''::text)),
    CONSTRAINT catalogue_entries_epub_nonempty CHECK ((btrim(epub_href) <> ''::text))
);

-- Catalog browsing pages one connection's entries in title then author order.
CREATE INDEX catalogue_entries_connection_title_idx
    ON public.catalogue_entries (connection_id, title, author, entry_identifier);

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
