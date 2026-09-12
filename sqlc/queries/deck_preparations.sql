-- Deck-preparation lifecycle and book-anchored vocabulary-study queries.
-- Domain guards stay in the repository; these statements own the SQL shape,
-- row mapping, and RETURNING clauses.

-- name: GetDeckPreparation :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: GetDeckPreparationForUpdate :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: GetDeckPreparationForAnalysis :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND analysis_run_id = sqlc.arg('analysis_run')
  AND (book_id IS NULL OR retired_at IS NULL);

-- name: GetActiveDeckVocabularyStudy :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND studying_at IS NOT NULL
  AND graduated_at IS NULL
ORDER BY studying_at DESC
LIMIT 1;

-- name: ListDeckPreparationsForSourceMaterial :many
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND source_material_id = sqlc.arg('source_material')
ORDER BY COALESCE(completed_at, created_at) DESC, id;

-- name: DownloadDeckPreparation :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'ready';

-- name: GetDeckPreparationBySourceAnalysis :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND analysis_run_id = sqlc.arg('analysis_run')
  AND retired_at IS NULL;

-- name: GetDeckPreparationBySourceHash :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND content_hash = sqlc.arg('content_hash')
  AND retired_at IS NULL;

-- name: GetDeckPreparationBySourceAnalysisUnretired :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND analysis_run_id = sqlc.arg('analysis_run');

-- name: GetDeckPreparationBySourceHashLegacy :one
SELECT id, owner_id, source_material_id, state, artifact, filename, deck_name,
       content_hash, total_cards, cards_with_english,
       cards_with_contextual_sentence_translations, quality_omissions, error,
       created_at, updated_at, started_at, completed_at, analysis_run_id,
       current_run_id, studying_at, reviewed_at, graduated_at, released_at,
       book_id, retired_at
FROM deck_preparations
WHERE owner_id = sqlc.arg('owner')
  AND source_material_id = sqlc.arg('source_material')
  AND content_hash = sqlc.arg('content_hash')
  AND analysis_run_id IS NULL;

-- name: CreateDeckPreparation :one
INSERT INTO deck_preparations(owner_id, source_material_id, book_id, analysis_run_id, filename, deck_name, content_hash)
VALUES (sqlc.arg('owner'), sqlc.arg('source_material'), sqlc.arg('book_id'), sqlc.arg('analysis_run'), sqlc.arg('filename'), sqlc.arg('deck_name'), sqlc.arg('content_hash'))
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
          book_id, retired_at;

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
          book_id, retired_at;

-- name: CancelDeckPreparation :one
UPDATE deck_preparations
SET state = 'cancelled', error = '', completed_at = now(), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
          book_id, retired_at;

-- name: CompleteDeckPreparation :one
UPDATE deck_preparations
SET state = 'ready', artifact = sqlc.arg('artifact'), filename = sqlc.arg('filename'),
    deck_name = sqlc.arg('deck_name'), total_cards = sqlc.arg('total_cards'),
    cards_with_english = sqlc.arg('cards_with_english'),
    cards_with_contextual_sentence_translations = sqlc.arg('cards_with_contextual_sentence_translations'),
    quality_omissions = sqlc.arg('quality_omissions'), error = '',
    completed_at = now(), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'preparing'
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
          book_id, retired_at;

-- name: CompletePreparedDeckArtifact :one
UPDATE deck_preparations
SET state = 'ready', artifact = sqlc.arg('artifact'), filename = sqlc.arg('filename'),
    deck_name = sqlc.arg('deck_name'), total_cards = sqlc.arg('total_cards'),
    cards_with_english = sqlc.arg('cards_with_english'),
    cards_with_contextual_sentence_translations = sqlc.arg('cards_with_contextual_sentence_translations'),
    quality_omissions = sqlc.arg('quality_omissions'), error = '',
    completed_at = now(), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'preparing'
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
          book_id, retired_at;

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
          book_id, retired_at;

