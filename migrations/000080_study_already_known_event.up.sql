-- The Already-known event (ADR 0136 §1, spec #2727, ticket #2730). Answering
-- Already known on an introduction review is an answer, never a grade, so it
-- cannot live in study_review_log (graded-only, grade CHECK). It is its own
-- append-only table, keyed like Study progress by the Study item: kind,
-- language, canonical lemma, UPOS and lexical discriminator. Idioms and the
-- Known-vocabulary backfill write to it as well; source says which.
--
-- Additive structural DDL only: a new table, no change to an existing one and
-- no data backfill. Forward-only in production (ADR 0038, ADR 0070).
CREATE TABLE public.study_already_known_event (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    item_kind text DEFAULT 'word'::text NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text DEFAULT ''::text NOT NULL,
    lexical_discriminator text DEFAULT ''::text NOT NULL,
    progress_id uuid,
    material_id uuid,
    source text NOT NULL,
    answered_at timestamptz NOT NULL,
    study_day date NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT study_already_known_event_pkey PRIMARY KEY (owner_id, id),
    CONSTRAINT study_already_known_event_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_already_known_event_progress_fkey FOREIGN KEY (owner_id, progress_id, item_kind) REFERENCES public.study_progress(owner_id, id, item_kind),
    CONSTRAINT study_already_known_event_material_fkey FOREIGN KEY (owner_id, material_id, item_kind) REFERENCES public.study_material(owner_id, id, item_kind),
    CONSTRAINT study_already_known_event_kind_check CHECK ((item_kind = ANY (ARRAY['word'::text, 'idiom'::text]))),
    CONSTRAINT study_already_known_event_source_check CHECK ((source = ANY (ARRAY['introduction'::text, 'backfill'::text]))),
    CONSTRAINT study_already_known_event_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND ((item_kind = 'word'::text AND btrim(upos) <> ''::text) OR (item_kind = 'idiom'::text AND upos = ''::text AND lexical_discriminator = ''::text))))
);

CREATE INDEX study_already_known_event_item_idx
    ON public.study_already_known_event(owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, answered_at);
