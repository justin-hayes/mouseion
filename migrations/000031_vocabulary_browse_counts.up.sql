CREATE TABLE public.vocabulary_browse_counts (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    language text NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    occurrence_count bigint NOT NULL CHECK (occurrence_count > 0),
    corrected boolean NOT NULL DEFAULT false,
    PRIMARY KEY (owner_id, book_id, analysis_run_id, canonical_lemma, upos),
    FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    FOREIGN KEY (analysis_run_id) REFERENCES public.analysis_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (owner_id, corpus_id) REFERENCES public.corpora(owner_id, id) ON DELETE CASCADE
);

CREATE INDEX vocabulary_browse_counts_identity_idx
    ON public.vocabulary_browse_counts(owner_id, language, canonical_lemma, upos, book_id);

CREATE TABLE public.vocabulary_browse_count_readiness (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    language text NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    builder_version integer NOT NULL CHECK (builder_version > 0),
    ready_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, book_id),
    FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    FOREIGN KEY (analysis_run_id) REFERENCES public.analysis_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (owner_id, corpus_id) REFERENCES public.corpora(owner_id, id) ON DELETE CASCADE
);
