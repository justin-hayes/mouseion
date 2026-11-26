-- Study re-prepares a word recorded as unpreparable for no-dictionary-entry
-- when the loaded dictionary index's pin for the language changes (ADR 0138
-- §1, ticket #2775). The omission's cause is the dictionary, so it is only
-- worth trying again against a different one.
--
-- omission_dictionary_pin is the pinned dictionary revision a
-- no-dictionary-entry omission was recorded against. It is empty for every
-- other row, including every omission recorded before this column existed: an
-- empty pin differs from any loaded pin, so such a word is tried once more and
-- then records its pin. Additive, with one consumer (Study planning) and no
-- data backfill. Meaning and sentence-quality omissions never set it.
ALTER TABLE public.study_material
    ADD COLUMN omission_dictionary_pin text DEFAULT ''::text NOT NULL,
    ADD CONSTRAINT study_material_omission_pin_check CHECK ((btrim(omission_dictionary_pin) = ''::text OR (state = 'unpreparable'::text AND omission_reason = 'no-dictionary-entry'::text)));
