-- Study's Idiom material (ADR 0137, spec #2748, ticket #2753): where an Idiom's
-- candidate sentences are, and the decisions that make their lemmas effective.

-- name: ListStudyIdiomSentences :many
-- The sentences of the owner's currently analyzed Books in My Books, in the
-- study language, that contain every component word of an Idiom headword: each
-- word as a token's surface form or its effective lemma (a confirmed Lemma
-- correction supplies the lemma, an excluded Occurrence matches nothing). It is
-- a cheap superset of the sentences the matcher accepts, which Go still
-- decides. The Current reading's sentences come first. words are the distinct,
-- lower-case component words.
WITH eligible AS MATERIALIZED (
  SELECT cai.owner_id, cai.book_id, cai.analysis_run_id, cai.corpus_id, cai.source_material_id, ar.snapshot_id,
         COALESCE(cai.book_id = g.book_id, false) AS current_reading
    FROM current_analysis_identity cai
    JOIN analysis_runs ar ON ar.owner_id = cai.owner_id AND ar.id = cai.analysis_run_id
    JOIN books b ON b.owner_id = cai.owner_id AND b.id = cai.book_id
     AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
    JOIN book_membership m ON m.owner_id = cai.owner_id AND m.book_id = cai.book_id AND m.state = 'active'
    LEFT JOIN primary_goals g ON g.owner_id = cai.owner_id AND g.language = sqlc.arg('language')::text
   WHERE cai.owner_id = sqlc.arg('owner')::uuid
), matched AS (
  SELECT e.book_id, e.analysis_run_id, e.corpus_id, e.current_reading, t.sentence_ordinal,
         min(u.start_offset + s.start_offset)::bigint AS book_start_offset,
         count(DISTINCT w.word) AS matched_words
    FROM eligible e
    JOIN corpus_tokens t ON t.owner_id = e.owner_id AND t.analysis_run_id = e.analysis_run_id AND t.corpus_id = e.corpus_id
    JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.analysis_run_id = t.analysis_run_id
     AND s.corpus_id = t.corpus_id AND s.sentence_ordinal = t.sentence_ordinal
    JOIN source_material_units u ON u.owner_id = e.owner_id AND u.source_material_id = e.source_material_id
     AND u.snapshot_id = e.snapshot_id AND u.unit_id = s.unit_id
    LEFT JOIN occurrence_lemma_corrections d ON d.owner_id = e.owner_id AND d.book_id = e.book_id
     AND d.analysis_run_id = t.analysis_run_id AND d.corpus_id = t.corpus_id
     AND d.source_document_id = s.unit_id AND d.start_offset = t.start_offset AND d.end_offset = t.end_offset
    CROSS JOIN LATERAL unnest(sqlc.arg('words')::text[]) AS w(word)
   WHERE t.language = sqlc.arg('language')::text AND NOT COALESCE(d.excluded, false)
     AND (lower(t.surface) = w.word OR lower(COALESCE(d.canonical_lemma, t.canonical_lemma)) = w.word)
   GROUP BY e.book_id, e.analysis_run_id, e.corpus_id, e.current_reading, t.sentence_ordinal
)
SELECT book_id::text AS book_id, analysis_run_id::text AS analysis_run_id, corpus_id::text AS corpus_id,
       sentence_ordinal, current_reading::boolean AS current_reading, book_start_offset
  FROM matched
 WHERE matched_words = cardinality(sqlc.arg('words')::text[])
 ORDER BY current_reading DESC, book_id, sentence_ordinal
 LIMIT sqlc.arg('max_rows')::int;

-- name: ListStudySentenceDecisions :many
-- The Lemma corrections and Occurrence exclusions on the tokens of the given
-- sentences of one Book's analysis: the decisions that make a lemma effective.
SELECT t.sentence_ordinal, t.token_ordinal, d.canonical_lemma AS decision_lemma, COALESCE(d.excluded, false)::boolean AS decision_excluded
  FROM corpus_tokens t
  JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.analysis_run_id = t.analysis_run_id
   AND s.corpus_id = t.corpus_id AND s.sentence_ordinal = t.sentence_ordinal
  JOIN occurrence_lemma_corrections d ON d.owner_id = t.owner_id AND d.book_id = sqlc.arg('book')::uuid
   AND d.analysis_run_id = t.analysis_run_id AND d.corpus_id = t.corpus_id
   AND d.source_document_id = s.unit_id AND d.start_offset = t.start_offset AND d.end_offset = t.end_offset
 WHERE t.owner_id = sqlc.arg('owner')::uuid AND t.analysis_run_id = sqlc.arg('analysis_run')::uuid
   AND t.corpus_id = sqlc.arg('corpus')::uuid AND t.sentence_ordinal = ANY(sqlc.arg('sentence_ordinals')::bigint[]);
