-- PostgreSQL has enforced this check on every new write since the baseline
-- added it NOT VALID, but it has never scanned the rows that predate it. The
-- catalogue alias backfill used to validate it as its last step; that command
-- is retiring and the step never ran in the deployed database, so validate the
-- constraint here (issue #2529). VALIDATE CONSTRAINT takes a SHARE UPDATE
-- EXCLUSIVE lock and scans the table, so it does not block ordinary reads or
-- writes, and re-running it is a no-op.
ALTER TABLE public.book_aliases
    VALIDATE CONSTRAINT book_aliases_connection_contract;

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
