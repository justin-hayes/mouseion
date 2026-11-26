-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
DROP TABLE public.study_batch_ledger;
DROP TABLE public.study_review_log;
DROP FUNCTION public.study_review_log_append_only();
DROP TABLE public.study_progress;
