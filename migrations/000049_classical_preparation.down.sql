-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
DROP TABLE public.classical_prepared_definitions;
DROP TABLE public.classical_sources;
