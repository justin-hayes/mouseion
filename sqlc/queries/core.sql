-- Core persistence queries: users, sessions, supported languages, analysis
-- jobs, catalogue sync, source materials, artifacts, corpora, vocabulary
-- states, known vocabulary, generated vocabulary, example/curated sentences,
-- decks, cards, and processing history.

-- name: HasUsers :one
SELECT EXISTS(SELECT 1 FROM users);

-- name: CreateUser :one
INSERT INTO users(username, is_admin) VALUES ($1, $2)
RETURNING id::text, username, created_at;

-- name: CreateUserWithPassword :one
INSERT INTO users(username, password_hash, is_admin) VALUES ($1, $2, $3)
RETURNING id::text, username, created_at;

-- name: CreateFirstUserAndSession :one
INSERT INTO users(username, password_hash, is_admin) VALUES ($1, $2, false)
RETURNING id::text, username, created_at;

-- name: InsertSession :exec
INSERT INTO sessions(user_id, token_hash, expires_at) VALUES ($1, $2, $3);

-- name: GetUserByUsername :one
SELECT id::text, username, created_at, COALESCE(password_hash, '') AS password_hash
FROM users WHERE username = $1;

-- name: GetUserByID :one
SELECT id::text, username, created_at FROM users WHERE id = $1;

-- name: GetStoredActiveStudyLanguage :one
SELECT active_study_language FROM users WHERE id = $1;

-- name: SetActiveStudyLanguage :execrows
UPDATE users SET active_study_language = $2 WHERE id = $1;

-- name: SetUserPassword :execrows
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: MostRecentlyActivatedStudyLanguage :one
SELECT b.language_tag
FROM books b
JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id AND m.state = 'active'
WHERE b.owner_id = $1 AND b.language_state = 'chosen' AND b.language_tag <> ''
ORDER BY m.activated_at DESC NULLS LAST, b.updated_at DESC, b.id DESC
LIMIT 1;

-- name: GetSession :one
SELECT u.id::text, u.username, u.created_at, s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: PutSupportedLanguage :one
INSERT INTO supported_languages(language, display_name) VALUES ($1, $2)
ON CONFLICT(language) DO UPDATE SET display_name = excluded.display_name
RETURNING language, display_name, created_at;

-- name: PutSupportedLanguageOrIgnore :exec
INSERT INTO supported_languages(language, display_name) VALUES ($1, $1)
ON CONFLICT(language) DO NOTHING;

-- name: ListSupportedLanguages :many
SELECT language, display_name, created_at FROM supported_languages ORDER BY display_name, language;

-- name: ListAnalysisJobs :many
SELECT j.river_job_id,
       j.display_number,
       j.owner_id::text,
       j.source_material_id::text,
       j.content_hash,
       (COALESCE(j.corpus_id::text, ''))::text AS corpus_id,
       (COALESCE(j.analysis_run_id::text, ''))::text AS analysis_run_id,
       (COALESCE(r.state, ''))::text AS analysis_state,
       j.progress,
       j.error,
       j.created_at,
       j.updated_at
FROM analysis_jobs j
LEFT JOIN analysis_runs r ON r.owner_id = j.owner_id AND r.id = j.analysis_run_id
WHERE j.owner_id = $1
ORDER BY j.created_at DESC, j.river_job_id DESC
LIMIT 100;

-- name: SetCatalogueSyncStatus :exec
INSERT INTO catalogue_sync_status(owner_id, connection_id, state, last_synced_at, last_upserted_count, last_error)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(owner_id, connection_id) DO UPDATE SET
  state = excluded.state,
  last_synced_at = COALESCE(excluded.last_synced_at, catalogue_sync_status.last_synced_at),
  last_upserted_count = excluded.last_upserted_count,
  last_error = excluded.last_error,
  updated_at = now();

-- name: GetCatalogueSyncStatus :one
SELECT owner_id::text, connection_id::text, state, last_synced_at, last_upserted_count, last_error, updated_at
FROM catalogue_sync_status WHERE owner_id = $1 AND connection_id = $2;

-- name: ListCatalogueSyncStatuses :many
SELECT owner_id::text, connection_id::text, state, last_synced_at, last_upserted_count, last_error, updated_at
FROM catalogue_sync_status WHERE owner_id = $1 ORDER BY connection_id;

