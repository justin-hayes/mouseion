-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback: it discards
-- every stored Already-known event.
DROP TABLE public.study_already_known_event;
