CREATE TABLE public.occurrence_lemma_corrections (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    source_document_id text NOT NULL,
    start_offset bigint NOT NULL,
    end_offset bigint NOT NULL,
    canonical_lemma text NOT NULL,
    normalization_profile text NOT NULL,
    normalization_version text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT occurrence_lemma_corrections_span_check CHECK (end_offset > start_offset),
    CONSTRAINT occurrence_lemma_corrections_lemma_check CHECK (btrim(canonical_lemma) <> ''),
    CONSTRAINT occurrence_lemma_corrections_source_check CHECK (btrim(source_document_id) <> ''),
    CONSTRAINT occurrence_lemma_corrections_owner_book_analysis_span_key
        UNIQUE (owner_id, book_id, analysis_run_id, source_document_id, start_offset, end_offset),
    CONSTRAINT occurrence_lemma_corrections_corpus_fkey
        FOREIGN KEY (owner_id, corpus_id, analysis_run_id)
        REFERENCES public.corpora(owner_id, id, analysis_run_id) ON DELETE CASCADE,
    CONSTRAINT occurrence_lemma_corrections_book_fkey
        FOREIGN KEY (owner_id, book_id)
        REFERENCES public.books(owner_id, id) ON DELETE CASCADE
);

CREATE INDEX occurrence_lemma_corrections_analysis_idx
    ON public.occurrence_lemma_corrections(owner_id, corpus_id, analysis_run_id);
