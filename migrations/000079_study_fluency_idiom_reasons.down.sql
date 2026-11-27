-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback: it fails while
-- a literal-use or ambiguous context is stored.
ALTER TABLE public.study_fluency_context
    DROP CONSTRAINT study_fluency_context_reason_check,
    ADD CONSTRAINT study_fluency_context_reason_check CHECK ((reason = ANY (ARRAY['same-meaning'::text, 'different-meaning'::text, 'idiomatic'::text, 'not-this-lemma'::text, 'undefendable'::text])));
