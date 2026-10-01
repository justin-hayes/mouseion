CREATE TABLE public.vocabulary_browse_selections (
    owner_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    added_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT vocabulary_browse_selections_identity_key PRIMARY KEY (owner_id, language, canonical_lemma, upos),
    CONSTRAINT vocabulary_browse_selections_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT vocabulary_browse_selections_language_check CHECK (language <> ''),
    CONSTRAINT vocabulary_browse_selections_lemma_check CHECK (canonical_lemma <> ''),
    CONSTRAINT vocabulary_browse_selections_upos_check CHECK (upos IN ('NOUN', 'VERB', 'ADJ', 'ADV'))
);

CREATE TABLE public.custom_vocabulary_decks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL,
    language text NOT NULL,
    name text NOT NULL,
    creation_key uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT custom_vocabulary_decks_owner_creation_key UNIQUE (owner_id, creation_key),
    CONSTRAINT custom_vocabulary_decks_owner_id_key UNIQUE (owner_id, id),
    CONSTRAINT custom_vocabulary_decks_owner_language_id_key UNIQUE (owner_id, language, id),
    CONSTRAINT custom_vocabulary_decks_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT custom_vocabulary_decks_language_check CHECK (language <> ''),
    CONSTRAINT custom_vocabulary_decks_name_check CHECK (btrim(name) <> '' AND char_length(name) <= 120)
);

CREATE TABLE public.custom_vocabulary_deck_identities (
    owner_id uuid NOT NULL,
    deck_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    added_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT custom_vocabulary_deck_identities_key PRIMARY KEY (owner_id, deck_id, canonical_lemma, upos),
    CONSTRAINT custom_vocabulary_deck_identities_language_fkey FOREIGN KEY (owner_id, language, deck_id)
        REFERENCES public.custom_vocabulary_decks(owner_id, language, id) ON DELETE CASCADE,
    CONSTRAINT custom_vocabulary_deck_identities_language_check CHECK (language <> ''),
    CONSTRAINT custom_vocabulary_deck_identities_lemma_check CHECK (canonical_lemma <> ''),
    CONSTRAINT custom_vocabulary_deck_identities_upos_check CHECK (upos IN ('NOUN', 'VERB', 'ADJ', 'ADV'))
);

CREATE INDEX custom_vocabulary_deck_identities_owner_language_idx
    ON public.custom_vocabulary_deck_identities(owner_id, language, deck_id);
