-- Forward-only in production (ADR 0038, ADR 0070). This down migration is a
-- development and recovery convenience: it removes exactly what the up
-- migration wrote (the backfill events, their progress and the placeholder
-- material) and nothing else, so a re-run starts clean. It must not be run
-- after a learner has reviewed a migrated word: the review log rows would
-- reference the progress, and the delete fails rather than lose them.
WITH removed_event AS (
    DELETE FROM public.study_already_known_event
     WHERE source = 'backfill'
    RETURNING owner_id, progress_id
), removed_progress AS (
    DELETE FROM public.study_progress p
     USING removed_event e
     WHERE p.owner_id = e.owner_id AND p.id = e.progress_id
    RETURNING p.id
)
DELETE FROM public.study_material
 WHERE omission_reason = 'known_vocabulary_backfill'
   AND (SELECT count(*) FROM removed_progress) >= 0;
