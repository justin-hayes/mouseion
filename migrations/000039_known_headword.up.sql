-- Known vocabulary records the citation headword the accepted snapshot showed,
-- so a Greek word whose canonical key differs from its dictionary form (the
-- final-sigma-normalized key λόγοσ against the displayed λόγος) is displayed as
-- the learner saw it. The column is display-only evidence: it is never part of
-- the identity and never matched on.
--
-- The column is additive. Every existing row and every modern-language Known
-- word keeps the empty headword, so no existing identity, key or outcome
-- changes. The identity shape is settled by the Ancient Greek MVP spec (#2098):
-- a frozen record that displays a word stores the headword it showed. This is
-- the ticket that first writes Greek Known rows (#2107).
ALTER TABLE public.known_vocabulary
    ADD COLUMN headword text NOT NULL DEFAULT '';
