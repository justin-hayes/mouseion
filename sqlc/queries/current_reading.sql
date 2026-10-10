-- Reading Journey and Primary Goal queries. Current analysis eligibility comes
-- from the current_analysis_identity view so the identity chain is not
-- duplicated in application SQL.

-- name: GetCurrentReadingBookID :one
SELECT book_id::text
FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language');

-- name: GetCurrentReading :one
SELECT g.owner_id::text, g.language, g.book_id::text, g.created_at, g.updated_at,
       COALESCE(s.id::text, '')::text AS snapshot_id,
       COALESCE(s.source_material_id::text, '')::text AS source_material_id,
       COALESCE(s.analysis_run_id::text, '')::text AS analysis_run_id,
       COALESCE(s.content_revision_id::text, '')::text AS content_revision_id,
       COALESCE(s.content_snapshot_id::text, '')::text AS content_snapshot_id,
       COALESCE(s.corpus_id::text, '')::text AS corpus_id
FROM primary_goals g
LEFT JOIN primary_goal_snapshots s ON s.owner_id = g.owner_id AND s.id = g.snapshot_id
WHERE g.owner_id = sqlc.arg('owner') AND g.language = sqlc.arg('language');

-- name: GetCurrentReadingForUpdate :one
SELECT g.owner_id::text, g.language, g.book_id::text, g.created_at, g.updated_at,
       COALESCE(s.id::text, '')::text AS snapshot_id,
       COALESCE(s.source_material_id::text, '')::text AS source_material_id,
       COALESCE(s.analysis_run_id::text, '')::text AS analysis_run_id,
       COALESCE(s.content_revision_id::text, '')::text AS content_revision_id,
       COALESCE(s.content_snapshot_id::text, '')::text AS content_snapshot_id,
       COALESCE(s.corpus_id::text, '')::text AS corpus_id
FROM primary_goals g
LEFT JOIN primary_goal_snapshots s ON s.owner_id = g.owner_id AND s.id = g.snapshot_id
WHERE g.owner_id = sqlc.arg('owner') AND g.language = sqlc.arg('language')
FOR UPDATE OF g;

-- name: GetCurrentReadingCandidateIdentity :one
SELECT ca.source_material_id::text, ca.analysis_run_id::text,
       ca.content_revision_id::text, ca.snapshot_id::text, ca.corpus_id::text
FROM books b
JOIN book_dispositions bd ON bd.owner_id = b.owner_id AND bd.book_id = b.id
JOIN source_materials s ON s.owner_id = b.owner_id AND s.book_id = b.id
JOIN current_analysis_identity ca
  ON ca.owner_id = b.owner_id AND ca.book_id = b.id
 AND ca.source_material_id = s.id
WHERE b.owner_id = sqlc.arg('owner')
  AND b.id = sqlc.arg('book')
  AND bd.disposition = 'to_read'
  AND b.language_state = 'chosen'
  AND b.language_tag = sqlc.arg('language')::text
  AND lower(s.media_type) = 'application/epub+zip';

-- name: CreateCurrentReadingSnapshot :one
INSERT INTO primary_goal_snapshots(
    owner_id, language, book_id, source_material_id, analysis_run_id,
    content_revision_id, content_snapshot_id, corpus_id
)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('book'),
        sqlc.arg('source_material'), sqlc.arg('analysis_run'),
        sqlc.arg('content_revision'), sqlc.arg('content_snapshot'), sqlc.arg('corpus'))
RETURNING id::text, owner_id::text, language, book_id::text,
          source_material_id::text, analysis_run_id::text,
          content_revision_id::text, content_snapshot_id::text, corpus_id::text,
          created_at, released_at;

-- name: InsertCurrentReadingSnapshotVocabulary :exec
INSERT INTO primary_goal_snapshot_vocabulary(
    owner_id, snapshot_id, corpus_id, language, canonical_lemma, upos,
    occurrence_count, observed_forms, eligible_sentence_refs, provenance, first_encounter, selected_at
)
VALUES (sqlc.arg('owner'), sqlc.arg('snapshot'), sqlc.arg('corpus'), sqlc.arg('language'),
        sqlc.arg('canonical_lemma'), sqlc.arg('upos'), sqlc.arg('occurrence_count'),
        sqlc.arg('observed_forms'), sqlc.arg('eligible_sentence_refs'), sqlc.arg('provenance'),
        sqlc.arg('first_encounter'), sqlc.arg('selected_at'));

