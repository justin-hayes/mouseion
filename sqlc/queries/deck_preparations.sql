-- Deck-preparation lifecycle and Goal-owned vocabulary queries.
-- Domain guards stay in the repository; these statements own the SQL shape,
-- row mapping, and RETURNING clauses.

-- name: GetDeckPreparation :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
         book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: GetDeckPreparationForUpdate :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
         book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: GetDeckPreparationForAnalysis :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
         book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND analysis_run_id = sqlc.arg('analysis_run')
  AND (book_id IS NULL OR retired_at IS NULL);

-- name: ListDeckPreparationsForSourceMaterial :many
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
         book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND source_material_id = sqlc.arg('source_material')
ORDER BY COALESCE(completed_at, created_at) DESC, id;

-- name: DownloadDeckPreparation :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
         book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'ready';

-- name: GetUnretiredDeckPreparationBySourceAnalysis :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
        book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND analysis_run_id = sqlc.arg('analysis_run')
  AND retired_at IS NULL;

-- name: GetUnretiredDeckPreparationBySourceHash :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
        book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND content_hash = sqlc.arg('content_hash')
  AND retired_at IS NULL;

-- name: GetDeckPreparationBySourceAnalysis :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
        book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND analysis_run_id = sqlc.arg('analysis_run');

-- name: GetDeckPreparationBySourceHashWithoutAnalysis :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
        book_id, retired_at, cards_with_fallback_gloss, render_input_version,
          presentation_version, deck_revision, goal_snapshot_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND content_hash = sqlc.arg('content_hash')
  AND analysis_run_id IS NULL;

-- name: CreateDeckPreparation :one
INSERT INTO deck_preparations(owner_id, source_material_id, book_id, analysis_run_id, goal_snapshot_id, filename, deck_name, content_hash)
VALUES (sqlc.arg('owner'), sqlc.arg('source_material'), sqlc.arg('book_id'), sqlc.arg('analysis_run'), sqlc.arg('goal_snapshot'), sqlc.arg('filename'), sqlc.arg('deck_name'), sqlc.arg('content_hash'))
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
        book_id, retired_at, cards_with_fallback_gloss, render_input_version,
        presentation_version, deck_revision, goal_snapshot_id;

-- name: RetireDeckPreparationsForBook :exec
UPDATE deck_preparations
SET retired_at = now(), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book') AND retired_at IS NULL;

-- name: ClaimDeckPreparation :one
UPDATE deck_preparations
SET state = 'preparing', started_at = now(), updated_at = now(), error = ''
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'queued'
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
           book_id, retired_at, cards_with_fallback_gloss, render_input_version,
           presentation_version, deck_revision, goal_snapshot_id;

-- name: CancelDeckPreparation :one
UPDATE deck_preparations
SET state = 'cancelled', error = '', completed_at = now(), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
            book_id, retired_at, cards_with_fallback_gloss, render_input_version,
             presentation_version, deck_revision, goal_snapshot_id;

-- name: CompleteDeckPreparation :one
UPDATE deck_preparations
SET state = 'ready', artifact = sqlc.arg('artifact'), filename = sqlc.arg('filename'),
    deck_name = sqlc.arg('deck_name'), total_cards = sqlc.arg('total_cards'),
    cards_with_english = sqlc.arg('cards_with_english'),
    cards_with_contextual_sentence_translations = sqlc.arg('cards_with_contextual_sentence_translations'),
    cards_with_fallback_gloss = sqlc.arg('cards_with_fallback_gloss'),
     quality_omissions = sqlc.arg('quality_omissions'),
     render_input_version = sqlc.arg('render_input_version'),
     presentation_version = sqlc.arg('presentation_version'), deck_revision = 1, error = '',
    completed_at = now(), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'preparing'
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
             book_id, retired_at, cards_with_fallback_gloss, render_input_version,
            presentation_version, deck_revision, goal_snapshot_id;

-- name: TransitionDeckPreparation :one
UPDATE deck_preparations
SET state = sqlc.arg('next_state'), error = sqlc.arg('message'),
    current_run_id = CASE WHEN sqlc.arg('next_state') = 'queued' THEN NULL ELSE current_run_id END,
    started_at = CASE WHEN sqlc.arg('next_state') = 'queued' THEN NULL ELSE started_at END,
    completed_at = CASE WHEN sqlc.arg('next_state') IN ('failed', 'cancelled') THEN now() ELSE NULL END,
    updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
  AND state = ANY(sqlc.arg('from_states')::text[])
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
              book_id, retired_at, cards_with_fallback_gloss, render_input_version,
            presentation_version, deck_revision, goal_snapshot_id;

-- name: SupersedePreparedDeckArtifact :one
WITH advanced_run AS (
    UPDATE deck_preparation_runs
    SET presentation_version = sqlc.arg('presentation_version'), updated_at = now()
    WHERE owner_id = sqlc.arg('owner') AND preparation_id = sqlc.arg('preparation')
      AND deck_preparation_runs.id = sqlc.arg('run') AND state = 'completed'
      AND deck_preparation_runs.presentation_version = sqlc.arg('expected_presentation_version')
    RETURNING deck_preparation_runs.id AS run_id
)
UPDATE deck_preparations p
SET artifact = sqlc.arg('artifact'), total_cards = sqlc.arg('total_cards'),
    cards_with_english = sqlc.arg('cards_with_english'),
    cards_with_contextual_sentence_translations = sqlc.arg('cards_with_contextual_sentence_translations'),
    cards_with_fallback_gloss = sqlc.arg('cards_with_fallback_gloss'),
    quality_omissions = sqlc.arg('quality_omissions'),
    render_input_version = sqlc.arg('render_input_version'),
    presentation_version = sqlc.arg('presentation_version'),
    deck_revision = deck_revision + 1, error = '', updated_at = now()
