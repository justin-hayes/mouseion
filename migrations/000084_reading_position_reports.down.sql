-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback: it discards
-- every Position report, so every Book loses its Reading position.
DROP TABLE public.reading_position_reports;
