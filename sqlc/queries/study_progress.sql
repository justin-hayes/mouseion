-- Study progress, the review log and the per-day batch ledger (ADR 0134, spec
-- #2691).

-- name: ListStudyProgress :many
-- reintroducing is true while the word's own Study material has been
-- superseded by a correction and its replacement is not yet in place.
-- awaiting_first_material is true while the word, migrated from Known
-- vocabulary as Already known, still points at the placeholder its first Study
-- material replaces (migration 000081): it is not a correction.
-- held is true while the word has an open Wrong-word report (ADR 0135 §2).
-- context_reported_at is when its latest Unsuitable-context report that retires
-- a context (not ended as undone) was made (ADR 0135 §3), the epoch when there
-- is none.
SELECT sqlc.embed(p), (m.superseded_at IS NOT NULL)::boolean AS reintroducing,
       (m.omission_reason IS NOT DISTINCT FROM 'known_vocabulary_backfill')::boolean AS awaiting_first_material,
       EXISTS (SELECT 1 FROM study_report r
                WHERE r.owner_id = p.owner_id AND r.item_kind = p.item_kind AND r.language = p.language AND r.canonical_lemma = p.canonical_lemma
                  AND r.upos = p.upos AND r.lexical_discriminator = p.lexical_discriminator
                  AND r.kind = 'wrong_word' AND r.ended_at IS NULL) AS held,
       COALESCE((SELECT max(r.created_at) FROM study_report r
                  WHERE r.owner_id = p.owner_id AND r.progress_id = p.id
                    AND r.kind = 'unsuitable_context' AND r.end_reason IS DISTINCT FROM 'undone'), 'epoch'::timestamptz)::timestamptz AS context_reported_at
  FROM study_progress p
  JOIN study_material m ON m.owner_id = p.owner_id AND m.id = p.material_id
 WHERE p.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language')
 ORDER BY p.introduced_at, p.canonical_lemma, p.upos, p.lexical_discriminator, p.item_kind;

-- name: CountStudyReviewsDue :one
-- The reviews still due on one Study day, for the Study tab's due chip. It is
-- domain.StudyCountsToday as one count: a review whose due time falls before
-- next_day_start and that was not answered since day_start, or a relearning
-- word; never a withdrawn word, a held one (open Wrong-word report), one whose
-- material was superseded, or one whose context was reported since day_start.
-- The caller passes domain.StudyDayStart and domain.NextStudyDayStart of now,
-- so SQL needs no time zone.
SELECT count(*)::bigint
  FROM study_progress p
  JOIN study_material m ON m.owner_id = p.owner_id AND m.id = p.material_id
 WHERE p.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language')
   AND p.withdrawn_at IS NULL
   AND m.superseded_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM study_report r
                    WHERE r.owner_id = p.owner_id AND r.item_kind = p.item_kind AND r.language = p.language AND r.canonical_lemma = p.canonical_lemma
                      AND r.upos = p.upos AND r.lexical_discriminator = p.lexical_discriminator
                      AND r.kind = 'wrong_word' AND r.ended_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM study_report r
                    WHERE r.owner_id = p.owner_id AND r.progress_id = p.id
                      AND r.kind = 'unsuitable_context' AND r.end_reason IS DISTINCT FROM 'undone'
                      AND r.created_at >= sqlc.arg('day_start')::timestamptz)
   AND ((p.state = 'review'
         AND p.due < sqlc.arg('next_day_start')::timestamptz
         AND (p.last_review IS NULL OR p.last_review < sqlc.arg('day_start')::timestamptz))
        OR p.state = 'relearning');

-- name: LockStudyProgress :one
SELECT * FROM study_progress
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
 FOR UPDATE;

-- name: GetStudyBatchLedger :one
SELECT batches_offered, batches_taken, batches_declined FROM study_batch_ledger
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND study_day = sqlc.arg('study_day')::date;

