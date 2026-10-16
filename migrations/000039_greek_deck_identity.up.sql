-- Ancient Greek deck identity: the lexical discriminator and the frozen
-- citation headword join the generated-vocabulary, deck-preparation and deck
-- manifest tables, so a Reading of an imported Book prepares and exports the
-- same discriminated identities Browse and the Reading snapshot already carry.
--
-- The identity shape is settled by the Ancient Greek MVP spec (#2098): "each
-- tuple-keyed table gains [the discriminator] in the ticket that first writes
-- Greek rows to it. The tables are corpus tokens, Browse counts, snapshot
-- vocabulary, Known, and the generated-vocabulary, deck-preparation and
-- deck-manifest tables." This is the ticket that first writes Greek rows to the
-- deck tables (#2110).
--
-- Every migration here is additive. Existing rows keep the empty discriminator
-- and an empty headword, and the rebuilt keys change no existing value, so
-- modern-language decks, generated provenance and outcomes are unchanged.

-- deck_preparation_vocabulary references generated_vocabulary's identity, so
-- the foreign key is dropped before the referenced key is rebuilt and restored
-- afterwards over the extended identity.
ALTER TABLE public.deck_preparation_vocabulary
    DROP CONSTRAINT deck_preparation_vocabulary_owner_id_language_canonical_le_fkey;

-- Generated vocabulary is the durable first-provenance record of one card
-- identity. The discriminator joins its primary key so homographs that share a
-- canonical lemma and part of speech (δέω 1 and δέω 2) keep separate
-- provenance. The headword is the citation form shown to learners and is
-- display-only, so it is not part of the key.
ALTER TABLE public.generated_vocabulary
    DROP CONSTRAINT generated_vocabulary_pkey,
    ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '',
    ADD COLUMN headword text NOT NULL DEFAULT '',
    ADD CONSTRAINT generated_vocabulary_pkey PRIMARY KEY (owner_id, language, canonical_lemma, upos, lexical_discriminator);

-- Deck-preparation vocabulary is the immutable identity snapshot attached to
-- one preparation. It gains the same discriminator in its key and the frozen
-- headword, so a preparation keeps each homograph as its own member.
ALTER TABLE public.deck_preparation_vocabulary
    DROP CONSTRAINT deck_preparation_vocabulary_pkey,
    ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '',
    ADD COLUMN headword text NOT NULL DEFAULT '',
    ADD CONSTRAINT deck_preparation_vocabulary_pkey PRIMARY KEY (owner_id, deck_preparation_id, language, canonical_lemma, upos, lexical_discriminator);

DROP INDEX public.deck_preparation_vocabulary_owner_identity_idx;

CREATE INDEX deck_preparation_vocabulary_owner_identity_idx
    ON public.deck_preparation_vocabulary(owner_id, language, canonical_lemma, upos, lexical_discriminator);

ALTER TABLE ONLY public.deck_preparation_vocabulary
    ADD CONSTRAINT deck_preparation_vocabulary_owner_id_language_canonical_le_fkey FOREIGN KEY (owner_id, language, canonical_lemma, upos, lexical_discriminator) REFERENCES public.generated_vocabulary(owner_id, language, canonical_lemma, upos, lexical_discriminator);

-- The deck manifest item is the frozen selection decision behind one card. It
-- carries the discriminator and the frozen headword directly, so a durable run
-- preserves the exact displayed identity without re-deriving it.
ALTER TABLE public.deck_preparation_manifest_items
    ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '',
    ADD COLUMN headword text NOT NULL DEFAULT '';
