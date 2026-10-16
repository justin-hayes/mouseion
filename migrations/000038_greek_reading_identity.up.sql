-- Ancient Greek Reading identity: the lexical discriminator and the frozen
-- citation headword join the current-reading snapshot vocabulary and Known
-- vocabulary, so a Reading of an imported Book freezes and accepts the same
-- discriminated identities Browse already shows.
--
-- The identity shape is settled by the Ancient Greek MVP spec (#2098):
-- "each tuple-keyed table gains [the discriminator] in the ticket that first
-- writes Greek rows to it. The tables are corpus tokens, Browse counts,
-- snapshot vocabulary, Known, ..." and "Migrations are additive successor
-- migrations that rebuild the affected keys without changing any existing
-- value." This is the ticket that first writes Greek rows to the snapshot
-- vocabulary and Known (#2106).
--
-- Every migration here is additive. Existing rows keep the empty discriminator
-- and an empty headword, and the rebuilt keys change no existing value, so
-- modern-language snapshots, Known identities and outcomes are unchanged.

-- The snapshot vocabulary key gains the discriminator, so homographs that share
-- a canonical lemma and part of speech (δέω 1 and δέω 2) stay separate members.
-- The headword is the citation form the snapshot showed; it is display-only and
-- empty for every modern-language row.
ALTER TABLE public.primary_goal_snapshot_vocabulary
    DROP CONSTRAINT primary_goal_snapshot_vocabulary_pkey,
    ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '',
    ADD COLUMN headword text NOT NULL DEFAULT '',
    ADD CONSTRAINT primary_goal_snapshot_vocabulary_pkey PRIMARY KEY (owner_id, snapshot_id, language, canonical_lemma, upos, lexical_discriminator);

-- Known vocabulary gains the discriminator in its unique identity, so a Known
-- δέω with no discriminator never marks δέω 2 Known. Every existing row has the
-- empty discriminator, so the rebuilt key changes no identity.
ALTER TABLE public.known_vocabulary
    DROP CONSTRAINT known_vocabulary_owner_id_language_canonical_lemma_upos_key,
    ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '',
    ADD CONSTRAINT known_vocabulary_owner_id_language_canonical_lemma_upos_key UNIQUE (owner_id, language, canonical_lemma, upos, lexical_discriminator);
