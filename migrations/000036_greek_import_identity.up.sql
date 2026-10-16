-- Ancient Greek imported analyses: the lexical discriminator and citation
-- headword on corpus tokens, the run-level syntax state, and the discriminator
-- as part of the effective vocabulary identity in the Browse projection.
--
-- Every migration here is additive. Existing rows keep the empty discriminator
-- and an empty headword, and every existing analysis run keeps syntax state
-- 'ud', so modern-language identities, keys and outcomes are unchanged.

-- A source lemma such as δέω2 decodes to the citation headword δέω and the
-- scheme-qualified discriminator morpheus:2. Both default to the empty string,
-- which is what every existing row and every modern-language word carries.
ALTER TABLE public.corpus_tokens
    ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '',
    ADD COLUMN headword text NOT NULL DEFAULT '';

-- A run records whether its tokens carry UD syntax, native AGDT syntax that is
-- never read as UD, or no syntax at all. Ordinary analyses are 'ud'.
ALTER TABLE public.analysis_runs
    ADD COLUMN syntax_state text NOT NULL DEFAULT 'ud';

ALTER TABLE ONLY public.analysis_runs
    ADD CONSTRAINT analysis_runs_syntax_state_check CHECK (syntax_state = ANY (ARRAY['ud'::text, 'native_agdt'::text, 'absent'::text]));

-- The effective vocabulary identity gains the discriminator, so homographs that
-- share a canonical lemma and part of speech stay distinct. Every existing row
-- has the empty discriminator, so the rebuilt key changes no value. The builder
-- version stays 3: the projection's eligibility rule is unchanged for any
-- existing token (an empty discriminator), so no readiness marker is stale and
-- modern-language counts are byte-for-byte identical.
ALTER TABLE public.vocabulary_browse_counts
    DROP CONSTRAINT vocabulary_browse_counts_pkey,
    ADD COLUMN lexical_discriminator text NOT NULL DEFAULT '',
    ADD CONSTRAINT vocabulary_browse_counts_pkey PRIMARY KEY (owner_id, book_id, analysis_run_id, canonical_lemma, upos, lexical_discriminator);

DROP INDEX public.vocabulary_browse_counts_identity_idx;

CREATE INDEX vocabulary_browse_counts_identity_idx
    ON public.vocabulary_browse_counts(owner_id, language, canonical_lemma, upos, lexical_discriminator, book_id);
