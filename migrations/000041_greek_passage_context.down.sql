-- Rolling back drops the frozen passage of every prepared Greek item and the
-- imported reference translations. A prepared deck that already froze a passage
-- loses it and can only be recovered by re-importing the translation and
-- re-preparing the deck, so this is not a safe destructive production rollback.
ALTER TABLE public.deck_preparation_manifest_items
    DROP COLUMN passage_context,
    DROP COLUMN passage_citation,
    DROP COLUMN brownson_pin;

DROP TABLE public.reference_translations;

ALTER TABLE public.corpus_sentences
    DROP COLUMN citations;
