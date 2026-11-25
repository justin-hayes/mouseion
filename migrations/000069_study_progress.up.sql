-- Study progress, the review log and the per-day batch ledger (ADR 0134, spec
-- #2691). Owned by learner and study language, keyed by the full vocabulary
-- identity including its lexical discriminator. Progress belongs to the word,
-- never to a Book.

-- One row per word in study. Taking a New-word batch introduces its words: a
-- row is created with the introduction time and the learned meaning, the Gloss
-- of the Study material the word was introduced in. The FSRS memory state and
-- due time are updated by every graded review. material_id is the word's
-- current Study material, which a later re-introduction replaces.
-- withdrawn_at is set when a correction leaves the word with no valid
-- occurrence; its progress is kept and it is not due.
CREATE TABLE public.study_progress (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    lexical_discriminator text DEFAULT ''::text NOT NULL,
    material_id uuid NOT NULL,
    introduced_at timestamptz NOT NULL,
    learned_meaning text NOT NULL,
    state text DEFAULT 'new'::text NOT NULL,
    due timestamptz NOT NULL,
    stability double precision DEFAULT 0 NOT NULL,
    difficulty double precision DEFAULT 0 NOT NULL,
    scheduled_days bigint DEFAULT 0 NOT NULL,
    reps bigint DEFAULT 0 NOT NULL,
    lapses bigint DEFAULT 0 NOT NULL,
    remaining_steps integer DEFAULT 0 NOT NULL,
    last_review timestamptz,
    withdrawn_at timestamptz,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT study_progress_pkey PRIMARY KEY (owner_id, id),
    CONSTRAINT study_progress_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_progress_material_fkey FOREIGN KEY (owner_id, material_id) REFERENCES public.study_material(owner_id, id),
    CONSTRAINT study_progress_state_check CHECK ((state = ANY (ARRAY['new'::text, 'learning'::text, 'review'::text, 'relearning'::text]))),
    CONSTRAINT study_progress_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND btrim(upos) <> ''::text)),
    CONSTRAINT study_progress_meaning_check CHECK ((btrim(learned_meaning) <> ''::text)),
    CONSTRAINT study_progress_counts_check CHECK ((scheduled_days >= 0 AND reps >= 0 AND lapses >= 0 AND remaining_steps >= 0))
);

-- A word is in study at most once.
CREATE UNIQUE INDEX study_progress_word_idx
    ON public.study_progress(owner_id, language, canonical_lemma, upos, lexical_discriminator);

-- Due words of a language, in the order Today shows them.
CREATE INDEX study_progress_due_idx
    ON public.study_progress(owner_id, language, due)
    WHERE withdrawn_at IS NULL;

-- Append-only: one row per graded review, enough to replay a word's history
-- under other FSRS parameters. state_before and state_after hold the scheduling
-- state (state, due, stability, difficulty, scheduled days, reps, lapses,
-- remaining steps, last review) either side of the grade. study_day is the
-- Study day (05:00 Europe/Berlin) the review belongs to.
CREATE TABLE public.study_review_log (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    progress_id uuid NOT NULL,
    material_id uuid NOT NULL,
    grade text NOT NULL,
    reviewed_at timestamptz NOT NULL,
    study_day date NOT NULL,
    state_before jsonb NOT NULL,
    state_after jsonb NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT study_review_log_pkey PRIMARY KEY (owner_id, id),
    CONSTRAINT study_review_log_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_review_log_progress_fkey FOREIGN KEY (owner_id, progress_id) REFERENCES public.study_progress(owner_id, id),
    CONSTRAINT study_review_log_material_fkey FOREIGN KEY (owner_id, material_id) REFERENCES public.study_material(owner_id, id),
    CONSTRAINT study_review_log_grade_check CHECK ((grade = ANY (ARRAY['again'::text, 'got-it'::text]))),
    CONSTRAINT study_review_log_state_check CHECK ((jsonb_typeof(state_before) = 'object'::text AND jsonb_typeof(state_after) = 'object'::text))
);

CREATE INDEX study_review_log_progress_idx
    ON public.study_review_log(owner_id, progress_id, reviewed_at);

-- A logged review is never rewritten. Rows leave only with their owner
-- (ON DELETE CASCADE from users), so the guard covers updates.
CREATE FUNCTION public.study_review_log_append_only() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'study_review_log is append-only';
END;
$$;

CREATE TRIGGER study_review_log_append_only
    BEFORE UPDATE ON public.study_review_log
    FOR EACH ROW EXECUTE FUNCTION public.study_review_log_append_only();

-- The per-day batch ledger: the New-word batches the learner has taken and
-- declined on one Study day. Study progress is not changed by declining.
CREATE TABLE public.study_batch_ledger (
    owner_id uuid NOT NULL,
    language text NOT NULL,
    study_day date NOT NULL,
    batches_taken integer DEFAULT 0 NOT NULL,
    batches_declined integer DEFAULT 0 NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT study_batch_ledger_pkey PRIMARY KEY (owner_id, language, study_day),
    CONSTRAINT study_batch_ledger_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_batch_ledger_counts_check CHECK ((batches_taken >= 0 AND batches_declined >= 0))
);

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
