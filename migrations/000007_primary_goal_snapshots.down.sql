-- Development-only inverse. Production rollback must restore a backup because
-- this removes durable snapshot provenance retained by the feature contract.
ALTER TABLE public.primary_goals DROP CONSTRAINT primary_goals_snapshot_fkey;
DROP TABLE public.primary_goal_snapshot_vocabulary;
DROP TABLE public.primary_goal_snapshots;
ALTER TABLE public.primary_goals DROP COLUMN snapshot_id;
