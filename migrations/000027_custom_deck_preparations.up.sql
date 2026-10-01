CREATE TABLE public.custom_vocabulary_deck_preparations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL,
    custom_deck_id uuid NOT NULL,
    submission_key uuid NOT NULL,
    language text NOT NULL,
    deck_name text NOT NULL,
    filename text NOT NULL,
    state text NOT NULL DEFAULT 'queued',
    frozen_spec jsonb,
    artifact bytea,
    total_cards integer NOT NULL DEFAULT 0,
    selected_identities integer NOT NULL DEFAULT 0,
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    CONSTRAINT custom_vocabulary_deck_preparations_owner_deck_action_key
        UNIQUE (owner_id, custom_deck_id, submission_key),
    CONSTRAINT custom_vocabulary_deck_preparations_owner_id_key
        UNIQUE (owner_id, id),
    CONSTRAINT custom_vocabulary_deck_preparations_deck_fkey
        FOREIGN KEY (owner_id, custom_deck_id)
        REFERENCES public.custom_vocabulary_decks(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT custom_vocabulary_deck_preparations_state_check
        CHECK (state IN ('queued', 'preparing', 'ready', 'failed', 'cancelled')),
    CONSTRAINT custom_vocabulary_deck_preparations_language_check CHECK (language <> ''),
    CONSTRAINT custom_vocabulary_deck_preparations_deck_name_check CHECK (btrim(deck_name) <> ''),
    CONSTRAINT custom_vocabulary_deck_preparations_filename_check CHECK (filename <> ''),
    CONSTRAINT custom_vocabulary_deck_preparations_counts_check
        CHECK (total_cards >= 0 AND selected_identities >= 0 AND total_cards <= selected_identities),
    CONSTRAINT custom_vocabulary_deck_preparations_ready_check
        CHECK ((state = 'ready') = (artifact IS NOT NULL AND completed_at IS NOT NULL AND total_cards > 0)),
    CONSTRAINT custom_vocabulary_deck_preparations_frozen_spec_check
        CHECK (frozen_spec IS NULL OR jsonb_typeof(frozen_spec) = 'object')
);

CREATE UNIQUE INDEX custom_vocabulary_deck_preparations_one_active
    ON public.custom_vocabulary_deck_preparations(owner_id, custom_deck_id)
    WHERE state IN ('queued', 'preparing');

CREATE INDEX custom_vocabulary_deck_preparations_latest
    ON public.custom_vocabulary_deck_preparations(owner_id, custom_deck_id, created_at DESC, id DESC);

CREATE TABLE public.custom_vocabulary_deck_preparation_identities (
    owner_id uuid NOT NULL,
    preparation_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    CONSTRAINT custom_vocabulary_deck_preparation_identities_key
        PRIMARY KEY (owner_id, preparation_id, canonical_lemma, upos),
    CONSTRAINT custom_vocabulary_deck_preparation_identities_preparation_fkey
        FOREIGN KEY (owner_id, preparation_id)
        REFERENCES public.custom_vocabulary_deck_preparations(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT custom_vocabulary_deck_preparation_identities_language_check CHECK (language <> ''),
    CONSTRAINT custom_vocabulary_deck_preparation_identities_lemma_check CHECK (canonical_lemma <> ''),
    CONSTRAINT custom_vocabulary_deck_preparation_identities_upos_check CHECK (upos IN ('NOUN', 'VERB', 'ADJ', 'ADV'))
);
