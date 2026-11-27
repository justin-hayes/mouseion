-- Per-Book Idiom candidate counts (ADR 0137, "Idiom counts for ranking"). Each
-- row says how many Idiom candidates of one headword a Book's current analysis
-- contains, matched on effective lemmas. The Corpus sums them to rank Idioms
-- among new words; page requests never derive them.
--
-- DDL only: two new tables, nothing to backfill. A Book's counts are written by
-- a durable job after its analysis publishes, so an existing Book has no
-- readiness row until startup reconciliation queues its rebuild, which is
-- idempotent and safe to retry (it replaces the Book's rows in one transaction).
-- Removing a Book removes its rows (ON DELETE CASCADE).
CREATE TABLE public.idiom_candidate_counts (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    language text NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    headword text NOT NULL CHECK (btrim(headword) <> ''),
    candidate_count bigint NOT NULL CHECK (candidate_count > 0),
    PRIMARY KEY (owner_id, book_id, analysis_run_id, headword),
    FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    FOREIGN KEY (analysis_run_id) REFERENCES public.analysis_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (owner_id, corpus_id) REFERENCES public.corpora(owner_id, id) ON DELETE CASCADE
);

CREATE INDEX idiom_candidate_counts_headword_idx
    ON public.idiom_candidate_counts(owner_id, language, headword, book_id);

-- A Book's counts are usable only while this row names its current analysis and
-- the builder and idiom index version they were derived with. A Book with no
-- candidates has a readiness row and no count rows.
CREATE TABLE public.idiom_candidate_count_readiness (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    language text NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    builder_version integer NOT NULL CHECK (builder_version > 0),
    index_version text NOT NULL,
    ready_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, book_id),
    FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    FOREIGN KEY (analysis_run_id) REFERENCES public.analysis_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (owner_id, corpus_id) REFERENCES public.corpora(owner_id, id) ON DELETE CASCADE
);
