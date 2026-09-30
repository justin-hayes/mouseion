CREATE TABLE public.occurrence_lemma_review_flags (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    source_document_id text NOT NULL,
    start_offset bigint NOT NULL,
    end_offset bigint NOT NULL,
    reason text NOT NULL,
    evidence_provenance jsonb NOT NULL,
    resolution text,
    flagged_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    CONSTRAINT occurrence_lemma_review_flags_span_check CHECK (end_offset > start_offset),
    CONSTRAINT occurrence_lemma_review_flags_reason_check CHECK (btrim(reason) <> ''),
    CONSTRAINT occurrence_lemma_review_flags_provenance_check CHECK (jsonb_typeof(evidence_provenance) = 'object'),
    CONSTRAINT occurrence_lemma_review_flags_resolution_check CHECK (resolution IS NULL OR resolution IN ('keep', 'correct', 'exclude')),
    CONSTRAINT occurrence_lemma_review_flags_resolution_time_check CHECK ((resolution IS NULL) = (resolved_at IS NULL)),
    CONSTRAINT occurrence_lemma_review_flags_owner_book_analysis_span_key
        PRIMARY KEY (owner_id, book_id, analysis_run_id, source_document_id, start_offset, end_offset),
    CONSTRAINT occurrence_lemma_review_flags_corpus_fkey
        FOREIGN KEY (owner_id, corpus_id, analysis_run_id)
        REFERENCES public.corpora(owner_id, id, analysis_run_id) ON DELETE CASCADE,
    CONSTRAINT occurrence_lemma_review_flags_book_fkey
        FOREIGN KEY (owner_id, book_id)
        REFERENCES public.books(owner_id, id) ON DELETE CASCADE
);

CREATE INDEX occurrence_lemma_review_flags_unresolved_idx
    ON public.occurrence_lemma_review_flags(owner_id, book_id, analysis_run_id)
    WHERE resolution IS NULL;
