-- Fluency contexts and responses (ADR 0134 §7, spec #2691, milestone 2). Owned
-- by learner; a context belongs to the word's Study progress, so it is keyed by
-- the full vocabulary identity through it.

-- Up to five contexts of a mature word: other occurrences of the word in the
-- learner's library, prepared by the study job package. agreement is the
-- provider's answer to whether the occurrence uses the word's learned meaning.
-- A disagreeing context is kept so the occurrence is not asked about again and
-- is never shown; only an agreeing one carries a Gloss and a translation. The
-- source occurrence (analysis run, corpus, sentence and token) is plain data,
-- not a foreign key: a replaced analysis must not delete a sentence the learner
-- has been shown. The form columns record the form the context was chosen for:
-- its surface form, its morphological features, and whether it is a separable
-- verb split from its prefix.
--
-- At most five agreeing contexts per word is enforced by the insert, not by a
-- constraint, because it counts rows.
CREATE TABLE public.study_fluency_context (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    progress_id uuid NOT NULL,
    agreement text NOT NULL,
    book_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    sentence_ordinal bigint NOT NULL,
    token_ordinal bigint NOT NULL,
    sentence text NOT NULL,
    target_word text NOT NULL,
    gloss text DEFAULT ''::text NOT NULL,
    sentence_translation text DEFAULT ''::text NOT NULL,
    sentence_translation_targets jsonb DEFAULT '[]'::jsonb NOT NULL,
    form_surface text DEFAULT ''::text NOT NULL,
    form_features text DEFAULT ''::text NOT NULL,
    form_split boolean DEFAULT false NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT study_fluency_context_pkey PRIMARY KEY (owner_id, id),
    CONSTRAINT study_fluency_context_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_fluency_context_progress_fkey FOREIGN KEY (owner_id, progress_id) REFERENCES public.study_progress(owner_id, id),
    CONSTRAINT study_fluency_context_agreement_check CHECK ((agreement = ANY (ARRAY['agrees'::text, 'disagrees'::text]))),
    CONSTRAINT study_fluency_context_sentence_check CHECK ((btrim(sentence) <> ''::text AND btrim(target_word) <> ''::text)),
    CONSTRAINT study_fluency_context_agrees_check CHECK (((agreement <> 'agrees'::text) OR (btrim(gloss) <> ''::text AND btrim(sentence_translation) <> ''::text))),
    CONSTRAINT study_fluency_context_targets_check CHECK ((jsonb_typeof(sentence_translation_targets) = 'array'::text))
);

-- An occurrence is assessed once per word.
CREATE UNIQUE INDEX study_fluency_context_occurrence_idx
    ON public.study_fluency_context(owner_id, progress_id, analysis_run_id, sentence_ordinal, token_ordinal);

CREATE INDEX study_fluency_context_progress_idx
    ON public.study_fluency_context(owner_id, progress_id, created_at);

-- Append-only: one row per answer to a Fluency card, Recognised or Didn't
-- recognise it. It is evidence for a later decision on rotating sentences in
-- Today and never changes Study progress or its review log.
CREATE TABLE public.study_fluency_response (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    context_id uuid NOT NULL,
    response text NOT NULL,
    responded_at timestamptz NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT study_fluency_response_pkey PRIMARY KEY (owner_id, id),
    CONSTRAINT study_fluency_response_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_fluency_response_context_fkey FOREIGN KEY (owner_id, context_id) REFERENCES public.study_fluency_context(owner_id, id),
    CONSTRAINT study_fluency_response_response_check CHECK ((response = ANY (ARRAY['recognised'::text, 'not-recognised'::text])))
);

CREATE INDEX study_fluency_response_context_idx
    ON public.study_fluency_response(owner_id, context_id, responded_at);

-- A recorded response is never rewritten. Rows leave only with their owner (ON
-- DELETE CASCADE from users), so the guard covers updates.
CREATE FUNCTION public.study_fluency_response_append_only() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'study_fluency_response is append-only';
END;
$$;

CREATE TRIGGER study_fluency_response_append_only
    BEFORE UPDATE ON public.study_fluency_response
    FOR EACH ROW EXECUTE FUNCTION public.study_fluency_response_append_only();

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
