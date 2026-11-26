-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
ALTER TABLE public.study_material
    DROP CONSTRAINT study_material_omission_pin_check,
    DROP COLUMN omission_dictionary_pin;
