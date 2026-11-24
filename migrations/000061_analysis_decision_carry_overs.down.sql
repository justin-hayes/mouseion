-- Development convenience only (ADR 0038, ADR 0070).
DROP TABLE public.analysis_decision_carry_overs;
ALTER TABLE public.analysis_runs DROP CONSTRAINT analysis_runs_owner_id_id_key;
