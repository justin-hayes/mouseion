-- Analysis-insight vocabulary aggregation.

-- name: ListProjectedCorpusVocabulary :many
-- Per-Book effective vocabulary counts for one corpus, read from the Browse
-- count projection. A corpus whose current analysis has no ready projection
-- produces no row. A ready projection with no counts produces one row with an
-- empty identity so the two cases stay distinguishable.
SELECT COALESCE(c.language, '')::text AS language,
       COALESCE(c.canonical_lemma, '')::text AS canonical_lemma,
       COALESCE(c.upos, '')::text AS upos,
       COALESCE(c.occurrence_count, 0)::bigint AS occurrence_count
FROM current_analysis_identity cai
JOIN vocabulary_browse_count_readiness r ON r.owner_id = cai.owner_id
  AND r.book_id = cai.book_id
  AND r.analysis_run_id = cai.analysis_run_id
  AND r.corpus_id = cai.corpus_id
  AND r.builder_version = 3
LEFT JOIN vocabulary_browse_counts c ON c.owner_id = r.owner_id
  AND c.book_id = r.book_id
  AND c.analysis_run_id = r.analysis_run_id
  AND c.corpus_id = r.corpus_id
WHERE cai.owner_id = sqlc.arg('owner')
  AND cai.corpus_id = sqlc.arg('corpus')
ORDER BY c.language, c.canonical_lemma, c.upos;
