-- Study reports (ADR 0135, spec #2714): a learner's report that a word's Study
-- material is wrong. See migration 000073.

-- name: CreateStudyReport :one
-- Records a report of the word's current Study material, copying the
-- occurrence, sentence, Gloss and translation from it. An Unsuitable-context
-- report also records what Undo restores. No row means the material is not
-- ready and current.
INSERT INTO study_report (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, progress_id, material_id, kind, note,
                          book_id, analysis_run_id, corpus_id, sentence_ordinal, token_ordinal, sentence, target_word, gloss, sentence_translation,
                          previous_material_id, previous_corrected_at, previous_learned_meaning,
                          previous_learned_sense_ids, previous_learned_entry_ids, previous_learned_dictionary_pin, created_at)
SELECT p.owner_id, p.item_kind, p.language, p.canonical_lemma, p.upos, p.lexical_discriminator, p.id, m.id, sqlc.arg('kind')::text, sqlc.arg('note')::text,
       m.book_id, m.analysis_run_id, m.corpus_id, m.sentence_ordinal, m.token_ordinal, m.sentence, m.target_word, m.gloss, m.sentence_translation,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.material_id END,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.corrected_at END,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.learned_meaning END,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.learned_sense_ids END,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.learned_entry_ids END,
       CASE WHEN sqlc.arg('kind')::text = 'unsuitable_context' THEN p.learned_dictionary_pin END,
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

-- name: ListRetiredStudyReportsOfWord :many
-- The Unsuitable-context reports that retire one word's occurrences: those not
-- ended as undone (ADR 0135 §3). Ending a report because the learner moved on
-- keeps its sentence retired.
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND progress_id = sqlc.arg('progress') AND kind = 'unsuitable_context'
   AND end_reason IS DISTINCT FROM 'undone'
 ORDER BY created_at, id;

-- name: ListRetiredStudyReportsOfWords :many
-- The same for several words, keyed by their Study progress.
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND progress_id = ANY (sqlc.arg('progress')::uuid[]) AND kind = 'unsuitable_context'
   AND end_reason IS DISTINCT FROM 'undone'
 ORDER BY created_at, id;

-- name: ListRetiredStudyReportsOfIdentity :many
-- The same by the word's vocabulary identity, which the word's material
-- preparation knows.
SELECT * FROM study_report
 WHERE owner_id = sqlc.arg('owner') AND item_kind = sqlc.arg('item_kind') AND language = sqlc.arg('language') AND canonical_lemma = sqlc.arg('canonical_lemma')
   AND upos = sqlc.arg('upos') AND lexical_discriminator = sqlc.arg('lexical_discriminator')
   AND kind = 'unsuitable_context' AND end_reason IS DISTINCT FROM 'undone'
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
-- learned meaning and correction time it had, and a withdrawal for want of a
-- suitable context ends. The FSRS state was never touched.
UPDATE study_progress
   SET material_id = sqlc.arg('material'), learned_meaning = sqlc.arg('learned_meaning'),
       learned_sense_ids = sqlc.arg('learned_sense_ids')::text[], learned_entry_ids = sqlc.arg('learned_entry_ids')::text[],
       learned_dictionary_pin = sqlc.arg('learned_dictionary_pin'),
       corrected_at = sqlc.narg('corrected_at')::timestamptz,
       withdrawn_at = CASE WHEN withdrawn_reason = 'no_suitable_context' THEN NULL ELSE withdrawn_at END,
       withdrawn_reason = NULL, updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: StudyActionSince :one
-- Whether the learner took any Study action in the language after the instant:
-- a graded review, an Already-known answer, a batch taken or declined, a Fluency response or another
-- report. Times are the database's own clock.
SELECT (
    EXISTS (SELECT 1 FROM study_review_log l
              JOIN study_progress p ON p.owner_id = l.owner_id AND p.id = l.progress_id
             WHERE l.owner_id = sqlc.arg('owner') AND p.language = sqlc.arg('language') AND l.created_at > sqlc.arg('since')::timestamptz)
    OR EXISTS (SELECT 1 FROM study_already_known_event k
                WHERE k.owner_id = sqlc.arg('owner') AND k.language = sqlc.arg('language') AND k.created_at > sqlc.arg('since')::timestamptz)
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
 WHERE owner_id = sqlc.arg('owner') AND item_kind = sqlc.arg('item_kind') AND language = sqlc.arg('language') AND canonical_lemma = sqlc.arg('canonical_lemma')
   AND upos = sqlc.arg('upos') AND lexical_discriminator = sqlc.arg('lexical_discriminator')
   AND superseded_at IS NULL AND id <> sqlc.arg('keep');

-- name: HasOpenWrongWordReport :one
-- Whether the word is held, for the grade that must not land on a held word.
SELECT EXISTS (SELECT 1 FROM study_report
                WHERE owner_id = sqlc.arg('owner') AND item_kind = sqlc.arg('item_kind') AND language = sqlc.arg('language') AND canonical_lemma = sqlc.arg('canonical_lemma')
                  AND upos = sqlc.arg('upos') AND lexical_discriminator = sqlc.arg('lexical_discriminator')
                  AND kind = 'wrong_word' AND ended_at IS NULL)::boolean AS held;

-- name: GetStudyOccurrenceToken :one
-- The token a retired occurrence names, with the locator and the surface and
-- source lemma an Analysis update carries a decision across runs by (ADR 0126).
SELECT s.unit_id::text AS unit_id, t.start_offset::bigint AS start_offset, t.end_offset::bigint AS end_offset,
       t.surface, t.raw_lemma
  FROM corpus_tokens t
  JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.analysis_run_id = t.analysis_run_id
   AND s.corpus_id = t.corpus_id AND s.sentence_ordinal = t.sentence_ordinal
 WHERE t.owner_id = sqlc.arg('owner') AND t.analysis_run_id = sqlc.arg('analysis_run') AND t.corpus_id = sqlc.arg('corpus')
   AND t.sentence_ordinal = sqlc.arg('sentence_ordinal') AND t.token_ordinal = sqlc.arg('token_ordinal');

-- name: StudyMaterialIsSuperseded :one
-- Whether the material the word is reviewed in has been replaced, so a grade of
-- its card is stale.
SELECT (m.superseded_at IS NOT NULL)::boolean AS superseded
  FROM study_progress p
  JOIN study_material m ON m.owner_id = p.owner_id AND m.id = p.material_id
 WHERE p.owner_id = sqlc.arg('owner') AND p.id = sqlc.arg('id');
