-- Study material (ADR 0134, spec #2691): one record per word of the sentence,
-- contextual Gloss and translation Study shows for it. Owned by learner, study
-- language and the full vocabulary identity, including its lexical
-- discriminator, and keyed by word and evidence.
--
-- A word has at most one current record (superseded_at IS NULL). A later
-- re-introduction after a Lemma correction prepares new material for the same
-- word and supersedes this one, so Study progress, review log and batch ledger
-- tables that later migrations add can reference a record without it being
-- rewritten. The source occurrence (analysis run, corpus, sentence and token)
-- is plain data, not a foreign key: a replaced analysis must not delete the
-- sentence a learner has already been shown.
--
-- state is the preparation state. A pending record has no evidence yet; a ready
-- record carries the full set; an unpreparable record carries only its reason.
-- A provider omission is never recorded: it leaves the record pending and the
-- durable job retries it.
CREATE TABLE public.study_material (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    lexical_discriminator text DEFAULT ''::text NOT NULL,
    state text NOT NULL,
    omission_reason text DEFAULT ''::text NOT NULL,
    book_id uuid,
    analysis_run_id uuid,
    corpus_id uuid,
    sentence_ordinal bigint,
    token_ordinal bigint,
    sentence text DEFAULT ''::text NOT NULL,
    target_word text DEFAULT ''::text NOT NULL,
    gloss text DEFAULT ''::text NOT NULL,
    sentence_translation text DEFAULT ''::text NOT NULL,
    sentence_translation_targets jsonb DEFAULT '[]'::jsonb NOT NULL,
    superseded_at timestamptz,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT study_material_pkey PRIMARY KEY (owner_id, id),
    CONSTRAINT study_material_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_material_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'ready'::text, 'unpreparable'::text]))),
    CONSTRAINT study_material_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND btrim(upos) <> ''::text)),
    CONSTRAINT study_material_omission_check CHECK ((((state = 'unpreparable'::text) AND (btrim(omission_reason) <> ''::text)) OR ((state <> 'unpreparable'::text) AND (omission_reason = ''::text)))),
    CONSTRAINT study_material_ready_check CHECK (((state <> 'ready'::text) OR (book_id IS NOT NULL AND analysis_run_id IS NOT NULL AND corpus_id IS NOT NULL AND sentence_ordinal IS NOT NULL AND token_ordinal IS NOT NULL AND btrim(sentence) <> ''::text AND btrim(target_word) <> ''::text AND btrim(gloss) <> ''::text AND btrim(sentence_translation) <> ''::text))),
    CONSTRAINT study_material_pending_check CHECK (((state = 'ready'::text) OR (book_id IS NULL AND analysis_run_id IS NULL AND corpus_id IS NULL AND sentence_ordinal IS NULL AND token_ordinal IS NULL))),
    CONSTRAINT study_material_targets_check CHECK ((jsonb_typeof(sentence_translation_targets) = 'array'::text))
);

-- One current record per word.
CREATE UNIQUE INDEX study_material_current_word_idx
    ON public.study_material(owner_id, language, canonical_lemma, upos, lexical_discriminator)
    WHERE superseded_at IS NULL;

-- One record per word and evidence.
CREATE UNIQUE INDEX study_material_word_evidence_idx
    ON public.study_material(owner_id, language, canonical_lemma, upos, lexical_discriminator, analysis_run_id, sentence_ordinal, token_ordinal)
    WHERE state = 'ready';

CREATE INDEX study_material_state_idx
    ON public.study_material(owner_id, language, state)
    WHERE superseded_at IS NULL;

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
