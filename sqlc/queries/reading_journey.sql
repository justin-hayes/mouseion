-- Reading Journey and Primary Goal queries. Current analysis eligibility comes
-- from the current_analysis_identity view so the identity chain is not
-- duplicated in application SQL.

-- name: GetReadingJourney :one
SELECT revision, updated_at
FROM reading_journeys
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language');

-- name: ListReadingJourneyMembers :many
SELECT m.book_id::text AS book_id, m.position, m.created_at
FROM reading_journey_membership m
JOIN books b
  ON b.owner_id = m.owner_id AND b.id = m.book_id
 AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
WHERE m.owner_id = sqlc.arg('owner') AND m.language = sqlc.arg('language')::text
ORDER BY m.position, m.created_at, m.book_id;

-- name: InsertReadingJourneyIfAbsent :exec
INSERT INTO reading_journeys(owner_id, language)
VALUES (sqlc.arg('owner'), sqlc.arg('language'))
ON CONFLICT (owner_id, language) DO NOTHING;

-- name: GetReadingJourneyRevisionForUpdate :one
SELECT revision
FROM reading_journeys
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
FOR UPDATE;

-- name: DeleteNonChosenJourneyMembers :execrows
DELETE FROM reading_journey_membership m
USING books b
WHERE m.owner_id = sqlc.arg('owner')
  AND m.language = sqlc.arg('language')::text
  AND m.book_id = b.id
  AND b.owner_id = sqlc.arg('owner')
  AND (b.language_state <> 'chosen' OR b.language_tag <> sqlc.arg('language')::text);

-- name: ListReadingJourneyMembersForUpdate :many
SELECT m.book_id::text AS book_id, m.position, m.created_at
FROM reading_journey_membership m
JOIN books b
  ON b.owner_id = m.owner_id AND b.id = m.book_id
 AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
WHERE m.owner_id = sqlc.arg('owner') AND m.language = sqlc.arg('language')::text
ORDER BY m.position, m.created_at, m.book_id
FOR UPDATE;

-- name: ListAllReadingJourneyMembersForUpdate :many
SELECT book_id::text AS book_id, position, created_at
FROM reading_journey_membership
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
ORDER BY position, created_at, book_id
FOR UPDATE;

-- name: UpsertReadingJourneyMemberPosition :exec
INSERT INTO reading_journey_membership(owner_id, language, book_id, position)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('book'), sqlc.arg('position'))
ON CONFLICT (owner_id, language, book_id)
DO UPDATE SET position = EXCLUDED.position;

-- name: BumpReadingJourneyRevision :one
UPDATE reading_journeys
SET revision = revision + 1, updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
RETURNING revision;

-- name: GetBookLanguageState :one
SELECT language_state, COALESCE(language_tag, '') AS language_tag
FROM books
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('book');

-- name: BookExists :one
SELECT EXISTS(
  SELECT 1 FROM books
  WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('book')
);

-- name: ResolveJourneyLinkedBook :one
SELECT (COALESCE(sm.book_id::text, b.id::text, ''))::text AS linked_book_id
FROM source_materials sm
LEFT JOIN book_aliases a
  ON a.owner_id = sm.owner_id
 AND a.namespace = sqlc.arg('namespace')
 AND a.value = sm.source_identifier
LEFT JOIN books b ON b.owner_id = a.owner_id AND b.id = a.book_id
WHERE sm.owner_id = sqlc.arg('owner') AND sm.id = sqlc.arg('source');

-- name: DerivedJourneyBooksExist :one
SELECT EXISTS(
  SELECT 1
  FROM books b
  JOIN book_membership bm
    ON bm.owner_id = b.owner_id AND bm.book_id = b.id AND bm.state = 'active'
  WHERE b.owner_id = sqlc.arg('owner')
    AND b.language_state = 'chosen'
    AND b.language_tag = sqlc.arg('language')::text
);

-- name: DeleteReadingJourney :exec
DELETE FROM reading_journeys
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language');

-- name: DeleteReadingJourneyMember :exec
DELETE FROM reading_journey_membership
WHERE owner_id = sqlc.arg('owner')
  AND language = sqlc.arg('language')
  AND book_id = sqlc.arg('book');

-- name: DeletePrimaryGoalForBook :exec
DELETE FROM primary_goals
WHERE owner_id = sqlc.arg('owner')
  AND language = sqlc.arg('language')
  AND book_id = sqlc.arg('book');

