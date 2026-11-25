-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
DROP TABLE public.study_fluency_response;
DROP FUNCTION public.study_fluency_response_append_only();
DROP TABLE public.study_fluency_context;
