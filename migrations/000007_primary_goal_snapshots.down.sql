ALTER TABLE public.primary_goals DROP CONSTRAINT primary_goals_snapshot_fkey;
DROP TABLE public.primary_goal_snapshot_vocabulary;
DROP TABLE public.primary_goal_snapshots;
ALTER TABLE public.primary_goals DROP COLUMN snapshot_id;
