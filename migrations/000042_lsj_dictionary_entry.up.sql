-- LSJ dictionary entry on Ancient Greek prepared-deck items: the matched LSJ
-- entry's native identity and the pinned LSJ revision join the deck manifest
-- item, so a prepared Greek deck never changes when the dictionary index is
-- re-derived (ADR 0101, ticket #2169).
--
-- The freeze is additive and follows 000041_greek_passage_context. Existing
-- rows keep the empty string in both columns, and the manifest digest carries
-- the values only from manifest schema version 11, so modern-language decks and
-- already-prepared Greek decks keep their stored identities.

ALTER TABLE public.deck_preparation_manifest_items
    ADD COLUMN lsj_entry_id text NOT NULL DEFAULT '',
    ADD COLUMN lsj_pin text NOT NULL DEFAULT '';
