-- Forward-only in production (ADR 0038, ADR 0070). Development convenience
-- only. Per-learner ownership cannot be reconstructed from the
-- installation-wide rows, so this restores the old shape empty for a local
-- rollback to 000050.
ALTER TABLE public.reference_translations RENAME TO reference_translations_v2;

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
