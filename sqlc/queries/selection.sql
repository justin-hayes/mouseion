-- Sentence selection and review queries. SelectAcquisitionCandidate is the
-- static replacement for the runtime string-concatenated acquisition query:
-- an unconditional predicate of the form ($book = '' OR source = $book) keeps
-- the statement statically analyzable.

-- name: SelectAcquisitionCandidate :one
SELECT sc.corpus_id, sc.eligible_sentence_refs
FROM selection_candidates sc
JOIN corpora c ON c.owner_id = sc.owner_id AND c.id::text = sc.corpus_id
WHERE sc.owner_id = sqlc.arg('owner')
  AND sc.language = sqlc.arg('language')
  AND sc.canonical_lemma = sqlc.arg('canonical_lemma')
  AND sc.upos = sqlc.arg('upos')
  AND (sqlc.arg('book')::text = '' OR c.source_material_id::text = sqlc.arg('book'))
ORDER BY sc.selected_at DESC, sc.corpus_id DESC
LIMIT 1;

-- name: UpsertReviewSentenceFromAnalysis :one
INSERT INTO example_sentences(owner_id, corpus_id, sentence_key, sentence_text, source_location, language, canonical_lemma, upos, selection_rank, selection_score, selection_reasons, is_chosen)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, 0, '[]', true)
ON CONFLICT(owner_id, corpus_id, sentence_key) DO UPDATE SET
  sentence_text = excluded.sentence_text,
  source_location = excluded.source_location,
  language = excluded.language,
  canonical_lemma = excluded.canonical_lemma,
  upos = excluded.upos,
  selection_rank = excluded.selection_rank,
  selection_score = excluded.selection_score,
  selection_reasons = excluded.selection_reasons,
  is_chosen = true
RETURNING id, owner_id, corpus_id, sentence_key, sentence_text, source_location, language, canonical_lemma, upos, selection_rank, selection_score, selection_reasons, is_chosen, created_at;

-- name: ListSelectedSentences :many
SELECT id, owner_id, corpus_id, sentence_key, sentence_text, source_location, language, canonical_lemma, upos, selection_rank, selection_score, selection_reasons, is_chosen, created_at
FROM example_sentences
WHERE owner_id = sqlc.arg('owner') AND corpus_id = sqlc.arg('corpus')
  AND language = sqlc.arg('language') AND canonical_lemma = sqlc.arg('canonical_lemma') AND upos = sqlc.arg('upos')
ORDER BY selection_rank;

-- name: ListReviewSentences :many
WITH latest AS (
  SELECT corpus_id FROM example_sentences
  WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
    AND canonical_lemma = sqlc.arg('canonical_lemma') AND upos = sqlc.arg('upos')
  GROUP BY corpus_id ORDER BY max(created_at) DESC, corpus_id DESC LIMIT 1
)
SELECT e.id, e.owner_id, e.corpus_id, e.sentence_key, e.sentence_text, e.source_location, e.language, e.canonical_lemma, e.upos, e.selection_rank, e.selection_score, e.selection_reasons, e.is_chosen, e.created_at
FROM example_sentences e
WHERE e.owner_id = sqlc.arg('owner') AND e.language = sqlc.arg('language')
  AND e.canonical_lemma = sqlc.arg('canonical_lemma') AND e.upos = sqlc.arg('upos')
  AND e.corpus_id = (SELECT corpus_id FROM latest)
ORDER BY e.is_chosen DESC, e.selection_rank, e.id;

-- name: ListReviewSentencesForBook :many
SELECT e.id, e.owner_id, e.corpus_id, e.sentence_key, e.sentence_text, e.source_location, e.language, e.canonical_lemma, e.upos, e.selection_rank, e.selection_score, e.selection_reasons, e.is_chosen, e.created_at
FROM example_sentences e
JOIN corpora c ON c.owner_id = e.owner_id AND c.id = e.corpus_id
WHERE e.owner_id = sqlc.arg('owner') AND c.source_material_id = sqlc.arg('book')
  AND e.language = sqlc.arg('language') AND e.canonical_lemma = sqlc.arg('canonical_lemma') AND e.upos = sqlc.arg('upos')
ORDER BY e.is_chosen DESC, e.selection_rank, e.id;