-- name: PutSourceMaterial :one
INSERT INTO source_materials(owner_id, language, source_identifier, title, media_type, content_hash, content, full_text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT(owner_id, source_identifier) DO UPDATE SET
  language = excluded.language, title = excluded.title, media_type = excluded.media_type
RETURNING id::text;

-- name: GetSourceMaterial :one
SELECT s.id::text,
       s.owner_id::text,
       s.language,
       s.source_identifier,
       s.title,
       s.media_type,
       (CASE WHEN r.digest_version = 1 THEN r.content_digest ELSE s.content_hash END)::text AS content_hash,
       (COALESCE(r.content_digest, ''))::text AS content_digest,
       COALESCE(r.content, s.content) AS content,
       COALESCE(r.full_text, s.full_text) AS full_text,
       (COALESCE(r.revision_id::text, ''))::text AS content_revision_id,
       COALESCE(r.digest_version, 0) AS digest_version,
       s.created_at
FROM source_materials s
LEFT JOIN source_content_revisions r ON r.owner_id = s.owner_id AND r.revision_id = s.current_content_revision_id
WHERE s.owner_id = $1 AND s.id = $2;

-- name: FindSourceMaterialForAcquisition :one
SELECT s.id::text,
       s.owner_id::text,
       s.language,
       s.source_identifier,
       s.title,
       s.media_type,
       (CASE WHEN r.digest_version = 1 THEN r.content_digest ELSE s.content_hash END)::text AS content_hash,
       (COALESCE(r.content_digest, ''))::text AS content_digest,
       COALESCE(r.content, s.content) AS content,
       COALESCE(r.full_text, s.full_text) AS full_text,
       (COALESCE(r.revision_id::text, ''))::text AS content_revision_id,
       COALESCE(r.digest_version, 0) AS digest_version,
       s.created_at
FROM source_materials s
LEFT JOIN source_content_revisions r ON r.owner_id = s.owner_id AND r.revision_id = s.current_content_revision_id
WHERE s.owner_id = $1 AND ((s.source_identifier = sqlc.arg('source_identifier') AND CASE WHEN r.digest_version = 1 THEN r.content_digest ELSE s.content_hash END = sqlc.arg('content_hash')::text) OR r.content_digest = sqlc.arg('content_hash')::text OR s.content_hash = sqlc.arg('content_hash')::text)
ORDER BY (s.source_identifier = sqlc.arg('source_identifier')) DESC, s.created_at, s.id
LIMIT 1;

-- name: PutNormalizedCorpusArtifact :exec
INSERT INTO normalized_corpus_artifacts(content_hash, language, schema_version, normalization_profile, normalization_version, analyzer_name, analyzer_version)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT(content_hash) DO NOTHING;

-- name: PutSharedLemma :exec
INSERT INTO shared_lemmas(content_hash, language, canonical_lemma, upos, morphology, frequency)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(content_hash, canonical_lemma, upos, morphology) DO UPDATE SET frequency = excluded.frequency;

-- name: GetNormalizedCorpusArtifact :one
SELECT content_hash, language, schema_version, normalization_profile, normalization_version, analyzer_name, analyzer_version, created_at
FROM normalized_corpus_artifacts WHERE content_hash = $1;

-- name: ListSharedLemmas :many
SELECT content_hash, language, canonical_lemma, upos, morphology, frequency
FROM shared_lemmas WHERE content_hash = $1 ORDER BY id;

-- name: GetCorpus :one
SELECT id::text,
       owner_id::text,
       source_material_id::text,
       artifact_hash,
       (COALESCE(analysis_run_id::text, ''))::text AS analysis_run_id,
       status,
       analyzable_token_count,
       distinct_lemma_count,
       sentence_count,
       normalized_token_count,
       empty_sentence_count,
       median_sentence_token_count,
       p90_sentence_token_count,
       long_sentence_count,
       created_at
FROM corpora WHERE owner_id = $1 AND id = $2;

-- name: LatestCorpusForSource :one
SELECT id::text, owner_id::text, source_material_id::text, artifact_hash, status, created_at
FROM corpora WHERE owner_id = $1 AND source_material_id = $2 ORDER BY created_at DESC LIMIT 1;

-- name: PutCorpus :one
INSERT INTO corpora(owner_id, source_material_id, artifact_hash) VALUES ($1, $2, $3)
RETURNING id::text, owner_id::text, source_material_id::text, artifact_hash, status, created_at;

-- name: UpdateCorpusArtifactHash :exec
UPDATE corpora SET artifact_hash = $3 WHERE owner_id = $1 AND id = $2;

-- name: GetKnownVocabularyByIdentity :one
SELECT id::text, owner_id::text, language, canonical_lemma, upos, created_at
FROM known_vocabulary WHERE owner_id = $1 AND language = $2 AND canonical_lemma = $3 AND upos = $4 ORDER BY id LIMIT 1;

-- name: UpsertKnownVocabulary :one
INSERT INTO known_vocabulary(owner_id, language, canonical_lemma, upos)
VALUES ($1, $2, $3, $4)
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO UPDATE SET canonical_lemma = excluded.canonical_lemma
RETURNING id::text, owner_id::text, language, canonical_lemma, upos, created_at;

-- name: GetKnownVocabulary :one
SELECT kv.id::text,
       kv.owner_id::text,
       kv.language,
       kv.canonical_lemma,
       kv.upos,
       (CASE WHEN EXISTS (
         SELECT 1 FROM deck_preparation_vocabulary dv
         JOIN deck_preparations p ON p.owner_id = dv.owner_id AND p.id = dv.deck_preparation_id
         WHERE dv.owner_id = kv.owner_id AND dv.language = kv.language
           AND dv.canonical_lemma = kv.canonical_lemma AND dv.upos = kv.upos
           AND dv.graduated_at IS NOT NULL AND p.reviewed_at IS NOT NULL
       ) THEN 'Graduated from reviewed deck' ELSE 'Explicitly recorded' END)::text AS provenance,
       kv.created_at
FROM known_vocabulary kv WHERE kv.owner_id = $1 AND kv.id = $2;

-- name: ListKnownVocabulary :many
SELECT kv.id::text,
       kv.owner_id::text,
       kv.language,
       kv.canonical_lemma,
       kv.upos,
       (CASE WHEN EXISTS (
         SELECT 1 FROM deck_preparation_vocabulary dv
         JOIN deck_preparations p ON p.owner_id = dv.owner_id AND p.id = dv.deck_preparation_id
         WHERE dv.owner_id = kv.owner_id AND dv.language = kv.language
           AND dv.canonical_lemma = kv.canonical_lemma AND dv.upos = kv.upos
           AND dv.graduated_at IS NOT NULL AND p.reviewed_at IS NOT NULL
       ) THEN 'Graduated from reviewed deck' ELSE 'Explicitly recorded' END)::text AS provenance,
       kv.created_at
FROM known_vocabulary kv WHERE kv.owner_id = $1 AND kv.language = $2 ORDER BY kv.canonical_lemma, kv.upos, kv.id;

-- name: ListKnownVocabularyLanguages :many
WITH known_languages AS (
  SELECT DISTINCT language FROM known_vocabulary WHERE owner_id = $1 AND language <> ''
)
SELECT k.language, (COALESCE(NULLIF(n.display_name, ''), k.language))::text AS display_name
FROM known_languages k
LEFT JOIN supported_languages n ON n.language = k.language
ORDER BY COALESCE(NULLIF(n.display_name, ''), k.language), k.language;

-- name: IsKnownVocabularyIdentity :one
SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id = $1 AND language = $2 AND canonical_lemma = $3 AND (upos = $4 OR upos = ''));