-- name: ListCurrentReadingSnapshotVocabulary :many
SELECT owner_id::text, snapshot_id::text, corpus_id, language, canonical_lemma, upos,
       occurrence_count, observed_forms, eligible_sentence_refs, provenance, first_encounter, selected_at
FROM primary_goal_snapshot_vocabulary
WHERE owner_id = sqlc.arg('owner') AND snapshot_id = sqlc.arg('snapshot')
ORDER BY language, canonical_lemma, upos;

-- name: ListCurrentReadingSnapshotCandidates :many
SELECT sc.owner_id::text AS owner_id,
       sc.corpus_id, sc.language, sc.canonical_lemma, sc.upos,
       sc.occurrence_count, sc.observed_forms, sc.eligible_sentence_refs,
       sc.provenance, sc.selected_at,
       COALESCE(first_seen.start_offset, 9223372036854775807::bigint)::bigint AS first_encounter
FROM selection_candidates sc
LEFT JOIN LATERAL (
  SELECT MIN(COALESCE(ref->'location'->>'start_offset', ref->'location'->>'StartOffset', ref->'Location'->>'StartOffset')::bigint) AS start_offset
  FROM jsonb_array_elements(sc.eligible_sentence_refs) ref
) first_seen ON true
WHERE sc.owner_id = sqlc.arg('owner')
  AND sc.corpus_id = sqlc.arg('corpus')
  AND sc.language = sqlc.arg('language')
  AND sc.occurrence_count >= 2
ORDER BY sc.language, sc.canonical_lemma, sc.upos;

-- name: IsCurrentReadingVocabularyReserved :one
SELECT EXISTS (
  SELECT 1 FROM primary_goal_snapshots ps
  JOIN primary_goal_snapshot_vocabulary pv
    ON pv.owner_id = ps.owner_id AND pv.snapshot_id = ps.id
  WHERE ps.owner_id = sqlc.arg('owner') AND ps.language = sqlc.arg('language')
    AND ps.released_at IS NULL AND pv.language = sqlc.arg('language')
    AND pv.canonical_lemma = sqlc.arg('canonical_lemma') AND pv.upos = sqlc.arg('upos')
);

-- name: GetActiveCurrentReadingSnapshotForPreparation :one
SELECT s.id::text, s.owner_id::text, s.language, s.book_id::text,
       s.source_material_id::text, s.analysis_run_id::text,
       s.content_revision_id::text, s.content_snapshot_id::text, s.corpus_id::text,
       s.created_at, s.released_at
FROM primary_goal_snapshots s
JOIN deck_preparations p ON p.owner_id = s.owner_id AND p.goal_snapshot_id = s.id
WHERE p.owner_id = sqlc.arg('owner') AND p.id = sqlc.arg('preparation')
  AND p.source_material_id = s.source_material_id
  AND p.analysis_run_id = s.analysis_run_id
;

-- name: GetCurrentReadingSnapshotLifecycle :one
-- Durable facts that prove a lifecycle replay: who the commitment belonged to,
-- when it was frozen and released, and whether it was completed.
SELECT s.book_id::text AS book_id, s.created_at, s.released_at,
       EXISTS (
           SELECT 1 FROM reading_history h
           WHERE h.owner_id = s.owner_id AND h.goal_snapshot_id = s.id
       ) AS completed
FROM primary_goal_snapshots s
WHERE s.owner_id = sqlc.arg('owner') AND s.language = sqlc.arg('language')
  AND s.id = sqlc.arg('snapshot');

-- name: LockCurrentReadingSnapshot :one
SELECT id::text
FROM primary_goal_snapshots
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('snapshot')
FOR UPDATE;

-- name: LockCurrentReadingsForBook :many
SELECT snapshot_id::text
FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND book_id = sqlc.arg('book') AND snapshot_id IS NOT NULL
FOR UPDATE;

