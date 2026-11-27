-- Down migrations are a development convenience, not a supported production rollback (ADR 0038, ADR 0070).
ALTER TABLE public.study_material
    DROP CONSTRAINT study_material_matched_tokens_check,
    DROP COLUMN matched_token_ordinals;
