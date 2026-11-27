-- Matched spans of an Idiom's Study material (ADR 0137 "Idiom Study material",
-- spec #2748, ticket #2753). An Idiom is matched on several tokens of one
-- sentence, and the card shows every matched word. The material keeps their
-- positions in the sentence; token_ordinal holds the first of them and
-- target_word the matched surface forms joined by a space, so the existing
-- ready check still holds.
--
-- DDL only (ADR 0038): one column with a constant default, which Postgres fills
-- without a rewrite, so every existing row, all of them words, has no matched
-- tokens and no data backfill exists. A word has none; a ready Idiom has at
-- least one. Forward-only in production (ADR 0038, ADR 0070); a second run
-- fails on the existing column and changes nothing.
ALTER TABLE public.study_material
    ADD COLUMN matched_token_ordinals integer[] DEFAULT '{}'::integer[] NOT NULL,
    ADD CONSTRAINT study_material_matched_tokens_check CHECK ((
        (item_kind = 'word' AND cardinality(matched_token_ordinals) = 0)
        OR (item_kind = 'idiom' AND (state <> 'ready' OR cardinality(matched_token_ordinals) > 0))
    ));

COMMENT ON COLUMN public.study_material.matched_token_ordinals IS 'Idiom material: the sentence token ordinals the headword matched, in sentence order; empty for a word';
