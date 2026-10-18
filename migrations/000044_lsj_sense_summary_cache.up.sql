-- Per-entry cache for the provider-written Ancient Greek LSJ sense summary
-- (ADR 0101, ticket #2172). A summary is context-free: it condenses one pinned
-- LSJ entry, so its identity is the LSJ pin, the entry's native identity and the
-- provider prompt version. No learner identity and no Book data are stored, so a
-- summary is shared across every deck and occurrence that cites the same entry,
-- and a later preparation reuses it instead of calling the provider again.

CREATE TABLE public.lsj_sense_summaries (
    lsj_pin text NOT NULL,
    lsj_entry_id text NOT NULL,
    prompt_version text NOT NULL,
    summary_lines jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (lsj_pin, lsj_entry_id, prompt_version)
);
