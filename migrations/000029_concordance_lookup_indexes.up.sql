-- Analyzer lemma is an explicit Concordance lookup mode and is distinct from
-- canonical_lemma after learner corrections. Give that exact predicate its own
-- covering key; the canonical and surface lookup modes already have indexes.
CREATE INDEX corpus_tokens_owner_language_raw_lemma_upos_idx
    ON public.corpus_tokens(owner_id, language, raw_lemma, upos);