-- name: GetPrimaryGoalBookID :one
SELECT book_id::text
FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language');

-- name: GetPrimaryGoal :one
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

-- name: GetPrimaryGoalForUpdate :one
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

-- name: GetPrimaryGoalCandidateIdentity :one
SELECT ca.source_material_id::text, ca.analysis_run_id::text,
       ca.content_revision_id::text, ca.snapshot_id::text, ca.corpus_id::text
FROM reading_journey_membership jm
JOIN books b ON b.owner_id = jm.owner_id AND b.id = jm.book_id
JOIN source_materials s ON s.owner_id = jm.owner_id AND s.book_id = jm.book_id
JOIN current_analysis_identity ca
  ON ca.owner_id = jm.owner_id AND ca.book_id = jm.book_id
 AND ca.source_material_id = s.id
WHERE jm.owner_id = sqlc.arg('owner')
  AND jm.language = sqlc.arg('language')
  AND jm.book_id = sqlc.arg('book')
  AND b.language_state = 'chosen'
  AND b.language_tag = sqlc.arg('language')
  AND lower(s.media_type) = 'application/epub+zip'
  AND NOT EXISTS (
    SELECT 1 FROM reading_history h
    WHERE h.owner_id = jm.owner_id
      AND h.language = jm.language
      AND h.book_id = jm.book_id
  );

-- name: CreatePrimaryGoalSnapshot :one
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

-- name: InsertPrimaryGoalSnapshotVocabulary :exec
INSERT INTO primary_goal_snapshot_vocabulary(
    owner_id, snapshot_id, corpus_id, language, canonical_lemma, upos,
    occurrence_count, observed_forms, eligible_sentence_refs, provenance, first_encounter, selected_at
)
VALUES (sqlc.arg('owner'), sqlc.arg('snapshot'), sqlc.arg('corpus'), sqlc.arg('language'),
        sqlc.arg('canonical_lemma'), sqlc.arg('upos'), sqlc.arg('occurrence_count'),
        sqlc.arg('observed_forms'), sqlc.arg('eligible_sentence_refs'), sqlc.arg('provenance'),
        sqlc.arg('first_encounter'), sqlc.arg('selected_at'));

-- name: ListPrimaryGoalSnapshotVocabulary :many
SELECT owner_id::text, snapshot_id::text, corpus_id, language, canonical_lemma, upos,
       occurrence_count, observed_forms, eligible_sentence_refs, provenance, first_encounter, selected_at
FROM primary_goal_snapshot_vocabulary
WHERE owner_id = sqlc.arg('owner') AND snapshot_id = sqlc.arg('snapshot')
ORDER BY language, canonical_lemma, upos;

-- name: ListPrimaryGoalSnapshotCandidates :many
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
  AND sc.occurrence_count >= 3
  AND NOT EXISTS (
    SELECT 1 FROM known_vocabulary kv
    WHERE kv.owner_id = sc.owner_id AND kv.language = sc.language
      AND kv.canonical_lemma = sc.canonical_lemma
      AND (kv.upos = sc.upos OR kv.upos = '')
  )
  AND NOT EXISTS (
    SELECT 1 FROM vocabulary_states vs
    WHERE vs.owner_id = sc.owner_id AND vs.language = sc.language
      AND vs.canonical_lemma = sc.canonical_lemma AND vs.upos = sc.upos
      AND vs.state = 'known'
  )
  AND NOT EXISTS (
    SELECT 1
    FROM primary_goal_snapshots ps
    JOIN primary_goal_snapshot_vocabulary pv
      ON pv.owner_id = ps.owner_id AND pv.snapshot_id = ps.id
    WHERE ps.owner_id = sc.owner_id AND ps.released_at IS NULL
      AND pv.language = sc.language
      AND pv.canonical_lemma = sc.canonical_lemma
      AND pv.upos = sc.upos
  )
ORDER BY sc.language, sc.canonical_lemma, sc.upos;

-- name: GetActivePrimaryGoalSnapshotForPreparation :one
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

-- name: ReleasePrimaryGoalSnapshot :exec
UPDATE primary_goal_snapshots
SET released_at = COALESCE(released_at, now())
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('snapshot');

