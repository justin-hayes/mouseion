-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback: it discards
-- every recorded Fluency scan, so exhausted words are scanned again.
DROP TABLE public.study_fluency_scan;
