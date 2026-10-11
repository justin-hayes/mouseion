-- Measure Book-vocabulary selection rules against a learner's analyzed library
-- (ADR 0090, issue #1803). Read-only: it creates only session temp tables.
--
--   docker exec -i mouseion-db-1 psql -U mouseion -d mouseion -P pager=off \
--     < tools/measure-book-vocabulary-selection.sql
--
-- Each Book is scored as if it were the next Current reading, against every
-- other currently analyzed same-language Book with a ready count projection.
-- Coverage is (Known + selected tokens) / analyzable tokens. Reserved
-- vocabulary is ignored, so card counts are upper bounds for the active
-- reading's language.

CREATE TEMP TABLE measured_books AS
SELECT cai.owner_id, cai.book_id, b.language_tag AS lang, c.analyzable_token_count AS analyzable,
       left(coalesce(b.title, b.id::text), 40) AS title, cai.analysis_run_id, cai.corpus_id
FROM current_analysis_identity cai
JOIN books b ON b.owner_id = cai.owner_id AND b.id = cai.book_id
JOIN corpora c ON c.owner_id = cai.owner_id AND c.id = cai.corpus_id
JOIN vocabulary_browse_count_readiness r ON r.owner_id = cai.owner_id AND r.book_id = cai.book_id
  AND r.analysis_run_id = cai.analysis_run_id AND r.corpus_id = cai.corpus_id AND r.builder_version = 3
WHERE b.language_state = 'chosen';

CREATE TEMP TABLE measured_identities AS
SELECT mb.owner_id, mb.book_id, mb.lang, v.canonical_lemma AS lemma, v.upos, v.occurrence_count AS n,
       EXISTS (SELECT 1 FROM known_vocabulary k
               WHERE k.owner_id = mb.owner_id AND k.language = mb.lang
                 AND k.canonical_lemma = v.canonical_lemma AND k.upos IN (v.upos, '')) AS known
FROM measured_books mb
JOIN vocabulary_browse_counts v ON v.owner_id = mb.owner_id AND v.book_id = mb.book_id
  AND v.analysis_run_id = mb.analysis_run_id AND v.corpus_id = mb.corpus_id;
CREATE INDEX ON measured_identities (owner_id, lang, lemma, upos);
ANALYZE measured_identities;

CREATE TEMP TABLE measured_candidates AS
SELECT i.*, l.total, l.books - 1 AS spread
FROM measured_identities i
JOIN (SELECT owner_id, lang, lemma, upos, sum(n) AS total, count(*) AS books
      FROM measured_identities GROUP BY 1, 2, 3, 4) l USING (owner_id, lang, lemma, upos);

\echo '== Library'
SELECT lang, count(*) AS books, sum(analyzable) AS analyzable_tokens FROM measured_books GROUP BY 1 ORDER BY 1;

\echo '== Per Book: cards and coverage at the three-occurrence floor and with the across-Books route (T = 15)'
SELECT mb.lang, mb.title, mb.analyzable AS tokens,
       round(100.0 * coalesce(sum(n) FILTER (WHERE known), 0) / mb.analyzable, 1) AS known_pct,
       count(*) FILTER (WHERE NOT known AND n >= 3) AS floor_cards,
       count(*) FILTER (WHERE NOT known AND n < 3 AND total >= 15) AS across_cards,
       count(*) FILTER (WHERE NOT known AND n = 1 AND total >= 15) AS of_which_single,
       round(100.0 * coalesce(sum(n) FILTER (WHERE known OR n >= 3 OR total >= 15), 0) / mb.analyzable, 1) AS coverage_pct
FROM measured_books mb JOIN measured_candidates c USING (owner_id, book_id)
GROUP BY mb.lang, mb.title, mb.analyzable
ORDER BY mb.lang, mb.analyzable;

\echo '== Threshold sweep: extra cards per Book over the floor, coverage, and median occurrences of the extras in other Books'
SELECT c.lang, t.threshold,
       round(count(*) FILTER (WHERE NOT known AND n < 3 AND total >= t.threshold)::numeric / count(DISTINCT book_id)) AS extra_per_book,
       round(count(*) FILTER (WHERE NOT known AND n = 1 AND total >= t.threshold)::numeric / count(DISTINCT book_id)) AS of_which_single,
       percentile_cont(0.5) WITHIN GROUP (ORDER BY total - n) FILTER (WHERE NOT known AND n < 3 AND total >= t.threshold) AS median_other_occurrences,
       round(100.0 * sum(n) FILTER (WHERE known OR n >= 3 OR total >= t.threshold) / sum(n), 1) AS token_weighted_coverage_pct
FROM measured_candidates c
CROSS JOIN (VALUES (10), (15), (20), (30)) AS t(threshold)
GROUP BY c.lang, t.threshold
ORDER BY c.lang, t.threshold;

\echo '== Across-Books rescues (T = 15) where one other Book supplies at least half of the other-Book occurrences'
SELECT c.lang, count(*) AS rescues,
       round(100.0 * avg((top.top_other * 2 >= c.total - c.n)::int), 1) AS one_book_dominates_pct
FROM measured_candidates c
JOIN LATERAL (
  SELECT max(o.n) AS top_other FROM measured_identities o
  WHERE o.owner_id = c.owner_id AND o.lang = c.lang AND o.lemma = c.lemma AND o.upos = c.upos AND o.book_id <> c.book_id
) top ON true
WHERE NOT c.known AND c.n < 3 AND c.total >= 15
GROUP BY c.lang ORDER BY c.lang;
