-- Seed the successor disposition model from the strongest surviving legacy
-- evidence. This is deliberately separate from migration 15's structural DDL.
-- The migration runner executes this statement transactionally; ON CONFLICT
-- makes a separately retried application preserve any disposition already
-- written by an earlier attempt or by a learner.
-- Ownership is with the release operator; recovery is to retry this migration
-- after a failed transaction, not to run a partial ad-hoc backfill.
INSERT INTO public.book_dispositions (owner_id, book_id, disposition)
SELECT b.owner_id,
       b.id,
       CASE
           WHEN EXISTS (
               SELECT 1
               FROM public.primary_goals AS goal
               WHERE goal.owner_id = b.owner_id
                 AND goal.book_id = b.id
           )
           OR EXISTS (
               SELECT 1
               FROM public.reading_journey_membership AS journey
               WHERE journey.owner_id = b.owner_id
                 AND journey.book_id = b.id
           ) THEN 'to_read'
           WHEN EXISTS (
               SELECT 1
               FROM public.reading_history AS history
               WHERE history.owner_id = b.owner_id
                 AND history.book_id = b.id
           ) THEN 'set_aside'
           WHEN membership.state = 'removed' THEN 'set_aside'
           ELSE 'inbox'
       END
FROM public.books AS b
LEFT JOIN public.book_membership AS membership
  ON membership.owner_id = b.owner_id
 AND membership.book_id = b.id
ON CONFLICT (owner_id, book_id) DO NOTHING;