-- name: DeckPreparationExists :one
SELECT EXISTS(SELECT 1 FROM deck_preparations WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id'));

-- name: ListDeckPreparationVocabulary :many
SELECT owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at, graduated_at
FROM deck_preparation_vocabulary
WHERE owner_id = sqlc.arg('owner') AND deck_preparation_id = sqlc.arg('preparation')
ORDER BY language, canonical_lemma, upos;

-- name: ListReservedDeckVocabulary :many
SELECT dv.owner_id, dv.deck_preparation_id, dv.language, dv.canonical_lemma, dv.upos, dv.generated_at, dv.graduated_at
FROM deck_preparation_vocabulary dv
JOIN deck_preparations p ON p.owner_id = dv.owner_id AND p.id = dv.deck_preparation_id
WHERE dv.owner_id = sqlc.arg('owner') AND dv.language = sqlc.arg('language')
  AND p.studying_at IS NOT NULL AND p.graduated_at IS NULL AND dv.graduated_at IS NULL
ORDER BY dv.canonical_lemma, dv.upos;

-- name: CountDeckPreparationVocabularyToGraduate :one
SELECT count(*)
FROM deck_preparation_vocabulary dv
WHERE dv.owner_id = sqlc.arg('owner') AND dv.deck_preparation_id = sqlc.arg('preparation')
  AND dv.graduated_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM known_vocabulary kv
    WHERE kv.owner_id = dv.owner_id AND kv.language = dv.language
      AND kv.canonical_lemma = dv.canonical_lemma
      AND (kv.upos = dv.upos OR kv.upos = '')
  );

-- name: CountDeckPreparationVocabulary :one
SELECT count(*)
FROM deck_preparation_vocabulary
WHERE owner_id = sqlc.arg('owner') AND deck_preparation_id = sqlc.arg('preparation');

-- name: RepairDeckPreparationVocabulary :exec
INSERT INTO deck_preparation_vocabulary(owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at)
SELECT gv.owner_id, sqlc.arg('preparation'), gv.language, gv.canonical_lemma, gv.upos, gv.first_generated_at
FROM generated_vocabulary gv
WHERE gv.owner_id = sqlc.arg('owner') AND gv.first_source_material_id = sqlc.arg('source_material')
ON CONFLICT DO NOTHING;

-- name: StartDeckVocabularyStudy :one
UPDATE deck_preparations
SET studying_at = now(), released_at = NULL, updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('preparation') AND studying_at IS NULL
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
          book_id, retired_at;

-- name: GraduateDeckPreparationVocabularyStates :exec
INSERT INTO vocabulary_states(owner_id, language, canonical_lemma, upos, state)
SELECT dv.owner_id, dv.language, dv.canonical_lemma, dv.upos, 'known'
FROM deck_preparation_vocabulary dv
WHERE dv.owner_id = sqlc.arg('owner') AND dv.deck_preparation_id = sqlc.arg('preparation')
  AND NOT EXISTS (
    SELECT 1 FROM known_vocabulary kv
    WHERE kv.owner_id = dv.owner_id AND kv.language = dv.language
      AND kv.canonical_lemma = dv.canonical_lemma
      AND (kv.upos = dv.upos OR kv.upos = '')
  )
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO UPDATE
SET state = 'known', updated_at = now();

-- name: RecordGraduatedDeckVocabulary :exec
INSERT INTO known_vocabulary(owner_id, language, canonical_lemma, upos)
SELECT dv.owner_id, dv.language, dv.canonical_lemma, dv.upos
FROM deck_preparation_vocabulary dv
WHERE dv.owner_id = sqlc.arg('owner') AND dv.deck_preparation_id = sqlc.arg('preparation')
  AND NOT EXISTS (
    SELECT 1 FROM known_vocabulary kv
    WHERE kv.owner_id = dv.owner_id AND kv.language = dv.language
      AND kv.canonical_lemma = dv.canonical_lemma
      AND (kv.upos = dv.upos OR kv.upos = '')
  )
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO NOTHING;

-- name: MarkDeckPreparationVocabularyGraduated :exec
UPDATE deck_preparation_vocabulary
SET graduated_at = COALESCE(graduated_at, now())
WHERE owner_id = sqlc.arg('owner') AND deck_preparation_id = sqlc.arg('preparation');

-- name: ConfirmDeckVocabularyReview :one
UPDATE deck_preparations
SET studying_at = NULL, reviewed_at = COALESCE(reviewed_at, now()),
    graduated_at = COALESCE(graduated_at, now()), released_at = NULL, updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('preparation')
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
          book_id, retired_at;

-- name: ReleaseDeckVocabularyStudy :one
UPDATE deck_preparations
SET studying_at = NULL, released_at = now(), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('preparation') AND studying_at IS NOT NULL
RETURNING id, owner_id, source_material_id, state, artifact, filename, deck_name,
          content_hash, total_cards, cards_with_english,
          cards_with_contextual_sentence_translations, quality_omissions, error,
          created_at, updated_at, started_at, completed_at, analysis_run_id,
          current_run_id, studying_at, reviewed_at, graduated_at, released_at,
          book_id, retired_at;

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
