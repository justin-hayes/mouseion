ALTER TABLE public.known_vocabulary
    DROP CONSTRAINT known_vocabulary_owner_id_language_canonical_lemma_upos_key,
    DROP COLUMN lexical_discriminator,
    ADD CONSTRAINT known_vocabulary_owner_id_language_canonical_lemma_upos_key UNIQUE (owner_id, language, canonical_lemma, upos);

ALTER TABLE public.primary_goal_snapshot_vocabulary
    DROP CONSTRAINT primary_goal_snapshot_vocabulary_pkey,
    DROP COLUMN headword,
    DROP COLUMN lexical_discriminator,
    ADD CONSTRAINT primary_goal_snapshot_vocabulary_pkey PRIMARY KEY (owner_id, snapshot_id, language, canonical_lemma, upos);