-- name: TakeStudyBatchSlot :one
-- Records one batch taken on the Study day under the limit on distinct batches
-- offered. A batch declined earlier is taken in place of the declined one and
-- consumes no other. No row means none is left. Mirrors
-- domain.StudyBatchLedger.Take.
INSERT INTO study_batch_ledger (owner_id, language, study_day, batches_offered, batches_taken)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('study_day')::date, 1, 1)
ON CONFLICT (owner_id, language, study_day) DO UPDATE
   SET batches_taken = study_batch_ledger.batches_taken + 1,
       batches_declined = GREATEST(study_batch_ledger.batches_declined - 1, 0),
       batches_offered = GREATEST(study_batch_ledger.batches_offered,
           study_batch_ledger.batches_taken + study_batch_ledger.batches_declined + CASE WHEN study_batch_ledger.batches_declined > 0 THEN 0 ELSE 1 END),
       updated_at = now()
 WHERE study_batch_ledger.batches_taken + study_batch_ledger.batches_declined < sqlc.arg('batch_limit')::int
    OR (study_batch_ledger.batches_declined > 0 AND study_batch_ledger.batches_taken < sqlc.arg('batch_limit')::int)
RETURNING batches_taken;

-- name: DeclineStudyBatchSlot :one
INSERT INTO study_batch_ledger (owner_id, language, study_day, batches_offered, batches_declined)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('study_day')::date, 1, 1)
ON CONFLICT (owner_id, language, study_day) DO UPDATE
   SET batches_declined = study_batch_ledger.batches_declined + 1,
       batches_offered = GREATEST(study_batch_ledger.batches_offered, study_batch_ledger.batches_taken + study_batch_ledger.batches_declined + 1),
       updated_at = now()
 WHERE study_batch_ledger.batches_taken + study_batch_ledger.batches_declined < sqlc.arg('batch_limit')::int
RETURNING batches_declined;

-- name: RecordStudyBatchOffer :exec
-- Records that Today has offered this many batches on the Study day. The count
-- only grows, so showing the same offer again changes nothing.
-- An offer is not a Study action, so it leaves updated_at, which Undo reads as
-- the time of the last batch taken or declined, alone (a new row starts at the
-- epoch).
INSERT INTO study_batch_ledger (owner_id, language, study_day, batches_offered, updated_at)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('study_day')::date, sqlc.arg('offered')::int, 'epoch'::timestamptz)
ON CONFLICT (owner_id, language, study_day) DO UPDATE
   SET batches_offered = GREATEST(study_batch_ledger.batches_offered, EXCLUDED.batches_offered);

-- name: IntroduceStudyWord :one
-- Introduces a word from its ready, current Study material: the word enters
-- study new and due now, with the material's Gloss, citations and dictionary
-- pin as its learned meaning. A word already in study, held by an open
-- Wrong-word report, or whose material is not ready and current, returns no
-- row.
INSERT INTO study_progress (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, material_id, introduced_at, learned_meaning,
                            learned_sense_ids, learned_entry_ids, learned_dictionary_pin, state, due)
SELECT m.owner_id, m.item_kind, m.language, m.canonical_lemma, m.upos, m.lexical_discriminator, m.id, sqlc.arg('now')::timestamptz, m.gloss,
       m.cited_sense_ids, m.cited_entry_ids, m.dictionary_pin, 'new', sqlc.arg('now')::timestamptz
  FROM study_material m
 WHERE m.owner_id = sqlc.arg('owner') AND m.id = sqlc.arg('material') AND m.language = sqlc.arg('language')
   AND m.state = 'ready' AND m.superseded_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM study_report r
                    WHERE r.owner_id = m.owner_id AND r.item_kind = m.item_kind AND r.language = m.language AND r.canonical_lemma = m.canonical_lemma
                      AND r.upos = m.upos AND r.lexical_discriminator = m.lexical_discriminator
                      AND r.kind = 'wrong_word' AND r.ended_at IS NULL)
ON CONFLICT (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator) DO NOTHING
RETURNING *;

-- name: TakeCarriedStudyWord :one
-- Takes a word again in today's batch: a word introduced on an earlier Study
-- day and never answered is introduced afresh, new and due now. A word that
-- was answered, withdrawn or introduced today returns no row.
UPDATE study_progress
   SET introduced_at = sqlc.arg('now')::timestamptz, due = sqlc.arg('now')::timestamptz, updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND language = sqlc.arg('language')
   AND state = 'new' AND reps = 0 AND withdrawn_at IS NULL
   AND introduced_at < sqlc.arg('day_start')::timestamptz
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

