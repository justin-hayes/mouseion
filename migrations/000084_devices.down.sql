-- Dropping the table discards every issued Device credential; a learner would
-- have to add their Devices again, so the down migration is not assumed safe
-- for destructive production rollback.
DROP TABLE IF EXISTS public.devices;
