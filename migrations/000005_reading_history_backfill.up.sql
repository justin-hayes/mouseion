-- Data backfill for legacy finished Goals. It is deliberately separate from
-- the structural migration above. The owner/language/book key and
-- ON CONFLICT clause make this safe to retry; running it again never changes
-- a completion timestamp or vocabulary state. A failed run is recovered by
-- retrying this migration before the cleanup migration is applied. The
-- migration runner holds its database advisory lock and executes this file as
-- one transaction, so row locks and all three state changes roll back
-- together on failure. Do not run it as an ad-hoc partial script; recovery is
-- to retry the migration, and rollback is to restore the database backup
-- because the copied history is intentionally retained.
INSERT INTO public.reading_history(owner_id, language, book_id, completed_at)
SELECT owner_id, language, book_id, reading_finished_at
FROM public.primary_goals
WHERE reading_finished_at IS NOT NULL
ON CONFLICT (owner_id, language, book_id) DO NOTHING;

UPDATE public.reading_journeys AS journey
SET revision = revision + 1, updated_at = now()
WHERE EXISTS (
    SELECT 1
    FROM public.primary_goals AS goal
    JOIN public.reading_journey_membership AS membership
      ON membership.owner_id = goal.owner_id
     AND membership.language = goal.language
     AND membership.book_id = goal.book_id
    WHERE goal.owner_id = journey.owner_id
      AND goal.language = journey.language
      AND goal.reading_finished_at IS NOT NULL
);

DELETE FROM public.reading_journey_membership AS membership
USING public.primary_goals AS goal
WHERE goal.owner_id = membership.owner_id
  AND goal.language = membership.language
  AND goal.book_id = membership.book_id
  AND goal.reading_finished_at IS NOT NULL;

DELETE FROM public.primary_goals AS goal
USING public.reading_history AS history
WHERE goal.owner_id = history.owner_id
  AND goal.language = history.language
  AND goal.book_id = history.book_id
  AND goal.reading_finished_at IS NOT NULL;

-- Restore contiguous learner order after removing migrated completions. The
-- revision changes only for Journeys that actually lost a member, preserving
-- stale-write protection for clients that read the pre-backfill revision.
WITH ranked AS (
    SELECT owner_id, language, book_id,
           row_number() OVER (PARTITION BY owner_id, language ORDER BY position, created_at, book_id) AS new_position
    FROM public.reading_journey_membership
)
UPDATE public.reading_journey_membership AS membership
SET position = ranked.new_position
FROM ranked
WHERE membership.owner_id = ranked.owner_id
  AND membership.language = ranked.language
  AND membership.book_id = ranked.book_id;
