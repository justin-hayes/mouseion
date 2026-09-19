-- Goal completion accepts the frozen snapshot into Known vocabulary. Nullable
-- provenance columns preserve existing explicit/imported rows while making each
-- newly accepted identity independently auditable. The migration is additive;
-- ownership is the application migration owner and retries are safe because no
-- existing rows are rewritten.
ALTER TABLE public.reading_history
    ADD COLUMN goal_snapshot_id uuid,
    ADD COLUMN snapshot_vocabulary_count integer NOT NULL DEFAULT 0,
    ADD COLUMN eligible_vocabulary_count integer NOT NULL DEFAULT 0,
    ADD COLUMN graduated_vocabulary_count integer NOT NULL DEFAULT 0,
    ADD COLUMN already_known_vocabulary_count integer NOT NULL DEFAULT 0,
    ADD CONSTRAINT reading_history_goal_snapshot_fkey
        FOREIGN KEY (owner_id, goal_snapshot_id)
        REFERENCES public.primary_goal_snapshots(owner_id, id),
    ADD CONSTRAINT reading_history_vocabulary_counts_nonnegative CHECK (
        snapshot_vocabulary_count >= 0
        AND eligible_vocabulary_count >= 0
        AND graduated_vocabulary_count >= 0
        AND already_known_vocabulary_count >= 0
        AND eligible_vocabulary_count + already_known_vocabulary_count = snapshot_vocabulary_count
        AND graduated_vocabulary_count <= eligible_vocabulary_count
    );

ALTER TABLE public.known_vocabulary
    ADD COLUMN completion_book_id uuid,
    ADD COLUMN completion_at timestamptz,
    ADD COLUMN completion_goal_snapshot_id uuid,
    ADD COLUMN completion_source_material_id uuid,
    ADD COLUMN completion_analysis_run_id uuid,
    ADD COLUMN completion_content_revision_id uuid,
    ADD COLUMN completion_content_snapshot_id uuid,
    ADD COLUMN completion_corpus_id uuid,
    ADD COLUMN completion_deck_preparation_id uuid,
    ADD COLUMN generated_first_deck_id uuid,
    ADD COLUMN generated_first_source_material_id uuid,
    ADD COLUMN generated_first_at timestamptz,
    ADD CONSTRAINT known_vocabulary_completion_book_fkey
        FOREIGN KEY (owner_id, completion_book_id)
        REFERENCES public.books(owner_id, id),
    ADD CONSTRAINT known_vocabulary_completion_snapshot_fkey
        FOREIGN KEY (owner_id, completion_goal_snapshot_id)
        REFERENCES public.primary_goal_snapshots(owner_id, id),
    ADD CONSTRAINT known_vocabulary_completion_source_fkey
        FOREIGN KEY (owner_id, completion_source_material_id)
        REFERENCES public.source_materials(owner_id, id),
    ADD CONSTRAINT known_vocabulary_completion_revision_fkey
        FOREIGN KEY (owner_id, completion_source_material_id, completion_content_revision_id)
        REFERENCES public.source_content_revisions(owner_id, source_material_id, revision_id),
    ADD CONSTRAINT known_vocabulary_completion_snapshot_content_fkey
        FOREIGN KEY (owner_id, completion_source_material_id, completion_content_snapshot_id)
        REFERENCES public.source_material_unit_snapshots(owner_id, source_material_id, snapshot_id),
    ADD CONSTRAINT known_vocabulary_completion_analysis_fkey
        FOREIGN KEY (owner_id, completion_analysis_run_id, completion_source_material_id)
        REFERENCES public.analysis_runs(owner_id, id, source_material_id),
    ADD CONSTRAINT known_vocabulary_completion_corpus_fkey
        FOREIGN KEY (owner_id, completion_corpus_id)
        REFERENCES public.corpora(owner_id, id),
    ADD CONSTRAINT known_vocabulary_completion_deck_fkey
        FOREIGN KEY (owner_id, completion_deck_preparation_id)
        REFERENCES public.deck_preparations(owner_id, id),
    ADD CONSTRAINT known_vocabulary_generated_deck_fkey
        FOREIGN KEY (owner_id, generated_first_deck_id)
        REFERENCES public.decks(owner_id, id),
    ADD CONSTRAINT known_vocabulary_generated_source_fkey
        FOREIGN KEY (owner_id, generated_first_source_material_id)
        REFERENCES public.source_materials(owner_id, id),
    ADD CONSTRAINT known_vocabulary_completion_provenance_consistent CHECK (
        (completion_book_id IS NULL AND completion_at IS NULL AND completion_goal_snapshot_id IS NULL
         AND completion_source_material_id IS NULL AND completion_analysis_run_id IS NULL
         AND completion_content_revision_id IS NULL AND completion_content_snapshot_id IS NULL
         AND completion_corpus_id IS NULL AND completion_deck_preparation_id IS NULL
         AND generated_first_deck_id IS NULL AND generated_first_source_material_id IS NULL
         AND generated_first_at IS NULL)
        OR (completion_book_id IS NOT NULL AND completion_at IS NOT NULL
            AND completion_goal_snapshot_id IS NOT NULL AND completion_source_material_id IS NOT NULL
            AND completion_analysis_run_id IS NOT NULL AND completion_content_revision_id IS NOT NULL
            AND completion_content_snapshot_id IS NOT NULL AND completion_corpus_id IS NOT NULL)
    );
