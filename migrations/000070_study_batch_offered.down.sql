-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
ALTER TABLE public.study_batch_ledger DROP CONSTRAINT study_batch_ledger_offered_check;
ALTER TABLE public.study_batch_ledger DROP COLUMN batches_offered;
