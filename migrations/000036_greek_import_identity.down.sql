DROP INDEX public.vocabulary_browse_counts_identity_idx;

CREATE INDEX vocabulary_browse_counts_identity_idx
    ON public.vocabulary_browse_counts(owner_id, language, canonical_lemma, upos, book_id);

ALTER TABLE public.vocabulary_browse_counts
    DROP CONSTRAINT vocabulary_browse_counts_pkey,
    DROP COLUMN lexical_discriminator,
    ADD CONSTRAINT vocabulary_browse_counts_pkey PRIMARY KEY (owner_id, book_id, analysis_run_id, canonical_lemma, upos);

ALTER TABLE ONLY public.analysis_runs
    DROP CONSTRAINT analysis_runs_syntax_state_check,
    DROP COLUMN syntax_state;

ALTER TABLE public.corpus_tokens
    DROP COLUMN lexical_discriminator,
    DROP COLUMN headword;
