-- Development-only rollback convenience (ADR 0038, ADR 0070). Production
-- migrations are forward-only; this down is not a supported production
-- rollback.
DROP VIEW public.latest_analysis_failure;

ALTER TABLE public.analysis_run_attempts
    DROP CONSTRAINT analysis_run_attempts_failure_code_check,
    DROP COLUMN failure_code;

ALTER TABLE public.analysis_runs
    DROP CONSTRAINT analysis_runs_failure_code_check,
    DROP COLUMN failure_code;