FROM advanced_run
WHERE p.owner_id = sqlc.arg('owner') AND p.id = sqlc.arg('preparation')
  AND p.current_run_id = advanced_run.run_id AND p.state = 'ready' AND p.retired_at IS NULL
RETURNING p.id, p.owner_id, p.source_material_id, p.state, p.artifact, p.filename,
          p.deck_name, p.content_hash, p.total_cards, p.cards_with_english,
          p.cards_with_contextual_sentence_translations, p.quality_omissions, p.error,
          p.created_at, p.updated_at, p.started_at, p.completed_at, p.analysis_run_id,
          p.current_run_id, p.studying_at, p.reviewed_at, p.graduated_at, p.released_at,
          p.book_id, p.retired_at, p.cards_with_fallback_gloss, p.render_input_version,
           p.presentation_version, p.deck_revision, p.goal_snapshot_id;

-- name: ListStalePreparedDecks :many
SELECT p.owner_id::text, p.id::text AS preparation_id, p.current_run_id::text AS run_id,
       r.presentation_version
FROM deck_preparations p
JOIN deck_preparation_runs r
  ON r.owner_id = p.owner_id AND r.preparation_id = p.id AND r.id = p.current_run_id
WHERE p.state = 'ready' AND p.retired_at IS NULL AND p.current_run_id IS NOT NULL
  AND r.state = 'completed'
  AND (r.presentation_version < sqlc.arg('presentation_version')
       OR r.render_input_version < sqlc.arg('render_input_version'))
  AND p.error <> sqlc.arg('requires_repreparation_error')
ORDER BY p.updated_at, p.id
LIMIT sqlc.arg('limit');

-- name: MarkPreparedDeckRequiresRepreparation :exec
UPDATE deck_preparations
SET error = sqlc.arg('error'), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('preparation')
  AND current_run_id = sqlc.arg('run') AND state = 'ready' AND retired_at IS NULL
  AND (error = '' OR error = sqlc.arg('error'));

-- name: DeckPreparationExists :one
SELECT EXISTS(SELECT 1 FROM deck_preparations WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id'));

-- name: InsertDeckPreparationHistory :exec
INSERT INTO processing_history(owner_id, operation, status, details, completed_at)
VALUES (sqlc.arg('owner'), 'prepared_deck', sqlc.arg('status'), sqlc.arg('details'), now());

-- name: ListDeckPreparationVocabulary :many
SELECT owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at, graduated_at
FROM deck_preparation_vocabulary
WHERE owner_id = sqlc.arg('owner') AND deck_preparation_id = sqlc.arg('preparation')
ORDER BY language, canonical_lemma, upos;

-- name: ListReservedDeckVocabulary :many
SELECT ps.owner_id, ps.id AS deck_preparation_id, pv.language, pv.canonical_lemma, pv.upos, ps.created_at AS generated_at, NULL::timestamptz AS graduated_at
FROM primary_goal_snapshots ps
JOIN primary_goals pg ON pg.owner_id = ps.owner_id AND pg.snapshot_id = ps.id
JOIN primary_goal_snapshot_vocabulary pv ON pv.owner_id = ps.owner_id AND pv.snapshot_id = ps.id
WHERE ps.owner_id = sqlc.arg('owner') AND ps.language = sqlc.arg('language')
  AND ps.released_at IS NULL AND pv.language = sqlc.arg('language')
ORDER BY canonical_lemma, upos;

-- name: InsertGeneratedVocabulary :exec
INSERT INTO generated_vocabulary(owner_id, language, canonical_lemma, upos, first_deck_id, first_source_material_id)
SELECT sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('canonical_lemma'), sqlc.arg('upos'), sqlc.arg('deck'), source_material_id
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('preparation')
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO NOTHING;

-- name: PutPreparedDeckCard :exec
INSERT INTO cards(owner_id, deck_id, dedup_key, canonical_lemma, upos, front, back)
VALUES (sqlc.arg('owner'), sqlc.arg('deck'), sqlc.arg('dedup_key'), sqlc.arg('canonical_lemma'), sqlc.arg('upos'), sqlc.arg('front'), sqlc.arg('back'))
ON CONFLICT(owner_id, dedup_key) DO UPDATE
SET deck_id = excluded.deck_id, front = excluded.front, back = excluded.back;

-- name: AttachDeckPreparationVocabulary :exec
INSERT INTO deck_preparation_vocabulary(owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at)
SELECT gv.owner_id, sqlc.arg('preparation'), gv.language, gv.canonical_lemma, gv.upos, gv.first_generated_at
FROM generated_vocabulary gv
WHERE gv.owner_id = sqlc.arg('owner') AND gv.language = sqlc.arg('language')
  AND gv.canonical_lemma = sqlc.arg('canonical_lemma') AND gv.upos = sqlc.arg('upos')
ON CONFLICT DO NOTHING;

-- name: SetVocabularyStateGenerated :exec
UPDATE vocabulary_states
SET state = 'generated', updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
  AND canonical_lemma = sqlc.arg('canonical_lemma') AND upos = sqlc.arg('upos');
