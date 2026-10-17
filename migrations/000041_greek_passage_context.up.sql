-- Ancient Greek passage context (ADR 0101, ticket #2170).
--
-- Three additive shapes support the providerless Greek card's passage context:
--
--   * corpus_sentences.citations records every cited section a persisted
--     sentence touches, so a sentence crossing 1.2.17-18 can name both. The
--     list is the sorted, unique union of its tokens' source citations.
--   * reference_translations stores one operator-imported English translation
--     keyed by citation (Brownson's 1921-22 translation in the Perseus
--     perseus-eng2 digitisation). It is owner-scoped like every other imported
--     source, and each row records the pin of the file it came from.
--   * deck_preparation_manifest_items freezes the resolved passage text, its
--     citation label and the Brownson pin on the item, so a prepared deck never
--     changes after a re-import.
--
-- Every change is additive. Existing corpus sentences keep an empty citations
-- list, an installation without an imported translation keeps no rows, and
-- every existing manifest item keeps empty passage fields, so modern-language
-- decks and already-prepared Greek decks are unchanged.

ALTER TABLE public.corpus_sentences
    ADD COLUMN citations text[] NOT NULL DEFAULT '{}';

CREATE TABLE public.reference_translations (
    owner_id uuid NOT NULL,
    language text NOT NULL,
    citation text NOT NULL,
    translation text NOT NULL,
    source_pin text NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT reference_translations_citation_check CHECK ((citation <> ''::text)),
    CONSTRAINT reference_translations_language_check CHECK ((language <> ''::text)),
    CONSTRAINT reference_translations_source_pin_check CHECK ((source_pin <> ''::text)),
    CONSTRAINT reference_translations_translation_check CHECK ((translation <> ''::text))
);

ALTER TABLE ONLY public.reference_translations
    ADD CONSTRAINT reference_translations_pkey PRIMARY KEY (owner_id, language, citation);

ALTER TABLE ONLY public.reference_translations
    ADD CONSTRAINT reference_translations_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE public.deck_preparation_manifest_items
    ADD COLUMN passage_context text NOT NULL DEFAULT '',
    ADD COLUMN passage_citation text NOT NULL DEFAULT '',
    ADD COLUMN brownson_pin text NOT NULL DEFAULT '';
