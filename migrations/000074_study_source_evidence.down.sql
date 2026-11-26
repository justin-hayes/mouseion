-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
ALTER TABLE public.study_fluency_context DROP CONSTRAINT study_fluency_context_citations_check;
ALTER TABLE public.study_fluency_context
    DROP COLUMN dictionary_pin,
    DROP COLUMN cited_entry_ids,
    DROP COLUMN cited_sense_ids;
ALTER TABLE public.study_report
    DROP COLUMN previous_learned_dictionary_pin,
    DROP COLUMN previous_learned_entry_ids,
    DROP COLUMN previous_learned_sense_ids;
ALTER TABLE public.study_progress DROP CONSTRAINT study_progress_learned_citations_check;
ALTER TABLE public.study_progress
    DROP COLUMN learned_dictionary_pin,
    DROP COLUMN learned_entry_ids,
    DROP COLUMN learned_sense_ids;
ALTER TABLE public.study_material DROP CONSTRAINT study_material_passage_check;
ALTER TABLE public.study_material DROP CONSTRAINT study_material_citations_check;
ALTER TABLE public.study_material
    DROP COLUMN passage_translation_pin,
    DROP COLUMN passage_citation,
    DROP COLUMN passage_context,
    DROP COLUMN dictionary_pin,
    DROP COLUMN cited_entry_ids,
    DROP COLUMN cited_sense_ids;
