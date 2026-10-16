-- Ancient Greek Concordance form search matches a stored ancient-greek/1 search
-- key computed in Go, never SQL lower(), so NFC/NFD, case and final-sigma
-- variants find the same results while accents and breathings stay significant.
--
-- The column is additive: every existing row and every modern-language token
-- keeps the empty key and keeps today's lower(...) form matching, so no stored
-- identity or outcome changes. A Greek run written before this column existed
-- (#2103) carries the empty key; re-importing under the current interpretation
-- version writes a fresh run whose tokens carry their keys.
--
-- No standalone key index is added: the form predicate compares the stored key
-- for grc and SQL case folding for other languages, so a key-only index cannot
-- be used. The lookup already joins corpus tokens on their indexed keys.
ALTER TABLE public.corpus_tokens
    ADD COLUMN search_key text NOT NULL DEFAULT '';
