-- Down migrations are a development convenience, not a supported production rollback (ADR 0038, ADR 0070).
DROP TABLE public.idiom_candidate_count_readiness;
DROP TABLE public.idiom_candidate_counts;
