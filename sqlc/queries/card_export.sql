-- name: ListCorpusSentences :many
SELECT s.sentence_ordinal,
       s.sentence_text,
       COALESCE(t.token_ordinal, -1::bigint)::bigint AS token_ordinal,
       COALESCE(t.surface, '')::text AS surface,
       COALESCE(t.raw_lemma, '')::text AS raw_lemma,
       COALESCE(t.canonical_lemma, '')::text AS canonical_lemma,
       COALESCE(t.upos, '')::text AS upos,
       COALESCE(t.morphology, '{}'::jsonb)::jsonb AS morphology,
       COALESCE(t.dependency, '')::text AS dependency,
       COALESCE(t.head, 0::bigint)::bigint AS head
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

-- name: GetCorpusForAnalysis :one
SELECT c.id::text,
       c.owner_id::text,
       c.source_material_id::text,
       c.artifact_hash,
       (COALESCE(c.analysis_run_id::text, ''))::text AS analysis_run_id,
       c.status,
       c.created_at
FROM corpora c
JOIN analysis_runs r ON r.owner_id = c.owner_id
                    AND r.id = c.analysis_run_id
                    AND r.source_material_id = c.source_material_id
WHERE c.owner_id = $1
  AND r.id = $2
  AND r.state = 'completed'
  AND c.status = 'complete';

-- name: PutGeneratedCard :exec
INSERT INTO cards(owner_id, deck_id, dedup_key, canonical_lemma, upos, front, back)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT(owner_id, dedup_key) DO UPDATE SET
  deck_id = excluded.deck_id,
  front = excluded.front,
  back = excluded.back;

-- name: PutGeneratedVocabulary :exec
INSERT INTO generated_vocabulary(owner_id, language, canonical_lemma, upos, first_deck_id, first_source_material_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO NOTHING;

-- name: GetGeneratedVocabulary :one
SELECT owner_id::text,
       language,
       canonical_lemma,
       upos,
       first_deck_id::text,
       (COALESCE(first_source_material_id::text, ''))::text AS first_source_material_id,
       first_generated_at
FROM generated_vocabulary
WHERE owner_id = $1
  AND language = $2
  AND canonical_lemma = $3
  AND upos = $4;

-- name: ListGeneratedVocabulary :many
SELECT owner_id::text,
       language,
       canonical_lemma,
       upos,
       first_deck_id::text,
       (COALESCE(first_source_material_id::text, ''))::text AS first_source_material_id,
       first_generated_at
FROM generated_vocabulary
WHERE owner_id = $1
  AND language = $2
ORDER BY canonical_lemma, upos;
