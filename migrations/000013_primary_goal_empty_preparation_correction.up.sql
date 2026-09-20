-- Empty legacy preparations cannot be classified after migrations 000008 and
-- 000011: 000008 cleared studying_at while releasing them, and 000011 then
-- treated every completed empty preparation as active. Do not reconstruct that
-- lost learner state. The conservative recovery is to keep the Goal snapshot
-- valid but empty, detach the synthetic repair binding, and retain the deck,
-- generated-vocabulary, and operational history as inactive provenance.
--
-- A backup from before migration 000008 is the only recovery posture that can
-- establish whether one of these rows was genuinely active. The migration is
-- transactional and idempotent; ambiguous rows are intentionally not adopted.
-- Ownership and observation belong to the release operator. The expected
-- impact is zero snapshot-vocabulary rows for each repaired legacy empty
-- preparation, a NULL Goal binding, and a release timestamp while artifacts
-- and provenance counts remain unchanged. A non-zero migration result or a post-migration
-- count that violates those expectations is a failure; recovery is restoring
-- the pre-migration backup and correcting the cause before retrying this
-- forward migration, not manually editing partially repaired rows.
-- The migration runner serializes this transaction with application migration
-- work. A retry after rollback repeats the same guarded cleanup; duplicate work
-- is prevented by the legacy preparation/snapshot identity and is harmless.
-- Unbound rows are cleaned only when their preparation predates the snapshot
-- and the source language agrees with the Goal language. Other ambiguous rows
-- are deliberately left alone because their vocabulary source is unknowable.
WITH ambiguous_empty_preparations AS (
    SELECT DISTINCT
           p.owner_id,
           p.id AS preparation_id,
           g.snapshot_id
    FROM public.deck_preparations p
    JOIN public.current_analysis_identity ca
      ON ca.owner_id = p.owner_id
     AND ca.book_id = p.book_id
     AND ca.source_material_id = p.source_material_id
     AND ca.analysis_run_id = p.analysis_run_id
    JOIN public.source_materials sm
      ON sm.owner_id = p.owner_id
     AND sm.id = p.source_material_id
    JOIN public.primary_goals g
      ON g.owner_id = p.owner_id
     AND g.book_id = p.book_id
     AND g.language = sm.language
     AND g.snapshot_id IS NOT NULL
    JOIN public.primary_goal_snapshots ps
      ON ps.owner_id = g.owner_id
     AND ps.id = g.snapshot_id
    WHERE p.state = 'ready'
      AND p.completed_at IS NOT NULL
      AND p.retired_at IS NULL
      AND p.graduated_at IS NULL
      AND NOT EXISTS (
          SELECT 1
          FROM public.deck_preparation_vocabulary dv
          WHERE dv.owner_id = p.owner_id
            AND dv.deck_preparation_id = p.id
            AND dv.graduated_at IS NULL
      )
      AND (p.goal_snapshot_id IS NULL OR p.goal_snapshot_id = g.snapshot_id)
      AND p.created_at < ps.created_at
),
deleted_snapshot_vocabulary AS (
    DELETE FROM public.primary_goal_snapshot_vocabulary v
    USING ambiguous_empty_preparations p
    WHERE v.owner_id = p.owner_id
      AND v.snapshot_id = p.snapshot_id
    RETURNING v.owner_id, v.snapshot_id
)
UPDATE public.deck_preparations p
SET goal_snapshot_id = NULL,
    studying_at = NULL,
    released_at = COALESCE(p.released_at, now()),
    updated_at = now()
FROM ambiguous_empty_preparations ambiguous
WHERE p.owner_id = ambiguous.owner_id
  AND p.id = ambiguous.preparation_id
  AND p.goal_snapshot_id IS NOT NULL;