-- name: KnownVocabularyExists :one
SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id = $1 AND language = $2 AND canonical_lemma = $3 AND upos = $4);

-- name: PutVocabularyState :one
INSERT INTO vocabulary_states(owner_id, language, canonical_lemma, upos, state)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO UPDATE SET state = excluded.state, updated_at = now()
RETURNING id::text, owner_id::text, language, canonical_lemma, upos, state, updated_at;

-- name: GetVocabularyState :one
SELECT id::text, owner_id::text, language, canonical_lemma, upos, state, updated_at
FROM vocabulary_states WHERE owner_id = $1 AND id = $2;

-- name: GetVocabularyStateByIdentity :one
SELECT id::text, owner_id::text, language, canonical_lemma, upos, state, updated_at
FROM vocabulary_states WHERE owner_id = $1 AND language = $2 AND canonical_lemma = $3 AND upos = $4;

-- name: GetVocabularyStateForUpdate :one
SELECT state FROM vocabulary_states WHERE owner_id = $1 AND language = $2 AND canonical_lemma = $3 AND upos = $4 FOR UPDATE;

-- name: InsertVocabularyStateCandidate :exec
INSERT INTO vocabulary_states(owner_id, language, canonical_lemma, upos, state)
VALUES ($1, $2, $3, $4, 'candidate') ON CONFLICT DO NOTHING;

-- name: DeleteVocabularyState :execrows
DELETE FROM vocabulary_states WHERE owner_id = $1 AND id = $2;

-- name: ReservedVocabularyExists :one
SELECT EXISTS(
  SELECT 1
  FROM primary_goal_snapshots ps
  JOIN primary_goals pg ON pg.owner_id = ps.owner_id AND pg.snapshot_id = ps.id
  JOIN primary_goal_snapshot_vocabulary pv
    ON pv.owner_id = ps.owner_id AND pv.snapshot_id = ps.id
  WHERE ps.owner_id = $1 AND ps.language = $2
    AND ps.released_at IS NULL AND pv.language = $2
    AND pv.canonical_lemma = $3 AND pv.upos = $4
  UNION ALL
  SELECT 1 FROM deck_preparation_vocabulary dv
  JOIN deck_preparations p ON p.owner_id = dv.owner_id AND p.id = dv.deck_preparation_id
  WHERE dv.owner_id = $1 AND dv.language = $2 AND dv.canonical_lemma = $3 AND dv.upos = $4
    AND p.studying_at IS NOT NULL AND p.graduated_at IS NULL
);

