-- Fluency contexts and responses (ADR 0134 §7, spec #2691). Fluency never
-- writes study_progress or study_review_log.

-- name: GetStudyProgress :one
SELECT * FROM study_progress
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: ListStudyFluencyContexts :many
-- Every context of the owner's words in the study language, with when the
-- learner last answered it; the Unix epoch means never.
SELECT c.id::text AS id, c.progress_id::text AS progress_id, c.agreement, c.reason,
       c.book_id::text AS book_id, c.analysis_run_id::text AS analysis_run_id, c.corpus_id::text AS corpus_id,
       c.sentence_ordinal, c.token_ordinal, c.sentence, c.target_word, c.gloss,
       c.sentence_translation, c.sentence_translation_targets,
       c.form_surface, c.form_features, c.form_split, c.created_at,
       COALESCE((SELECT max(r.responded_at) FROM study_fluency_response r
                  WHERE r.owner_id = c.owner_id AND r.context_id = c.id), 'epoch'::timestamptz)::timestamptz AS last_response_at
  FROM study_fluency_context c
  JOIN study_progress p ON p.owner_id = c.owner_id AND p.id = c.progress_id
 WHERE c.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language')
 ORDER BY c.created_at, c.id;

-- name: ListStudyFluencyContextResponses :many
-- Every answer to the owner's contexts in the study language, oldest first:
-- the evidence the weighted rotation reads.
SELECT r.context_id::text AS context_id, r.response, r.responded_at
  FROM study_fluency_response r
  JOIN study_fluency_context c ON c.owner_id = r.owner_id AND c.id = r.context_id
  JOIN study_progress p ON p.owner_id = c.owner_id AND p.id = c.progress_id
 WHERE r.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language')
 ORDER BY r.responded_at, r.created_at, r.id;

-- name: InsertStudyFluencyContext :one
-- Stores one assessed occurrence of a word. An agreeing context beyond the
-- word's limit, and an occurrence already assessed, return no row.
INSERT INTO study_fluency_context (owner_id, progress_id, agreement, reason, book_id, analysis_run_id, corpus_id, sentence_ordinal, token_ordinal,
                                   sentence, target_word, gloss, sentence_translation, sentence_translation_targets,
                                   form_surface, form_features, form_split)
SELECT p.owner_id, p.id, sqlc.arg('agreement'), sqlc.arg('reason'), sqlc.arg('book'), sqlc.arg('analysis_run'), sqlc.arg('corpus'),
       sqlc.arg('sentence_ordinal'), sqlc.arg('token_ordinal'), sqlc.arg('sentence'), sqlc.arg('target_word'),
       sqlc.arg('gloss'), sqlc.arg('sentence_translation'), sqlc.arg('sentence_translation_targets')::jsonb,
       sqlc.arg('form_surface'), sqlc.arg('form_features'), sqlc.arg('form_split')
  FROM study_progress p
 WHERE p.owner_id = sqlc.arg('owner') AND p.id = sqlc.arg('progress')
   AND (sqlc.arg('agreement')::text <> 'agrees'
        OR (SELECT count(*) FROM study_fluency_context x
             WHERE x.owner_id = p.owner_id AND x.progress_id = p.id AND x.agreement = 'agrees') < sqlc.arg('per_word')::int)
ON CONFLICT (owner_id, progress_id, analysis_run_id, sentence_ordinal, token_ordinal) DO NOTHING
RETURNING id::text AS id;

-- name: AppendStudyFluencyResponse :one
-- Appends the learner's answer to a context of theirs that agrees with the
-- learned meaning. A missing, foreign or disagreeing context returns no row.
INSERT INTO study_fluency_response (owner_id, context_id, response, responded_at)
SELECT c.owner_id, c.id, sqlc.arg('response'), sqlc.arg('responded_at')
  FROM study_fluency_context c
 WHERE c.owner_id = sqlc.arg('owner') AND c.id = sqlc.arg('context') AND c.agreement = 'agrees'
RETURNING id::text AS id;

-- name: ListStudyFluencyResponses :many
SELECT id::text AS id, context_id::text AS context_id, response, responded_at
  FROM study_fluency_response
 WHERE owner_id = sqlc.arg('owner') AND context_id = sqlc.arg('context')
 ORDER BY responded_at, created_at, id;
