-- Reasons an Idiom's Fluency context can disagree with its Learned meaning
-- (ADR 0137 §6, spec #2748, ticket #2756): the idiom-card request can answer
-- literal use or ambiguous, which no existing reason says. They are kept as
-- evidence like the others and never shown.
--
-- DDL only (ADR 0038): the CHECK is replaced by a wider one that every stored
-- row already satisfies, so no data is rewritten and there is no backfill.
-- Forward-only in production (ADR 0038, ADR 0070); a second run drops and
-- re-adds the same constraint and changes nothing.
ALTER TABLE public.study_fluency_context
    DROP CONSTRAINT study_fluency_context_reason_check,
    ADD CONSTRAINT study_fluency_context_reason_check CHECK ((reason = ANY (ARRAY['same-meaning'::text, 'different-meaning'::text, 'idiomatic'::text, 'not-this-lemma'::text, 'undefendable'::text, 'literal-use'::text, 'ambiguous'::text])));