-- name: ListUnattachedGeneratedVocabulary :many
SELECT owner_id::text, language, canonical_lemma, upos, (COALESCE(first_deck_id::text, ''))::text AS first_deck_id, first_source_material_id, first_generated_at
FROM generated_vocabulary gv
WHERE gv.owner_id = $1 AND gv.language = $2 AND NOT EXISTS (
  SELECT 1 FROM deck_preparation_vocabulary dv
  WHERE dv.owner_id = gv.owner_id AND dv.language = gv.language
    AND dv.canonical_lemma = gv.canonical_lemma AND dv.upos = gv.upos)
ORDER BY gv.canonical_lemma, gv.upos;

-- name: PutSelectionCandidate :exec
INSERT INTO selection_candidates(owner_id, corpus_id, language, canonical_lemma, upos, occurrence_count, observed_forms, eligible_sentence_refs, provenance)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT(owner_id, corpus_id, language, canonical_lemma, upos) DO UPDATE SET
  occurrence_count = excluded.occurrence_count,
  observed_forms = excluded.observed_forms,
  eligible_sentence_refs = excluded.eligible_sentence_refs,
  provenance = excluded.provenance,
  selected_at = now();

-- name: PutExampleSentence :one
INSERT INTO example_sentences(owner_id, corpus_id, sentence_key, sentence_text, source_location)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT(owner_id, corpus_id, sentence_key) DO UPDATE SET
  sentence_text = excluded.sentence_text, source_location = excluded.source_location
RETURNING id, owner_id, corpus_id, sentence_key, sentence_text, source_location, created_at;

-- name: CorpusOwned :one
SELECT EXISTS(SELECT 1 FROM corpora WHERE owner_id = $1 AND id = $2);

-- name: DeleteSelectedSentences :exec
DELETE FROM example_sentences WHERE owner_id = $1 AND corpus_id = $2 AND language = $3 AND canonical_lemma = $4 AND upos = $5;

-- name: InsertSelectedSentence :exec
INSERT INTO example_sentences(owner_id, corpus_id, sentence_key, sentence_text, source_location, language, canonical_lemma, upos, selection_rank, selection_score, selection_reasons, is_chosen)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: PutCuratedSentence :one
INSERT INTO curated_sentences(owner_id, example_sentence_id, language, canonical_lemma, upos, notes)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO UPDATE SET
  example_sentence_id = excluded.example_sentence_id, notes = excluded.notes
RETURNING id::text, owner_id::text, example_sentence_id::text, language, canonical_lemma, upos, notes, created_at;

-- name: CuratedSentenceExists :one
SELECT EXISTS(SELECT 1 FROM example_sentences WHERE id = $1 AND owner_id = $2 AND language = $3 AND canonical_lemma = $4 AND upos = $5);

-- name: UpdateExampleSentenceText :exec
UPDATE example_sentences SET sentence_text = $1 WHERE id = $2 AND owner_id = $3;

-- name: UpsertCuratedSentence :one
INSERT INTO curated_sentences(owner_id, example_sentence_id, language, canonical_lemma, upos, notes)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO UPDATE SET
  example_sentence_id = excluded.example_sentence_id, notes = excluded.notes, created_at = now()
RETURNING id::text, owner_id::text, example_sentence_id::text, language, canonical_lemma, upos, notes, created_at;

-- name: InsertProcessingHistory :exec
INSERT INTO processing_history(owner_id, corpus_id, operation, status, details, completed_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertProcessingHistoryWithoutCorpus :exec
INSERT INTO processing_history(owner_id, operation, status, details, completed_at)
VALUES ($1, $2, $3, $4, $5);

-- name: PutProcessingHistory :one
INSERT INTO processing_history(owner_id, corpus_id, operation, status, details, completed_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id::text, owner_id::text, (COALESCE(corpus_id::text, ''))::text AS corpus_id, operation, status, details, started_at, completed_at;

-- name: PutDeck :one
INSERT INTO decks(owner_id, language, name) VALUES ($1, $2, $3)
ON CONFLICT(owner_id, language, name) DO UPDATE SET name = excluded.name
RETURNING id::text, owner_id::text, language, name, created_at;

-- name: PutCard :one
INSERT INTO cards(owner_id, deck_id, dedup_key, canonical_lemma, upos, front, back)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT(owner_id, dedup_key) DO UPDATE SET front = excluded.front, back = excluded.back
RETURNING id::text, owner_id::text, deck_id::text, dedup_key, canonical_lemma, upos, front, back, created_at;
