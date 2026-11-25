-- Study (ADR 0134, spec #2691): the stream of new-word candidates and the
-- Study material prepared for them.

-- name: ListStudyCandidates :many
-- Effective vocabulary identities of the owner's currently analyzed Books in
-- the study language, with their Personal analyzed-Book occurrences and whether
-- the Current reading contributes them. The counts are the Browse count
-- projection, which already applies confirmed Lemma corrections and Occurrence
-- exclusions. A Book whose projection is not ready contributes nothing yet.
-- Known and Reserved vocabulary are deliberately not consulted (ADR 0134 §3).
-- The builder version is the one Browse reads.
SELECT c.canonical_lemma::text AS canonical_lemma,
       c.upos::text AS upos,
       c.lexical_discriminator::text AS lexical_discriminator,
       sum(c.occurrence_count)::bigint AS personal_occurrences,
       COALESCE(bool_or(c.book_id = g.book_id), false)::boolean AS in_current_reading
  FROM vocabulary_browse_counts c
  JOIN current_analysis_identity cai ON cai.owner_id = c.owner_id AND cai.book_id = c.book_id
   AND cai.analysis_run_id = c.analysis_run_id AND cai.corpus_id = c.corpus_id
  JOIN books b ON b.owner_id = c.owner_id AND b.id = c.book_id
   AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
  JOIN vocabulary_browse_count_readiness r ON r.owner_id = c.owner_id AND r.book_id = c.book_id
   AND r.analysis_run_id = c.analysis_run_id AND r.corpus_id = c.corpus_id
   AND r.language = sqlc.arg('language')::text AND r.builder_version = 4
  LEFT JOIN primary_goals g ON g.owner_id = c.owner_id AND g.language = sqlc.arg('language')::text
 WHERE c.owner_id = sqlc.arg('owner') AND c.language = sqlc.arg('language')::text
 GROUP BY c.canonical_lemma, c.upos, c.lexical_discriminator;

-- name: ListCurrentStudyMaterial :many
SELECT id::text AS id, language, canonical_lemma, upos, lexical_discriminator, state, omission_reason,
       COALESCE(book_id::text, '')::text AS book_id,
       COALESCE(analysis_run_id::text, '')::text AS analysis_run_id,
       COALESCE(corpus_id::text, '')::text AS corpus_id,
       COALESCE(sentence_ordinal, 0)::bigint AS sentence_ordinal,
       COALESCE(token_ordinal, 0)::bigint AS token_ordinal,
       sentence, target_word, gloss, sentence_translation, sentence_translation_targets,
       created_at, updated_at
  FROM study_material
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND superseded_at IS NULL
 ORDER BY canonical_lemma, upos, lexical_discriminator;

-- name: GetStudyMaterial :one
SELECT id::text AS id, language, canonical_lemma, upos, lexical_discriminator, state, omission_reason,
       COALESCE(book_id::text, '')::text AS book_id,
       COALESCE(analysis_run_id::text, '')::text AS analysis_run_id,
       COALESCE(corpus_id::text, '')::text AS corpus_id,
       COALESCE(sentence_ordinal, 0)::bigint AS sentence_ordinal,
       COALESCE(token_ordinal, 0)::bigint AS token_ordinal,
       sentence, target_word, gloss, sentence_translation, sentence_translation_targets,
       created_at, updated_at
  FROM study_material
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: CountCurrentStudyMaterial :many
SELECT state, count(*)::bigint AS total
  FROM study_material
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND superseded_at IS NULL
 GROUP BY state;

-- name: ReserveStudyMaterial :one
-- Starts a word's material as pending. A word that already has current
-- material returns no row.
INSERT INTO study_material (owner_id, language, canonical_lemma, upos, lexical_discriminator, state)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('canonical_lemma'), sqlc.arg('upos'), sqlc.arg('lexical_discriminator'), 'pending')
ON CONFLICT (owner_id, language, canonical_lemma, upos, lexical_discriminator) WHERE superseded_at IS NULL DO NOTHING
RETURNING id::text AS id;

-- name: MarkStudyMaterialReady :execrows
UPDATE study_material
   SET state = 'ready', book_id = sqlc.arg('book'), analysis_run_id = sqlc.arg('analysis_run'),
       corpus_id = sqlc.arg('corpus'), sentence_ordinal = sqlc.arg('sentence_ordinal'),
       token_ordinal = sqlc.arg('token_ordinal'), sentence = sqlc.arg('sentence'),
       target_word = sqlc.arg('target_word'), gloss = sqlc.arg('gloss'),
       sentence_translation = sqlc.arg('sentence_translation'),
       sentence_translation_targets = sqlc.arg('sentence_translation_targets')::jsonb,
       updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'pending';

-- name: MarkStudyMaterialUnpreparable :execrows
UPDATE study_material
   SET state = 'unpreparable', omission_reason = sqlc.arg('reason'), updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'pending';
