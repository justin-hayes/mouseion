-- name: ListCorpusSentences :many
SELECT s.sentence_ordinal,
       s.sentence_text,
       s.citations,
       s.start_offset,
       COALESCE(t.token_ordinal, -1::bigint)::bigint AS token_ordinal,
       COALESCE(t.surface, '')::text AS surface,
       COALESCE(t.raw_lemma, '')::text AS raw_lemma,
       COALESCE(t.canonical_lemma, '')::text AS canonical_lemma,
       COALESCE(t.upos, '')::text AS upos,
       COALESCE(t.morphology, '{}'::jsonb)::jsonb AS morphology,
       COALESCE(t.dependency, '')::text AS dependency,
       COALESCE(t.head, 0::bigint)::bigint AS head,
       COALESCE(t.start_offset, 0::bigint)::bigint AS token_start_offset,
       COALESCE(t.end_offset, 0::bigint)::bigint AS token_end_offset
FROM corpus_sentences s
LEFT JOIN corpus_tokens t
  ON t.owner_id = s.owner_id
 AND t.corpus_id = s.corpus_id
 AND t.analysis_run_id = s.analysis_run_id
 AND t.sentence_ordinal = s.sentence_ordinal
WHERE s.owner_id = sqlc.arg('owner')
  AND s.corpus_id = sqlc.arg('corpus')
  AND s.sentence_ordinal = ANY(sqlc.arg('sentence_ordinals')::bigint[])
ORDER BY s.sentence_ordinal, t.token_ordinal;

-- Card export and generated-vocabulary persistence queries.

