-- Construction occurrences: a derived reading of a native_agdt run's stored
-- syntax (ADR 0123). Lemma corrections and Occurrence exclusions are never
-- read here.

-- name: GetConstructionRunSource :one
-- The run's syntax state, its corpus and its language. A run with no corpus
-- yet has nothing to build from.
SELECT ar.syntax_state,
       COALESCE(ar.corpus_id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS corpus_id,
       sm.language
  FROM analysis_runs ar
  JOIN source_materials sm ON sm.owner_id=ar.owner_id AND sm.id=ar.source_material_id
 WHERE ar.owner_id=sqlc.arg('owner') AND ar.id=sqlc.arg('analysis_run');

-- name: ListConstructionTokens :many
-- Every printed token of a run, in sentence and token order. Raw lemma and
-- morphology only: the effective lemma is never an input.
SELECT t.sentence_ordinal,
       t.token_ordinal,
       t.surface,
       t.raw_lemma,
       t.upos,
       t.morphology,
       t.dependency,
       t.head,
       COALESCE(t.head_unprinted_node, '')::text AS head_unprinted_node
  FROM corpus_tokens t
 WHERE t.owner_id=sqlc.arg('owner') AND t.analysis_run_id=sqlc.arg('analysis_run')
 ORDER BY t.sentence_ordinal, t.token_ordinal;

-- name: ListConstructionUnprintedNodes :many
-- Every unprinted syntax node of a run, in sentence and source id order.
SELECT n.sentence_ordinal,
       n.source_id,
       n.dependency,
       n.head_token_ordinal,
       COALESCE(n.head_unprinted_node, '')::text AS head_unprinted_node
  FROM corpus_unprinted_syntax_nodes n
 WHERE n.owner_id=sqlc.arg('owner') AND n.analysis_run_id=sqlc.arg('analysis_run')
 ORDER BY n.sentence_ordinal, n.source_id;

-- name: DeleteConstructionOccurrences :exec
DELETE FROM construction_occurrences
 WHERE owner_id=sqlc.arg('owner') AND analysis_run_id=sqlc.arg('analysis_run');

-- name: InsertConstructionOccurrence :exec
INSERT INTO construction_occurrences
       (owner_id, corpus_id, analysis_run_id, sentence_ordinal, language,
        construction_id, subtype_id, anchor_token_ordinal, extent_token_ordinals)
VALUES (sqlc.arg('owner'), sqlc.arg('corpus'), sqlc.arg('analysis_run'), sqlc.arg('sentence_ordinal'),
        sqlc.arg('language'), sqlc.arg('construction_id'), sqlc.narg('subtype_id'),
        sqlc.arg('anchor_token_ordinal'), sqlc.arg('extent_token_ordinals')::bigint[]);

-- name: UpsertConstructionRunMarker :exec
INSERT INTO construction_run_markers (owner_id, corpus_id, analysis_run_id, detector_version)
VALUES (sqlc.arg('owner'), sqlc.arg('corpus'), sqlc.arg('analysis_run'), sqlc.arg('detector_version'))
ON CONFLICT (analysis_run_id) DO UPDATE
   SET detector_version=EXCLUDED.detector_version, built_at=now();