-- name: SupersedeStudyMaterial :execrows
-- A correction left the word's introducing occurrence resolving elsewhere:
-- its current material is replaced, so the word has none and new material is
-- prepared. The superseded record is kept for the review log.
UPDATE study_material
   SET superseded_at = now(), updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND superseded_at IS NULL;

-- name: SetStudyProgressWithdrawn :execrows
UPDATE study_progress
   SET withdrawn_at = sqlc.narg('withdrawn_at')::timestamptz, withdrawn_reason = NULL, updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
   AND (withdrawn_at IS NULL) <> (sqlc.narg('withdrawn_at')::timestamptz IS NULL);

-- name: CompleteStudyReintroductions :execrows
-- Moves each word whose replacement material is ready onto it: the new
-- Gloss becomes the learned meaning and the word is marked corrected, unless
-- the replaced material is the placeholder of a word migrated from Known
-- vocabulary: that is its first material, not a correction. The FSRS state is
-- not touched.
UPDATE study_progress p
   SET material_id = m.id, learned_meaning = m.gloss, learned_sense_ids = m.cited_sense_ids,
       learned_entry_ids = m.cited_entry_ids, learned_dictionary_pin = m.dictionary_pin,
       corrected_at = CASE WHEN old.omission_reason IS DISTINCT FROM 'known_vocabulary_backfill' THEN sqlc.arg('now')::timestamptz ELSE p.corrected_at END,
       updated_at = now()
  FROM study_material m, study_material old
 WHERE p.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language') AND p.withdrawn_at IS NULL
   AND old.owner_id = p.owner_id AND old.id = p.material_id
   AND m.owner_id = p.owner_id AND m.item_kind = p.item_kind AND m.language = p.language AND m.canonical_lemma = p.canonical_lemma
   AND m.upos = p.upos AND m.lexical_discriminator = p.lexical_discriminator
   AND m.superseded_at IS NULL AND m.state = 'ready' AND m.id <> p.material_id;

-- name: WithdrawUnpreparableStudyReintroductions :execrows
-- A word whose replacement material cannot be prepared has no usable
-- occurrence left: it is withdrawn, with its progress kept. A word whose
-- contexts the learner reported as unsuitable is withdrawn with the reason
-- 'no_suitable_context' (ADR 0135 §3): its other contexts are retired or fail
-- the sentence-quality gate.
UPDATE study_progress p
   SET withdrawn_at = sqlc.arg('now')::timestamptz, updated_at = now(),
       withdrawn_reason = CASE WHEN EXISTS (SELECT 1 FROM study_report r
                                             WHERE r.owner_id = p.owner_id AND r.progress_id = p.id
                                               AND r.kind = 'unsuitable_context' AND r.end_reason IS DISTINCT FROM 'undone')
                               THEN 'no_suitable_context' END
  FROM study_material m
 WHERE p.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language') AND p.withdrawn_at IS NULL
   AND m.owner_id = p.owner_id AND m.item_kind = p.item_kind AND m.language = p.language AND m.canonical_lemma = p.canonical_lemma
   AND m.upos = p.upos AND m.lexical_discriminator = p.lexical_discriminator
   AND m.superseded_at IS NULL AND m.state = 'unpreparable' AND m.id <> p.material_id;

-- name: InsertStudyAlreadyKnownEvent :exec
-- An Already-known answer or backfilled word: an event, never a grade. The
-- item identity is copied from the word's progress, so the event is keyed like
-- Study progress.
INSERT INTO study_already_known_event (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, progress_id, material_id, source, answered_at, study_day)
SELECT p.owner_id, p.item_kind, p.language, p.canonical_lemma, p.upos, p.lexical_discriminator, p.id, p.material_id,
       sqlc.arg('source'), sqlc.arg('answered_at'), sqlc.arg('study_day')::date
  FROM study_progress p
 WHERE p.owner_id = sqlc.arg('owner') AND p.id = sqlc.arg('progress');

-- name: ListStudyAlreadyKnownEvents :many
SELECT * FROM study_already_known_event
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
 ORDER BY answered_at, created_at, id;
