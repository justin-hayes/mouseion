-- Study (ADR 0134, spec #2691): the stream of new-word candidates and the
-- Study material prepared for them.

-- name: ListStudyCandidates :many
-- Effective vocabulary identities of the owner's currently analyzed Books in
-- the study language, with their Personal analyzed-Book occurrences and whether
-- the Current reading contributes them. The counts are the Browse count
-- projection, which already applies confirmed Lemma corrections and Occurrence
-- exclusions. A Book whose projection is not ready contributes nothing yet.
-- Known and Reserved vocabulary are deliberately not consulted (ADR 0134 §3).
-- The builder version is the one Browse reads. With only_lemmas, the rows of
-- those lemmas alone are read (the identity index finds them), so a batch is
-- ordered from the few words whose material is ready without aggregating the
-- library; the caller keeps the identities it asked for, since a part of speech
-- or discriminator it did not ask for may come back with its lemma.
WITH eligible AS MATERIALIZED (
  -- The few Books that contribute: resolved once, so the count rows are not
  -- checked against each of them one by one.
  SELECT cai.owner_id, cai.book_id, cai.analysis_run_id, cai.corpus_id,
         COALESCE(cai.book_id = g.book_id, false) AS current_reading
    FROM current_analysis_identity cai
    JOIN books b ON b.owner_id = cai.owner_id AND b.id = cai.book_id
     AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
    JOIN vocabulary_browse_count_readiness r ON r.owner_id = cai.owner_id AND r.book_id = cai.book_id
     AND r.analysis_run_id = cai.analysis_run_id AND r.corpus_id = cai.corpus_id
     AND r.language = sqlc.arg('language')::text AND r.builder_version = 4
    LEFT JOIN primary_goals g ON g.owner_id = cai.owner_id AND g.language = sqlc.arg('language')::text
   WHERE cai.owner_id = sqlc.arg('owner')
)
SELECT c.canonical_lemma::text AS canonical_lemma,
       c.upos::text AS upos,
       c.lexical_discriminator::text AS lexical_discriminator,
       sum(c.occurrence_count)::bigint AS personal_occurrences,
       COALESCE(bool_or(e.current_reading), false)::boolean AS in_current_reading
  FROM eligible e
  JOIN vocabulary_browse_counts c ON c.owner_id = e.owner_id AND c.book_id = e.book_id
   AND c.analysis_run_id = e.analysis_run_id AND c.corpus_id = e.corpus_id
 WHERE c.owner_id = sqlc.arg('owner') AND c.language = sqlc.arg('language')::text
   AND (NOT sqlc.arg('only_lemmas')::boolean OR c.canonical_lemma = ANY(sqlc.arg('lemmas')::text[]))
 GROUP BY c.canonical_lemma, c.upos, c.lexical_discriminator;

-- name: ListCurrentStudyMaterial :many
SELECT id::text AS id, language, canonical_lemma, upos, lexical_discriminator, state, omission_reason,
       COALESCE(book_id::text, '')::text AS book_id,
       COALESCE(analysis_run_id::text, '')::text AS analysis_run_id,
       COALESCE(corpus_id::text, '')::text AS corpus_id,
       COALESCE(sentence_ordinal, 0)::bigint AS sentence_ordinal,
       COALESCE(token_ordinal, 0)::bigint AS token_ordinal,
       sentence, target_word, gloss, sentence_translation, sentence_translation_targets, evidence_fingerprint,
       cited_sense_ids, cited_entry_ids, dictionary_pin, passage_context, passage_citation, passage_translation_pin,
       created_at, updated_at
  FROM study_material
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND superseded_at IS NULL
 ORDER BY canonical_lemma, upos, lexical_discriminator;

-- name: GetStudyMaterial :one
SELECT id::text AS id, language, canonical_lemma, upos, lexical_discriminator, state, omission_reason,
       COALESCE(book_id::text, '')::text AS book_id,
       COALESCE(analysis_run_id::text, '')::text AS analysis_run_id,
       COALESCE(corpus_id::text, '')::text AS corpus_id,
       COALESCE(sentence_ordinal, 0)::bigint AS sentence_ordinal,
       COALESCE(token_ordinal, 0)::bigint AS token_ordinal,
       sentence, target_word, gloss, sentence_translation, sentence_translation_targets, evidence_fingerprint,
       cited_sense_ids, cited_entry_ids, dictionary_pin, passage_context, passage_citation, passage_translation_pin,
       created_at, updated_at
  FROM study_material
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id');

-- name: CountCurrentStudyMaterial :many
SELECT state, count(*)::bigint AS total
  FROM study_material
 WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language') AND superseded_at IS NULL
 GROUP BY state;

-- name: ReserveStudyMaterial :one
-- Starts a word's material as pending. A word that already has current
-- material returns no row.
INSERT INTO study_material (owner_id, language, canonical_lemma, upos, lexical_discriminator, state)
VALUES (sqlc.arg('owner'), sqlc.arg('language'), sqlc.arg('canonical_lemma'), sqlc.arg('upos'), sqlc.arg('lexical_discriminator'), 'pending')
ON CONFLICT (owner_id, language, canonical_lemma, upos, lexical_discriminator) WHERE superseded_at IS NULL DO NOTHING
RETURNING id::text AS id;

-- name: MarkStudyMaterialReady :execrows
UPDATE study_material
   SET state = 'ready', book_id = sqlc.arg('book'), analysis_run_id = sqlc.arg('analysis_run'),
       corpus_id = sqlc.arg('corpus'), sentence_ordinal = sqlc.arg('sentence_ordinal'),
       token_ordinal = sqlc.arg('token_ordinal'), sentence = sqlc.arg('sentence'),
       target_word = sqlc.arg('target_word'), gloss = sqlc.arg('gloss'),
       sentence_translation = sqlc.arg('sentence_translation'),
       sentence_translation_targets = sqlc.arg('sentence_translation_targets')::jsonb,
       cited_sense_ids = sqlc.arg('cited_sense_ids')::text[], cited_entry_ids = sqlc.arg('cited_entry_ids')::text[],
       dictionary_pin = sqlc.arg('dictionary_pin'), passage_context = sqlc.arg('passage_context'),
       passage_citation = sqlc.arg('passage_citation'), passage_translation_pin = sqlc.arg('passage_translation_pin'),
       updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'pending';

-- name: MarkStudyMaterialUnpreparable :execrows
UPDATE study_material
   SET state = 'unpreparable', omission_reason = sqlc.arg('reason'), updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'pending';

-- name: SetStudyMaterialEvidence :execrows
-- Records the occurrences a preparation attempt saw, before it finds the
-- material unpreparable.
UPDATE study_material
   SET evidence_fingerprint = sqlc.arg('evidence'), updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'pending';

-- name: RelocateStudyMaterial :execrows
-- An Analysis update re-published the word's introducing occurrence in a new
-- run: the material keeps its sentence, Gloss and translation and names the
-- occurrence where it now is.
UPDATE study_material
   SET analysis_run_id = sqlc.arg('analysis_run'), corpus_id = sqlc.arg('corpus'),
       sentence_ordinal = sqlc.arg('sentence_ordinal'), token_ordinal = sqlc.arg('token_ordinal'), updated_at = now()
 WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id') AND state = 'ready' AND superseded_at IS NULL;

-- name: GetStudyOccurrenceLocator :one
-- Where an analyzed token sits in its Book's extracted unit: the locator an
-- Analysis update carries decisions across runs by.
SELECT s.unit_id::text AS unit_id, t.start_offset::bigint AS start_offset, t.end_offset::bigint AS end_offset
  FROM corpus_tokens t
  JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.analysis_run_id = t.analysis_run_id
   AND s.corpus_id = t.corpus_id AND s.sentence_ordinal = t.sentence_ordinal
 WHERE t.owner_id = sqlc.arg('owner') AND t.analysis_run_id = sqlc.arg('analysis_run') AND t.corpus_id = sqlc.arg('corpus')
   AND t.sentence_ordinal = sqlc.arg('sentence_ordinal') AND t.token_ordinal = sqlc.arg('token_ordinal');

-- name: ListStudySourceTokens :many
-- The tokens of one Book's current analysis that a Study word's introducing
-- occurrence can be found as: the token itself when its run is still current,
-- and the token at the same unit and offsets when an Analysis update
-- re-published it in a new run. Each branch reads tokens by their primary key
-- through the Book's one current analysis, never the word's occurrences, so the
-- correction check costs the same for any word (#2705). Predicates on the
-- owner and language stay out of the token scans: they match an index that
-- holds every token of the owner. The eligibility is the concordance's: a
-- current analysis of a Book in the study language whose sentence's unit is in
-- the current snapshot. The effective lemma, Occurrence exclusion and part of
-- speech come back for Go to decide, as the listing's ranked stage does.
WITH current_run AS MATERIALIZED (
  SELECT ca.owner_id, ca.book_id, ca.analysis_run_id, ca.corpus_id, ca.source_material_id, ca.snapshot_id
    FROM current_analysis_identity ca
    JOIN books b ON b.owner_id = ca.owner_id AND b.id = ca.book_id
      AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
   WHERE ca.owner_id = sqlc.arg('owner')::uuid AND ca.book_id = sqlc.arg('book')::uuid
),
located AS MATERIALIZED (
  SELECT t.owner_id, t.language, t.analysis_run_id, t.corpus_id, t.sentence_ordinal, t.token_ordinal,
         t.canonical_lemma, t.upos, t.start_offset, t.end_offset, t.surface, t.raw_lemma
    FROM current_run cr
    JOIN corpus_tokens t ON t.analysis_run_id = cr.analysis_run_id
   WHERE cr.analysis_run_id = sqlc.arg('analysis_run')::uuid
     AND t.sentence_ordinal = sqlc.arg('sentence_ordinal')::bigint
     AND t.token_ordinal = sqlc.arg('token_ordinal')::bigint
  UNION ALL
  SELECT t.owner_id, t.language, t.analysis_run_id, t.corpus_id, t.sentence_ordinal, t.token_ordinal,
         t.canonical_lemma, t.upos, t.start_offset, t.end_offset, t.surface, t.raw_lemma
    FROM current_run cr
    JOIN corpus_sentences s ON s.analysis_run_id = cr.analysis_run_id
    JOIN corpus_tokens t ON t.analysis_run_id = s.analysis_run_id
      AND t.sentence_ordinal = s.sentence_ordinal
   WHERE sqlc.arg('unit_id')::text <> ''
     AND s.unit_id = sqlc.arg('unit_id')::text
     AND t.start_offset = sqlc.arg('start_offset')::bigint AND t.end_offset = sqlc.arg('end_offset')::bigint
)
SELECT l.analysis_run_id::text AS analysis_run_id,
       l.corpus_id::text AS corpus_id,
       l.sentence_ordinal,
       l.token_ordinal,
       l.canonical_lemma,
       l.upos,
       l.surface,
       l.raw_lemma,
       l.start_offset AS unit_start_offset,
       l.end_offset AS unit_end_offset,
       s.unit_id,
       s.start_offset::bigint AS sentence_unit_start_offset,
       s.end_offset::bigint AS sentence_unit_end_offset,
       cr.book_id::text AS book_id,
       cr.source_material_id::text AS source_material_id,
       cr.snapshot_id::text AS snapshot_id,
       d.canonical_lemma AS decision_lemma,
       COALESCE(d.excluded, false)::boolean AS decision_excluded
  FROM located l
  JOIN current_run cr ON cr.owner_id = l.owner_id
    AND cr.analysis_run_id = l.analysis_run_id AND cr.corpus_id = l.corpus_id
  JOIN corpus_sentences s ON s.analysis_run_id = l.analysis_run_id
    AND s.sentence_ordinal = l.sentence_ordinal
    AND s.owner_id = l.owner_id AND s.corpus_id = l.corpus_id
  JOIN source_material_units u ON u.owner_id = cr.owner_id
    AND u.source_material_id = cr.source_material_id
    AND u.snapshot_id = cr.snapshot_id AND u.unit_id = s.unit_id
  LEFT JOIN occurrence_lemma_corrections d ON d.owner_id = cr.owner_id
    AND d.book_id = cr.book_id AND d.corpus_id = l.corpus_id
    AND d.analysis_run_id = l.analysis_run_id AND d.source_document_id = s.unit_id
    AND d.start_offset = l.start_offset AND d.end_offset = l.end_offset
 WHERE l.language = sqlc.arg('language')::text;

-- name: StudyBookInLibrary :one
-- Whether the Book is still an active member of the learner's library.
SELECT EXISTS (SELECT 1 FROM book_membership
                WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book')::uuid AND state = 'active')::boolean;
