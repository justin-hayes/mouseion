-- Fluency scans that found a word's Corpus exhausted (#2817). The Fluency
-- worker records a row when candidate ranking returned no unassessed
-- candidate for a Mature word that still lacks its agreeing contexts, so
-- opening Today does not queue the word again every hour. exhausted_at is when
-- that scan began. A row is never cleared: it stops applying once the
-- language's Corpus changes or the word lapses after it, which
-- EnqueueFluencyPreparation reads from My Books, completed analysis runs,
-- confirmed Lemma corrections and Occurrence exclusions, Idiom count rebuilds
-- and the review log. The Study scheduler does not touch it.
--
-- Additive structural DDL only: a new table, no change to an existing one and
-- no data backfill. Forward-only in production (ADR 0038, ADR 0070).
CREATE TABLE public.study_fluency_scan (
    owner_id uuid NOT NULL,
    progress_id uuid NOT NULL,
    exhausted_at timestamptz NOT NULL,
    CONSTRAINT study_fluency_scan_pkey PRIMARY KEY (owner_id, progress_id),
    CONSTRAINT study_fluency_scan_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_fluency_scan_progress_fkey FOREIGN KEY (owner_id, progress_id) REFERENCES public.study_progress(owner_id, id) ON DELETE CASCADE
);
