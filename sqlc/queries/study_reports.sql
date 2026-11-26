-- Study reports (ADR 0135, spec #2714): a learner's report that a word's Study
-- material is wrong. See migration 000073.

-- name: CreateStudyReport :one
-- Records a report of the word's current Study material, copying the
-- occurrence, sentence, Gloss and translation from it. An Unsuitable-context
-- report also records what Undo restores. No row means the material is not
-- ready and current.
INSERT INTO study_report (owner_id, language, canonical_lemma, upos, lexical_discriminator, progress_id, material_id, kind, note,
                          book_id, analysis_run_id, corpus_id, sentence_ordinal, token_ordinal, sentence, target_word, gloss, sentence_translation,
                          previous_material_id, previous_corrected_at, previous_learned_meaning, created_at)
SELECT p.owner_id, p.language, p.canonical_lemma, p.upos, p.lexical_discriminator, p.id, m.id, sqlc.arg('kind')::text, sqlc.arg('note')::text,
       m.book_id, m.analysis_run_id, m.corpus_id, m.sentence_ordinal, m.token_ordinal, m.sentence, m.target_word, m.gloss, m.sentence_translation,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.material_id END,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.corrected_at END,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.learned_meaning END,
       sqlc.arg('created_at')::timestamptz
  FROM study_progress p
  JOIN study_material m ON m.owner_id = p.owner_id AND m.id = p.material_id
 WHERE p.owner_id = sqlc.arg('owner') AND p.id = sqlc.arg('progress')
   AND m.state = 'ready' AND m.superseded_at IS NULL
RETURNING *;

-- name: DeleteUnreviewedStudyProgress :execrows
-- A Wrong-word report of a word with no graded review leaves it unintroduced
-- (ADR 0135 §2): its Study progress goes, and the report keeps naming the word
-- (study_report.progress_id is set to null). A word with a graded review is
-- left alone.
DELETE FROM study_progress p
 WHERE p.owner_id = sqlc.arg('owner') AND p.id = sqlc.arg('id') AND p.reps = 0
   AND NOT EXISTS (SELECT 1 FROM study_review_log l WHERE l.owner_id = p.owner_id AND l.progress_id = p.id);

-- name: GetStudyReport :one
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: LockStudyReport :one
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
 FOR UPDATE;

-- name: GetOpenStudyReport :one
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND progress_id = sqlc.arg('progress')
   AND kind = sqlc.arg('kind') AND ended_at IS NULL
 ORDER BY created_at, id
 LIMIT 1;

-- name: GetOpenStudyReportOfMaterial :one
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND material_id = sqlc.arg('material') AND kind = sqlc.arg('kind') AND ended_at IS NULL;

-- name: ListOpenStudyReports :many
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND kind = sqlc.arg('kind') AND ended_at IS NULL
 ORDER BY created_at, id;

-- name: ListOpenStudyReportsForBook :many
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book') AND kind = sqlc.arg('kind') AND ended_at IS NULL
 ORDER BY created_at, id;

-- name: ListOpenStudyReportsOfWord :many
-- The open reports of one word; for an Unsuitable-context report these are the
-- word's retired occurrences.
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND progress_id = sqlc.arg('progress') AND kind = sqlc.arg('kind') AND ended_at IS NULL
 ORDER BY created_at, id;

-- name: EndStudyReport :execrows
-- Ends an open report. A report already ended is left as it is.
UPDATE study_report
   SET ended_at = sqlc.arg('now')::timestamptz, end_reason = sqlc.arg('reason')::text
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND ended_at IS NULL;

-- name: UpdateStudyProgressDue :exec
UPDATE study_progress
   SET due = sqlc.arg('due')::timestamptz, updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: UnsupersedeStudyMaterial :execrows
-- Returns a replaced Study material record to be the word's current one.
UPDATE study_material
   SET superseded_at = NULL, updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND superseded_at IS NOT NULL;

-- name: RestoreStudyProgressMaterial :exec
-- Undo of an Unsuitable-context report: the word goes back to the material,
-- learned meaning and correction time it had. The FSRS state was never touched.
UPDATE study_progress
   SET material_id = sqlc.arg('material'), learned_meaning = sqlc.arg('learned_meaning'),
       corrected_at = sqlc.narg('corrected_at')::timestamptz, updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: StudyActionSince :one
-- Whether the learner took any Study action in the language after the instant:
-- a graded review, a batch taken or declined, a Fluency response or another
-- report. Times are the database's own clock.
SELECT (
    EXISTS (SELECT 1 FROM study_review_log l
              JOIN study_progress p ON p.owner_id = l.owner_id AND p.id = l.progress_id
             WHERE l.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language') AND l.created_at > sqlc.arg('since')::timestamptz)
    OR EXISTS (SELECT 1 FROM study_batch_ledger b
                WHERE b.owner_id = sqlc.arg('owner') AND b.language = sqlc.arg('language') AND b.updated_at > sqlc.arg('since')::timestamptz)
    OR EXISTS (SELECT 1 FROM study_fluency_response f
                JOIN study_fluency_context c ON c.owner_id = f.owner_id AND c.id = f.context_id
                JOIN study_progress p ON p.owner_id = c.owner_id AND p.id = c.progress_id
               WHERE f.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language') AND f.created_at > sqlc.arg('since')::timestamptz)
    OR EXISTS (SELECT 1 FROM study_report r
                WHERE r.owner_id = sqlc.arg('owner') AND r.language = sqlc.arg('language') AND r.id <> sqlc.arg('report')
                  AND r.recorded_at > sqlc.arg('since')::timestamptz)
)::boolean AS acted;

-- name: SupersedeOtherCurrentStudyMaterialOfWord :execrows
-- Undo of an Unsuitable-context report: the word's current material, other than
-- the one being restored, is replaced so the restored record can be current.
UPDATE study_material
   SET superseded_at = now(), updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND canonical_lemma = sqlc.arg('canonical_lemma')
   AND upos = sqlc.arg('upos') AND lexical_discriminator = sqlc.arg('lexical_discriminator')
   AND superseded_at IS NULL AND id <> sqlc.arg('keep');

-- name: HasOpenWrongWordReport :one
-- Whether the word is held, for the grade that must not land on a held word.
SELECT EXISTS (SELECT 1 FROM study_report
                WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND canonical_lemma = sqlc.arg('canonical_lemma')
                  AND upos = sqlc.arg('upos') AND lexical_discriminator = sqlc.arg('lexical_discriminator')
                  AND kind = 'wrong_word' AND ended_at IS NULL)::boolean AS held;
