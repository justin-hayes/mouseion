-- Development-only inverse. Production rollback must restore a backup because
-- this removes durable snapshot provenance retained by the feature contract.
ALTER TABLE public.primary_goals DROP CONSTRAINT primary_goals_snapshot_fkey;
ALTER TABLE public.deck_preparations DROP CONSTRAINT deck_preparations_goal_snapshot_fkey;
ALTER TABLE public.deck_preparations DROP COLUMN goal_snapshot_id;
CREATE UNIQUE INDEX deck_preparations_one_studying_per_owner ON public.deck_preparations USING btree (owner_id) WHERE (studying_at IS NOT NULL);
DROP TABLE public.primary_goal_snapshot_vocabulary;
DROP TABLE public.primary_goal_snapshots;
ALTER TABLE public.primary_goals DROP COLUMN snapshot_id;
