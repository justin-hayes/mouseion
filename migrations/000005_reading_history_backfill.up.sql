-- Data backfill for legacy finished Goals. It is deliberately separate from
-- the structural migration above. The owner/language/book key and
-- ON CONFLICT clause make this safe to retry; running it again never changes
-- a completion timestamp or vocabulary state. A failed run is recovered by
-- retrying this migration before the cleanup migration is applied.
INSERT INTO public.reading_history(owner_id, language, book_id, completed_at)
SELECT owner_id, language, book_id, reading_finished_at
FROM public.primary_goals
WHERE reading_finished_at IS NOT NULL
ON CONFLICT (owner_id, language, book_id) DO NOTHING;

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
