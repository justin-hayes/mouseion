-- Reverse 000040_greek_deck_identity. The discriminator and headword columns
-- are dropped and the original keys restored. Because every migrated row keeps
-- the empty discriminator, collapsing the keys cannot merge two distinct rows.
ALTER TABLE public.deck_preparation_manifest_items
    DROP COLUMN headword,
    DROP COLUMN lexical_discriminator;

ALTER TABLE public.deck_preparation_vocabulary
    DROP CONSTRAINT deck_preparation_vocabulary_owner_id_language_canonical_le_fkey;

DROP INDEX public.deck_preparation_vocabulary_owner_identity_idx;

ALTER TABLE public.deck_preparation_vocabulary
    DROP CONSTRAINT deck_preparation_vocabulary_pkey,
    DROP COLUMN headword,
    DROP COLUMN lexical_discriminator,
    ADD CONSTRAINT deck_preparation_vocabulary_pkey PRIMARY KEY (owner_id, deck_preparation_id, language, canonical_lemma, upos);

-- Restore the referenced key before the foreign key is recreated over it.
ALTER TABLE public.generated_vocabulary
    DROP CONSTRAINT generated_vocabulary_pkey,
    DROP COLUMN headword,
    DROP COLUMN lexical_discriminator,
    ADD CONSTRAINT generated_vocabulary_pkey PRIMARY KEY (owner_id, language, canonical_lemma, upos);

CREATE INDEX deck_preparation_vocabulary_owner_identity_idx
    ON public.deck_preparation_vocabulary(owner_id, language, canonical_lemma, upos);

ALTER TABLE ONLY public.deck_preparation_vocabulary
    ADD CONSTRAINT deck_preparation_vocabulary_owner_id_language_canonical_le_fkey FOREIGN KEY (owner_id, language, canonical_lemma, upos) REFERENCES public.generated_vocabulary(owner_id, language, canonical_lemma, upos);
