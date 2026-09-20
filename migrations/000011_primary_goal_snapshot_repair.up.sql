-- Repair the empty-preparation case from the original snapshot backfill. A
-- preparation with no active vocabulary is still the historical Goal input;
-- its empty snapshot must not be widened from current candidates.
-- Ownership is the application migration runner. The repair is transactional
-- and idempotent: only unbound, completed legacy preparations are selected,
-- and rerunning it finds no rows after they are bound. The unique-Goal guard
-- avoids inventing a language association for an empty preparation. Recovery
-- is a forward fix from backup if the post-migration counts are unexpected;
-- the down migration does not recreate deleted widened candidates.
WITH empty_matches AS (
    SELECT p.owner_id, p.id AS preparation_id, g.snapshot_id
    FROM public.deck_preparations p
    JOIN public.primary_goals g
      ON g.owner_id = p.owner_id AND g.book_id = p.book_id
    JOIN public.current_analysis_identity ca
      ON ca.owner_id = g.owner_id AND ca.book_id = g.book_id
     AND ca.source_material_id = p.source_material_id
     AND ca.analysis_run_id = p.analysis_run_id
    WHERE g.snapshot_id IS NOT NULL
      AND p.goal_snapshot_id IS NULL
      AND p.state = 'ready'
      AND p.completed_at IS NOT NULL
      AND p.retired_at IS NULL
      AND p.graduated_at IS NULL
      AND (
          SELECT count(*)
          FROM public.primary_goals g2
          WHERE g2.owner_id = g.owner_id
            AND g2.book_id = g.book_id
            AND g2.snapshot_id IS NOT NULL
      ) = 1
      AND NOT EXISTS (
          SELECT 1
          FROM public.deck_preparation_vocabulary dv
          WHERE dv.owner_id = p.owner_id
            AND dv.deck_preparation_id = p.id
            AND dv.graduated_at IS NULL
      )
)
DELETE FROM public.primary_goal_snapshot_vocabulary v
USING empty_matches m
WHERE v.owner_id = m.owner_id
  AND v.snapshot_id = m.snapshot_id;

WITH empty_matches AS (
    SELECT p.owner_id, p.id AS preparation_id, g.snapshot_id
    FROM public.deck_preparations p
    JOIN public.primary_goals g
      ON g.owner_id = p.owner_id AND g.book_id = p.book_id
    JOIN public.current_analysis_identity ca
      ON ca.owner_id = g.owner_id AND ca.book_id = g.book_id
     AND ca.source_material_id = p.source_material_id
     AND ca.analysis_run_id = p.analysis_run_id
    WHERE g.snapshot_id IS NOT NULL
      AND p.goal_snapshot_id IS NULL
      AND p.state = 'ready'
      AND p.completed_at IS NOT NULL
      AND p.retired_at IS NULL
      AND p.graduated_at IS NULL
      AND (
          SELECT count(*)
          FROM public.primary_goals g2
          WHERE g2.owner_id = g.owner_id
            AND g2.book_id = g.book_id
            AND g2.snapshot_id IS NOT NULL
      ) = 1
      AND NOT EXISTS (
          SELECT 1
          FROM public.deck_preparation_vocabulary dv
          WHERE dv.owner_id = p.owner_id
            AND dv.deck_preparation_id = p.id
            AND dv.graduated_at IS NULL
      )
)
UPDATE public.deck_preparations p
SET goal_snapshot_id = m.snapshot_id,
    studying_at = COALESCE(p.studying_at, p.created_at),
    released_at = NULL,
    updated_at = now()
FROM empty_matches m
WHERE p.owner_id = m.owner_id
  AND p.id = m.preparation_id;
