-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback. The old failure
-- tables remain until the reviewed cleanup migration, so their rows are never
-- lost by this down.
DROP TABLE public.book_preparation_failures;
