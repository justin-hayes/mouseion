-- The per-day batch ledger also counts the New-word batches Today has offered
-- (ADR 0134, spec #2691, #2700). A batch is offered when Today first shows it,
-- so offered is at least taken plus declined; a batch offered and not yet
-- answered is the difference. Existing rows are backfilled to the batches
-- already answered, which is all they can have offered. Additive and
-- non-breaking: one consumer, the Study page.
ALTER TABLE public.study_batch_ledger
    ADD COLUMN batches_offered integer DEFAULT 0 NOT NULL;

UPDATE public.study_batch_ledger SET batches_offered = batches_taken + batches_declined;

ALTER TABLE public.study_batch_ledger
    ADD CONSTRAINT study_batch_ledger_offered_check CHECK ((batches_offered >= batches_taken + batches_declined));

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