-- name: ReleaseDeckStudiesForPrimaryGoalSnapshot :exec
UPDATE deck_preparations p
SET studying_at = NULL, released_at = COALESCE(p.released_at, now()), updated_at = now()
FROM primary_goal_snapshots s
WHERE s.owner_id = sqlc.arg('owner') AND s.id = sqlc.arg('snapshot')
  AND p.owner_id = s.owner_id AND p.book_id = s.book_id
  AND p.source_material_id = s.source_material_id
  AND p.analysis_run_id = s.analysis_run_id
  AND p.studying_at IS NOT NULL AND p.graduated_at IS NULL
  AND EXISTS (
      SELECT 1 FROM deck_preparation_vocabulary dv
      WHERE dv.owner_id = p.owner_id AND dv.deck_preparation_id = p.id
        AND dv.language = s.language AND dv.graduated_at IS NULL
  );

-- name: ReleasePrimaryGoalSnapshotsForBook :exec
UPDATE primary_goal_snapshots s
SET released_at = COALESCE(s.released_at, now())
FROM primary_goals g
WHERE g.owner_id = sqlc.arg('owner') AND g.book_id = sqlc.arg('book')
  AND g.language = sqlc.arg('language')
  AND s.owner_id = g.owner_id AND s.id = g.snapshot_id;

-- name: ReleasePrimaryGoalSnapshotsExceptLanguage :exec
UPDATE primary_goal_snapshots s
SET released_at = COALESCE(s.released_at, now())
FROM primary_goals g
WHERE g.owner_id = sqlc.arg('owner') AND g.book_id = sqlc.arg('book')
  AND g.language <> sqlc.arg('language')
  AND s.owner_id = g.owner_id AND s.id = g.snapshot_id;

-- name: ReleasePrimaryGoalSnapshotsForAllLanguages :exec
UPDATE primary_goal_snapshots s
SET released_at = COALESCE(s.released_at, now())
FROM primary_goals g
WHERE g.owner_id = sqlc.arg('owner') AND g.book_id = sqlc.arg('book')
  AND s.owner_id = g.owner_id AND s.id = g.snapshot_id;

-- name: InsertPrimaryGoal :one
INSERT INTO primary_goals(owner_id, language, book_id, snapshot_id)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('book'), sqlc.arg('snapshot'))
ON CONFLICT (owner_id, language) DO NOTHING
RETURNING owner_id::text, language, book_id::text, created_at, updated_at, snapshot_id::text;

-- name: ChangePrimaryGoalBook :one
UPDATE primary_goals
SET book_id = sqlc.arg('book'), snapshot_id = sqlc.arg('snapshot'), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
RETURNING owner_id::text, language, book_id::text, created_at, updated_at, snapshot_id::text;

-- name: DeletePrimaryGoal :exec
DELETE FROM primary_goals
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language');

-- name: GetReadingCompletion :one
SELECT owner_id::text, language, book_id::text, completed_at
FROM reading_history
WHERE owner_id = sqlc.arg('owner')
  AND language = sqlc.arg('language')
  AND book_id = sqlc.arg('book');

-- name: InsertReadingCompletion :one
INSERT INTO reading_history(owner_id, language, book_id, completed_at)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('book'), sqlc.arg('completed_at'))
ON CONFLICT (owner_id, language, book_id) DO NOTHING
RETURNING owner_id::text, language, book_id::text, completed_at;

-- name: ReadingCompletionExists :one
SELECT EXISTS(
  SELECT 1 FROM reading_history
  WHERE owner_id = sqlc.arg('owner')
    AND language = sqlc.arg('language')
    AND book_id = sqlc.arg('book')
);

-- name: PrimaryGoalCandidateEligible :one
SELECT EXISTS(
  SELECT 1
  FROM reading_journey_membership jm
  JOIN books b
    ON b.owner_id = jm.owner_id AND b.id = jm.book_id
  JOIN source_materials s
    ON s.owner_id = jm.owner_id AND s.book_id = jm.book_id
  JOIN current_analysis_identity ca
    ON ca.owner_id = jm.owner_id
   AND ca.book_id = jm.book_id
   AND ca.source_material_id = s.id
  WHERE jm.owner_id = sqlc.arg('owner')
    AND jm.language = sqlc.arg('language')
    AND jm.book_id = sqlc.arg('book')
    AND b.language_state = 'chosen'
    AND b.language_tag = sqlc.arg('language')
    AND lower(s.media_type) = 'application/epub+zip'
    AND NOT EXISTS (
      SELECT 1 FROM reading_history h
      WHERE h.owner_id = jm.owner_id
        AND h.language = jm.language
        AND h.book_id = jm.book_id
    )
);
