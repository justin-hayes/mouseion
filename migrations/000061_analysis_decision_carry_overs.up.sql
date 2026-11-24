-- ADR 0126: an Analysis update copies Lemma corrections and Occurrence
-- exclusions onto identical tokens of the run it publishes. This table records,
-- per published run, how many decisions the copy step carried over (copied or
-- absorbed) out of how many the replaced run held. /jobs reads it to report
-- "N of M decisions carried over".
--
-- Structural DDL only, with no backfill: runs published before this migration
-- have no row and show no line. Forward-only in production (ADR 0038, ADR 0070).
--
-- The composite unique key on analysis_runs lets the foreign keys below stay
-- owner-scoped, like the other learner-owned tables.
ALTER TABLE public.analysis_runs
    ADD CONSTRAINT analysis_runs_owner_id_id_key UNIQUE (owner_id, id);

CREATE TABLE public.analysis_decision_carry_overs (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    previous_analysis_run_id uuid NOT NULL,
    -- The River job that published the run, when a Classical preparation job
    -- did. The job's final state does not change this link: publication commits
    -- before the job's last write, so a job that later failed still published.
    river_job_id bigint,
    carried integer NOT NULL,
    total integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT analysis_decision_carry_overs_pkey PRIMARY KEY (owner_id, analysis_run_id),
    CONSTRAINT analysis_decision_carry_overs_carried_check CHECK (carried >= 0),
    CONSTRAINT analysis_decision_carry_overs_total_check CHECK (total >= carried),
    CONSTRAINT analysis_decision_carry_overs_book_fkey
        FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT analysis_decision_carry_overs_run_fkey
        FOREIGN KEY (owner_id, analysis_run_id) REFERENCES public.analysis_runs(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT analysis_decision_carry_overs_previous_run_fkey
        FOREIGN KEY (owner_id, previous_analysis_run_id) REFERENCES public.analysis_runs(owner_id, id) ON DELETE CASCADE
);
