-- A Primary Goal owns an immutable recurring-vocabulary snapshot. Snapshot
-- rows are retained after release so deck and vocabulary provenance remains
-- auditable when a Goal is cleared or replaced.
ALTER TABLE public.primary_goals
    ADD COLUMN snapshot_id uuid;

ALTER TABLE public.deck_preparations
    ADD COLUMN goal_snapshot_id uuid;

CREATE TABLE public.primary_goal_snapshots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    language text NOT NULL,
    book_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    content_revision_id uuid NOT NULL,
    content_snapshot_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    released_at timestamptz,
    CONSTRAINT primary_goal_snapshots_pkey PRIMARY KEY (owner_id, id),
    CONSTRAINT primary_goal_snapshots_language_nonempty CHECK (btrim(language) <> ''),
    CONSTRAINT primary_goal_snapshots_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT primary_goal_snapshots_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT primary_goal_snapshots_source_fkey FOREIGN KEY (owner_id, source_material_id) REFERENCES public.source_materials(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT primary_goal_snapshots_analysis_fkey FOREIGN KEY (owner_id, analysis_run_id, source_material_id) REFERENCES public.analysis_runs(owner_id, id, source_material_id) ON DELETE CASCADE,
    CONSTRAINT primary_goal_snapshots_corpus_fkey FOREIGN KEY (owner_id, corpus_id) REFERENCES public.corpora(owner_id, id) ON DELETE CASCADE
);

CREATE TABLE public.primary_goal_snapshot_vocabulary (
    owner_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    corpus_id text NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    occurrence_count integer NOT NULL,
    observed_forms jsonb NOT NULL,
    eligible_sentence_refs jsonb NOT NULL,
    provenance jsonb NOT NULL,
    first_encounter bigint DEFAULT 9223372036854775807 NOT NULL,
    selected_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT primary_goal_snapshot_vocabulary_pkey PRIMARY KEY (owner_id, snapshot_id, language, canonical_lemma, upos),
    CONSTRAINT primary_goal_snapshot_vocabulary_snapshot_fkey FOREIGN KEY (owner_id, snapshot_id) REFERENCES public.primary_goal_snapshots(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT primary_goal_snapshot_vocabulary_occurrence_check CHECK (occurrence_count > 0)
);

ALTER TABLE public.primary_goals
    ADD CONSTRAINT primary_goals_snapshot_fkey FOREIGN KEY (owner_id, snapshot_id) REFERENCES public.primary_goal_snapshots(owner_id, id);

ALTER TABLE public.deck_preparations
    ADD CONSTRAINT deck_preparations_goal_snapshot_fkey FOREIGN KEY (owner_id, goal_snapshot_id) REFERENCES public.primary_goal_snapshots(owner_id, id);
