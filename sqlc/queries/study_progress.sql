-- Study progress, the review log and the per-day batch ledger (ADR 0134, spec
-- #2691).

-- name: ListStudyProgress :many
SELECT * FROM study_progress
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
 ORDER BY introduced_at, canonical_lemma, upos, lexical_discriminator;

-- name: LockStudyProgress :one
SELECT * FROM study_progress
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
 FOR UPDATE;

-- name: GetStudyBatchLedger :one
SELECT batches_taken, batches_declined FROM study_batch_ledger
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND study_day = sqlc.arg('study_day')::date;

-- name: TakeStudyBatchSlot :one
-- Records one batch taken on the Study day, unless the day's batches are all
-- answered. No row means none is left.
INSERT INTO study_batch_ledger (owner_id, language, study_day, batches_taken)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('study_day')::date, 1)
ON CONFLICT (owner_id, language, study_day) DO UPDATE
   SET batches_taken = study_batch_ledger.batches_taken + 1, updated_at = now()
 WHERE study_batch_ledger.batches_taken + study_batch_ledger.batches_declined < sqlc.arg('batch_limit')::int
RETURNING batches_taken;

-- name: DeclineStudyBatchSlot :one
INSERT INTO study_batch_ledger (owner_id, language, study_day, batches_declined)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('study_day')::date, 1)
ON CONFLICT (owner_id, language, study_day) DO UPDATE
   SET batches_declined = study_batch_ledger.batches_declined + 1, updated_at = now()
 WHERE study_batch_ledger.batches_taken + study_batch_ledger.batches_declined < sqlc.arg('batch_limit')::int
RETURNING batches_declined;

-- name: IntroduceStudyWord :one
-- Introduces a word from its ready, current Study material: the word enters
-- study new and due now, with the material's Gloss as its learned meaning. A
-- word already in study, or whose material is not ready and current, returns
-- no row.
INSERT INTO study_progress (owner_id, language, canonical_lemma, upos, lexical_discriminator, material_id, introduced_at, learned_meaning, state, due)
SELECT m.owner_id, m.language, m.canonical_lemma, m.upos, m.lexical_discriminator, m.id, sqlc.arg('now')::timestamptz, m.gloss, 'new', sqlc.arg('now')::timestamptz
  FROM study_material m
 WHERE m.owner_id = sqlc.arg('owner') AND m.id = sqlc.arg('material') AND m.language = sqlc.arg('language')
   AND m.state = 'ready' AND m.superseded_at IS NULL
ON CONFLICT (owner_id, language, canonical_lemma, upos, lexical_discriminator) DO NOTHING
RETURNING *;

-- name: UpdateStudyProgressMemory :exec
UPDATE study_progress
   SET state = sqlc.arg('state'), due = sqlc.arg('due'), stability = sqlc.arg('stability'),
       difficulty = sqlc.arg('difficulty'), scheduled_days = sqlc.arg('scheduled_days'),
       reps = sqlc.arg('reps'), lapses = sqlc.arg('lapses'), remaining_steps = sqlc.arg('remaining_steps'),
       last_review = sqlc.arg('last_review'), updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: AppendStudyReviewLog :exec
INSERT INTO study_review_log (owner_id, progress_id, material_id, grade, reviewed_at, study_day, state_before, state_after)
VALUES (sqlc.arg('owner'), sqlc.arg('progress'), sqlc.arg('material'), sqlc.arg('grade'), sqlc.arg('reviewed_at'),
        sqlc.arg('study_day')::date, sqlc.arg('state_before')::jsonb, sqlc.arg('state_after')::jsonb);

-- name: ListStudyReviewLog :many
SELECT * FROM study_review_log
 WHERE owner_id = sqlc.arg('owner') AND progress_id = sqlc.arg('progress')
 ORDER BY reviewed_at, created_at, id;