-- name: LockCurrentReadingsExceptLanguage :many
SELECT snapshot_id::text
FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book') AND language <> sqlc.arg('language') AND snapshot_id IS NOT NULL
FOR UPDATE;

-- name: LockCurrentReadingsForAllLanguages :many
SELECT snapshot_id::text
FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book') AND snapshot_id IS NOT NULL
FOR UPDATE;

-- name: ReleaseCurrentReadingSnapshot :exec
UPDATE primary_goal_snapshots
SET released_at = COALESCE(released_at, now())
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('snapshot');

-- name: InsertCurrentReading :one
INSERT INTO primary_goals(owner_id, language, book_id, snapshot_id)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('book'), sqlc.arg('snapshot'))
ON CONFLICT (owner_id, language) DO NOTHING
RETURNING owner_id::text, language, book_id::text, created_at, updated_at, snapshot_id::text;

-- name: ChangeCurrentReadingBook :one
UPDATE primary_goals
SET book_id = sqlc.arg('book'), snapshot_id = sqlc.arg('snapshot'), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
RETURNING owner_id::text, language, book_id::text, created_at, updated_at, snapshot_id::text;

-- name: DeleteCurrentReading :exec
DELETE FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language');

-- name: GetReadingCompletion :one
SELECT owner_id::text, language, book_id::text, completed_at,
       COALESCE(goal_snapshot_id::text, '')::text AS goal_snapshot_id,
       completion_source,
        snapshot_vocabulary_count, eligible_vocabulary_count,
       graduated_vocabulary_count, already_known_vocabulary_count
FROM reading_history
WHERE owner_id = sqlc.arg('owner')
  AND language = sqlc.arg('language')
  AND book_id = sqlc.arg('book')
  AND goal_snapshot_id IS NOT DISTINCT FROM NULLIF(sqlc.arg('snapshot'), '')::uuid;

-- name: InsertReadingCompletion :one
INSERT INTO reading_history(
    owner_id, language, book_id, completed_at, goal_snapshot_id,
    snapshot_vocabulary_count, eligible_vocabulary_count,
    graduated_vocabulary_count, already_known_vocabulary_count
)
SELECT sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('book'), sqlc.arg('completed_at'),
       NULLIF(sqlc.arg('snapshot'), '')::uuid,
       sqlc.arg('snapshot_vocabulary_count'), sqlc.arg('eligible_vocabulary_count'),
       sqlc.arg('graduated_vocabulary_count'), sqlc.arg('already_known_vocabulary_count')
WHERE NULLIF(sqlc.arg('snapshot'), '') IS NOT NULL
   OR NOT EXISTS (
       SELECT 1
       FROM reading_history
       WHERE owner_id = sqlc.arg('owner')
         AND language = sqlc.arg('language')
         AND book_id = sqlc.arg('book')
         AND goal_snapshot_id IS NULL
   )
ON CONFLICT DO NOTHING
RETURNING owner_id::text, language, book_id::text, completed_at,
          COALESCE(goal_snapshot_id::text, '')::text AS goal_snapshot_id,
          snapshot_vocabulary_count, eligible_vocabulary_count,
           graduated_vocabulary_count, already_known_vocabulary_count, completion_source;

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

-- name: CountCurrentReadingSnapshotVocabulary :one
SELECT count(*)::int AS snapshot_count,
       count(*) FILTER (WHERE NOT EXISTS (
           SELECT 1 FROM known_vocabulary kv
           WHERE kv.owner_id = pv.owner_id
             AND kv.language = pv.language
             AND kv.canonical_lemma = pv.canonical_lemma
             AND (kv.upos = pv.upos OR kv.upos = '')
       ))::int AS eligible_count
FROM primary_goal_snapshot_vocabulary pv
WHERE pv.owner_id = sqlc.arg('owner')
  AND pv.snapshot_id = sqlc.arg('snapshot');

