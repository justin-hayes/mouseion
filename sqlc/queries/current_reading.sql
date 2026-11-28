-- Reading Journey and Primary Goal queries. Current analysis eligibility comes
-- from the current_analysis_identity view so the identity chain is not
-- duplicated in application SQL.

-- name: GetCurrentReadingBookID :one
SELECT book_id::text
FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language');

-- name: BookIsCurrentReading :one
-- Remove refuses the Book a reading is on. A Book can be current in more than
-- one study language, so this deliberately ignores the language.
SELECT EXISTS (
    SELECT 1 FROM primary_goals g
    WHERE g.owner_id = sqlc.arg('owner') AND g.book_id = sqlc.arg('book')
);

-- name: GetCurrentReading :one
SELECT g.owner_id::text, g.language, g.book_id::text, g.created_at, g.updated_at
FROM primary_goals g
WHERE g.owner_id = sqlc.arg('owner') AND g.language = sqlc.arg('language');

-- name: GetCurrentReadingForUpdate :one
SELECT g.owner_id::text, g.language, g.book_id::text, g.created_at, g.updated_at
FROM primary_goals g
WHERE g.owner_id = sqlc.arg('owner') AND g.language = sqlc.arg('language')
FOR UPDATE OF g;

-- name: GetCurrentReadingEvidence :one
-- The evidence the Current reading classifier reads under the Book lock: the
-- My Books row's analysis signals and language, plus the Book's disposition.
-- It selects only what eligibility needs, so the classifier does not depend on
-- display-only failure codes.
SELECT e.book_owner_id,
       e.book_id,
       e.book_language_state,
       e.book_language_tag,
       e.source_id,
       e.source_media_type,
       e.source_content_revision_id,
       e.source_content_snapshot_id,
       e.analysis_status,
       e.analysis_state,
       e.analysis_run_id,
       COALESCE(d.disposition::text, '')::text AS disposition
FROM my_books_evidence e
LEFT JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner') AND e.book_id = sqlc.arg('book');

-- name: GetCurrentReadingCandidateIdentity :one
-- Identity lookup only: eligibility is decided by the domain classifier before
-- this runs, so this query carries no disposition, language, or media rules.
SELECT ca.source_material_id::text, ca.analysis_run_id::text,
       ca.content_revision_id::text, ca.snapshot_id::text, ca.corpus_id::text
FROM current_analysis_identity ca
WHERE ca.owner_id = sqlc.arg('owner')
  AND ca.book_id = sqlc.arg('book');

-- name: InsertCurrentReading :one
-- Read next never freezes a vocabulary snapshot (ADR 0136 §4), so the row names
-- none.
INSERT INTO primary_goals(owner_id, language, book_id)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('book'))
ON CONFLICT (owner_id, language) DO NOTHING
RETURNING owner_id::text, language, book_id::text, created_at, updated_at;

-- name: ChangeCurrentReadingBook :one
-- Replaces the Book of the language's current reading.
UPDATE primary_goals
SET book_id = sqlc.arg('book'), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
RETURNING owner_id::text, language, book_id::text, created_at, updated_at;

-- name: DeleteCurrentReading :exec
DELETE FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language');

-- name: InsertReadingCompletion :exec
-- Finish appends one completion with no snapshot and no vocabulary counts
-- (ADR 0136 §4). Every Finish is a new row, so a Book can be completed again.
INSERT INTO reading_history(owner_id, language, book_id, completed_at)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('book'), sqlc.arg('completed_at'));

-- name: GetPreviouslyReadImport :one
SELECT owner_id::text, language, book_id::text, completed_at,
       COALESCE(goal_snapshot_id::text, '')::text AS goal_snapshot_id,
       snapshot_vocabulary_count, eligible_vocabulary_count,
       graduated_vocabulary_count, already_known_vocabulary_count, completion_source
FROM reading_history
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book')
  AND completion_source = 'previously_read_import'
ORDER BY completed_at DESC, completion_id DESC
LIMIT 1;

-- name: InsertPreviouslyReadImport :one
INSERT INTO reading_history(
    owner_id, language, book_id, completed_at, goal_snapshot_id,
    snapshot_vocabulary_count, eligible_vocabulary_count,
    graduated_vocabulary_count, already_known_vocabulary_count, completion_source
)
SELECT sqlc.arg('owner'), b.language_tag, b.id, sqlc.arg('completed_at'), NULL,
       0, 0, 0, 0, 'previously_read_import'
FROM books b
JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id AND m.state = 'active'
WHERE b.owner_id = sqlc.arg('owner') AND b.id = sqlc.arg('book')
  AND b.language_state = 'chosen'
  AND NOT EXISTS (
      SELECT 1 FROM primary_goals g WHERE g.owner_id = b.owner_id AND g.book_id = b.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM reading_history h WHERE h.owner_id = b.owner_id AND h.book_id = b.id
  )
ON CONFLICT DO NOTHING
RETURNING owner_id::text, language, book_id::text, completed_at,
          COALESCE(goal_snapshot_id::text, '')::text AS goal_snapshot_id,
          snapshot_vocabulary_count, eligible_vocabulary_count,
          graduated_vocabulary_count, already_known_vocabulary_count, completion_source;