-- name: GraduateCurrentReadingSnapshotVocabulary :one
WITH eligible AS (
    SELECT pv.owner_id, pv.language, pv.canonical_lemma, pv.upos,
           s.book_id, s.source_material_id, s.analysis_run_id,
           s.content_revision_id, s.content_snapshot_id, s.corpus_id,
           s.id AS snapshot_id, sqlc.arg('completed_at')::timestamptz AS completed_at,
           preparation.id AS deck_preparation_id,
           generated.first_deck_id, generated.first_source_material_id,
           generated.first_generated_at
    FROM primary_goal_snapshot_vocabulary pv
    JOIN primary_goal_snapshots s
      ON s.owner_id = pv.owner_id AND s.id = pv.snapshot_id
    LEFT JOIN LATERAL (
        SELECT p.id
        FROM deck_preparations p
        WHERE p.owner_id = s.owner_id
          AND p.book_id = s.book_id
          AND (p.goal_snapshot_id = s.id OR (
               p.source_material_id = s.source_material_id
               AND (p.analysis_run_id = s.analysis_run_id OR p.analysis_run_id IS NULL)
          ))
        ORDER BY p.created_at DESC, p.id DESC
        LIMIT 1
    ) preparation ON true
    LEFT JOIN LATERAL (
        SELECT gv.first_deck_id, gv.first_source_material_id, gv.first_generated_at
        FROM generated_vocabulary gv
        WHERE gv.owner_id = pv.owner_id
          AND gv.language = pv.language
          AND gv.canonical_lemma = pv.canonical_lemma
          AND gv.upos = pv.upos
        ORDER BY gv.first_generated_at, gv.first_deck_id
        LIMIT 1
    ) generated ON true
    WHERE pv.owner_id = sqlc.arg('owner')
      AND pv.snapshot_id = sqlc.arg('snapshot')
      AND NOT EXISTS (
          SELECT 1 FROM known_vocabulary kv
          WHERE kv.owner_id = pv.owner_id
            AND kv.language = pv.language
            AND kv.canonical_lemma = pv.canonical_lemma
            AND (kv.upos = pv.upos OR kv.upos = '')
      )
), inserted AS (
    INSERT INTO known_vocabulary(
        owner_id, language, canonical_lemma, upos,
        completion_book_id, completion_at, completion_goal_snapshot_id,
        completion_source_material_id, completion_analysis_run_id,
        completion_content_revision_id, completion_content_snapshot_id,
        completion_corpus_id, completion_deck_preparation_id,
        generated_first_deck_id, generated_first_source_material_id,
        generated_first_at
    )
    SELECT owner_id, language, canonical_lemma, upos,
           book_id, completed_at, snapshot_id, source_material_id, analysis_run_id,
           content_revision_id, content_snapshot_id, corpus_id, deck_preparation_id,
           first_deck_id, first_source_material_id, first_generated_at
    FROM eligible
    ON CONFLICT DO NOTHING
    RETURNING owner_id
)
SELECT count(*)::int AS graduated_count FROM inserted;

-- name: UpdateReadingCompletionOutcome :exec
UPDATE reading_history
SET graduated_vocabulary_count = sqlc.arg('graduated_vocabulary_count'),
    already_known_vocabulary_count = sqlc.arg('already_known_vocabulary_count')
WHERE owner_id = sqlc.arg('owner')
  AND language = sqlc.arg('language')
  AND goal_snapshot_id = NULLIF(sqlc.arg('snapshot'), '')::uuid;

-- name: CurrentReadingCandidateEligible :one
SELECT EXISTS(
  SELECT 1
  FROM books b
  JOIN book_dispositions bd
    ON bd.owner_id = b.owner_id AND bd.book_id = b.id
  JOIN source_materials s
    ON s.owner_id = b.owner_id AND s.book_id = b.id
  JOIN current_analysis_identity ca
    ON ca.owner_id = b.owner_id
   AND ca.book_id = b.id
   AND ca.source_material_id = s.id
  WHERE b.owner_id = sqlc.arg('owner')
    AND b.id = sqlc.arg('book')
    AND bd.disposition = 'to_read'
    AND b.language_state = 'chosen'
    AND b.language_tag = sqlc.arg('language')::text
    AND lower(s.media_type) = 'application/epub+zip'
);
