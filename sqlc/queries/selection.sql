-- Sentence selection and review queries.

-- name: ListLemmaReviewOccurrences :many
SELECT c.id::text AS corpus_id, c.analysis_run_id::text AS analysis_run_id,
       t.language,
       t.sentence_ordinal, t.token_ordinal, t.surface, t.raw_lemma,
       t.canonical_lemma, t.upos, t.dependency, t.start_offset, t.end_offset,
       t.lexical_discriminator,
       s.sentence_text, s.unit_id,
       COALESCE(d.canonical_lemma, '')::text AS corrected_lemma,
       COALESCE(d.excluded, false)::boolean AS excluded,
       COALESCE(a.normalization_profile, '')::text AS normalization_profile,
       COALESCE(a.normalization_version, '')::text AS normalization_version
FROM book_current_analyses cai
JOIN source_materials source ON source.owner_id = cai.owner_id
  AND source.id = cai.source_material_id AND source.book_id = cai.book_id
JOIN analysis_runs r ON r.owner_id = cai.owner_id AND r.id = cai.analysis_run_id
  AND r.source_material_id = cai.source_material_id AND r.state = 'completed'
JOIN corpora c ON c.owner_id = r.owner_id AND c.id = r.corpus_id
  AND c.source_material_id = r.source_material_id AND c.analysis_run_id = r.id
  AND c.status = 'complete'
LEFT JOIN normalized_corpus_artifacts a ON a.content_hash = c.artifact_hash
JOIN corpus_tokens t ON t.owner_id = c.owner_id AND t.corpus_id = c.id
  AND t.analysis_run_id = c.analysis_run_id
JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.corpus_id = t.corpus_id
  AND s.analysis_run_id = t.analysis_run_id AND s.sentence_ordinal = t.sentence_ordinal
LEFT JOIN occurrence_lemma_corrections d ON d.owner_id = cai.owner_id AND d.book_id = cai.book_id
  AND d.corpus_id = c.id AND d.analysis_run_id = c.analysis_run_id
   AND d.source_document_id = s.unit_id AND d.start_offset = t.start_offset AND d.end_offset = t.end_offset
WHERE cai.owner_id = sqlc.arg('owner') AND cai.book_id = sqlc.arg('book')
  AND source.current_content_revision_id = r.content_revision_id
  AND source.current_snapshot_id = r.snapshot_id
   AND (sqlc.arg('surface')::text = '' OR t.surface = sqlc.arg('surface'))
ORDER BY s.sentence_ordinal, t.token_ordinal;

-- name: ListCorpusLemmaReviewOccurrences :many
-- The same occurrences across every Book in the learner's My Books of one
-- language, for one observed form and its analyzer lemma (ADR 0136 §6).
SELECT cai.book_id::text AS book_id, c.id::text AS corpus_id, c.analysis_run_id::text AS analysis_run_id,
       t.language,
       t.sentence_ordinal, t.token_ordinal, t.surface, t.raw_lemma,
       t.canonical_lemma, t.upos, t.dependency, t.start_offset, t.end_offset,
       t.lexical_discriminator,
       s.sentence_text, s.unit_id,
       COALESCE(d.canonical_lemma, '')::text AS corrected_lemma,
       COALESCE(d.excluded, false)::boolean AS excluded,
       COALESCE(a.normalization_profile, '')::text AS normalization_profile,
       COALESCE(a.normalization_version, '')::text AS normalization_version
FROM book_current_analyses cai
JOIN source_materials source ON source.owner_id = cai.owner_id
  AND source.id = cai.source_material_id AND source.book_id = cai.book_id
JOIN analysis_runs r ON r.owner_id = cai.owner_id AND r.id = cai.analysis_run_id
  AND r.source_material_id = cai.source_material_id AND r.state = 'completed'
JOIN corpora c ON c.owner_id = r.owner_id AND c.id = r.corpus_id
  AND c.source_material_id = r.source_material_id AND c.analysis_run_id = r.id
  AND c.status = 'complete'
LEFT JOIN normalized_corpus_artifacts a ON a.content_hash = c.artifact_hash
JOIN corpus_tokens t ON t.owner_id = c.owner_id AND t.corpus_id = c.id
  AND t.analysis_run_id = c.analysis_run_id
JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.corpus_id = t.corpus_id
  AND s.analysis_run_id = t.analysis_run_id AND s.sentence_ordinal = t.sentence_ordinal
LEFT JOIN occurrence_lemma_corrections d ON d.owner_id = cai.owner_id AND d.book_id = cai.book_id
  AND d.corpus_id = c.id AND d.analysis_run_id = c.analysis_run_id
   AND d.source_document_id = s.unit_id AND d.start_offset = t.start_offset AND d.end_offset = t.end_offset
JOIN books b ON b.owner_id = cai.owner_id AND b.id = cai.book_id
  AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
JOIN book_membership m ON m.owner_id = cai.owner_id AND m.book_id = cai.book_id AND m.state = 'active'
WHERE cai.owner_id = sqlc.arg('owner')
  AND source.current_content_revision_id = r.content_revision_id
  AND source.current_snapshot_id = r.snapshot_id
  AND t.surface = sqlc.arg('surface')::text AND t.raw_lemma = sqlc.arg('lemma')::text
ORDER BY b.title, cai.book_id, s.sentence_ordinal, t.token_ordinal;

-- name: ListOccurrenceLemmaCorrections :many
SELECT source_document_id, start_offset, end_offset, canonical_lemma, excluded
FROM occurrence_lemma_corrections
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book')
  AND corpus_id = sqlc.arg('corpus') AND analysis_run_id = sqlc.arg('analysis_run')
ORDER BY source_document_id, start_offset, end_offset;

-- name: GetCurrentAnalysisOccurrenceEvidence :one
SELECT t.language, t.canonical_lemma, t.upos, t.dependency
FROM current_analysis_identity cai
JOIN corpus_tokens t ON t.owner_id = cai.owner_id AND t.corpus_id = cai.corpus_id
  AND t.analysis_run_id = cai.analysis_run_id
JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.corpus_id = t.corpus_id
  AND s.analysis_run_id = t.analysis_run_id AND s.sentence_ordinal = t.sentence_ordinal
WHERE cai.owner_id = sqlc.arg('owner') AND cai.book_id = sqlc.arg('book')
  AND cai.analysis_run_id = sqlc.arg('analysis_run')
  AND s.unit_id = sqlc.arg('source_document_id')
  AND t.start_offset = sqlc.arg('start_offset') AND t.end_offset = sqlc.arg('end_offset');

-- name: PutOccurrenceLemmaCorrection :one
INSERT INTO occurrence_lemma_corrections(
  owner_id, book_id, corpus_id, analysis_run_id, source_document_id,
  start_offset, end_offset, canonical_lemma, normalization_profile, normalization_version, excluded
)
SELECT cai.owner_id, cai.book_id, cai.corpus_id, cai.analysis_run_id, s.unit_id,
       t.start_offset, t.end_offset, sqlc.narg('canonical_lemma'),
       sqlc.narg('normalization_profile'), sqlc.narg('normalization_version'), sqlc.arg('excluded')
FROM current_analysis_identity cai
JOIN corpus_tokens t ON t.owner_id = cai.owner_id AND t.corpus_id = cai.corpus_id
  AND t.analysis_run_id = cai.analysis_run_id
JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.corpus_id = t.corpus_id
  AND s.analysis_run_id = t.analysis_run_id AND s.sentence_ordinal = t.sentence_ordinal
WHERE cai.owner_id = sqlc.arg('owner') AND cai.book_id = sqlc.arg('book')
  AND cai.analysis_run_id = sqlc.arg('analysis_run')
  AND s.unit_id = sqlc.arg('source_document_id')
  AND t.start_offset = sqlc.arg('start_offset') AND t.end_offset = sqlc.arg('end_offset')
  AND t.surface = sqlc.arg('expected_surface') AND t.raw_lemma = sqlc.arg('expected_raw_lemma')
  AND t.canonical_lemma = sqlc.arg('expected_canonical_lemma')
  AND t.upos = sqlc.arg('expected_upos')
ON CONFLICT(owner_id, book_id, analysis_run_id, source_document_id, start_offset, end_offset)
DO UPDATE SET canonical_lemma = excluded.canonical_lemma,
                normalization_profile = excluded.normalization_profile,
                normalization_version = excluded.normalization_version,
                excluded = excluded.excluded,
                updated_at = now()
WHERE occurrence_lemma_corrections.canonical_lemma IS NOT DISTINCT FROM sqlc.narg('expected_corrected_lemma')
  AND occurrence_lemma_corrections.excluded = sqlc.arg('expected_excluded')
RETURNING owner_id::text, book_id::text, corpus_id::text, analysis_run_id::text,
          source_document_id, start_offset, end_offset, canonical_lemma,
           normalization_profile, normalization_version, excluded, created_at, updated_at;

-- name: DeleteOccurrenceLemmaCorrection :execrows
DELETE FROM occurrence_lemma_corrections d
USING current_analysis_identity cai, corpus_tokens t, corpus_sentences s
WHERE d.owner_id = sqlc.arg('owner') AND d.book_id = sqlc.arg('book')
  AND d.analysis_run_id = sqlc.arg('analysis_run')
  AND d.source_document_id = sqlc.arg('source_document_id')
  AND d.start_offset = sqlc.arg('start_offset') AND d.end_offset = sqlc.arg('end_offset')
  AND cai.owner_id = d.owner_id AND cai.book_id = d.book_id
  AND cai.corpus_id = d.corpus_id AND cai.analysis_run_id = d.analysis_run_id
  AND t.owner_id = cai.owner_id AND t.corpus_id = cai.corpus_id AND t.analysis_run_id = cai.analysis_run_id
  AND s.owner_id = t.owner_id AND s.corpus_id = t.corpus_id AND s.analysis_run_id = t.analysis_run_id
  AND s.sentence_ordinal = t.sentence_ordinal AND s.unit_id = d.source_document_id
   AND t.start_offset = d.start_offset AND t.end_offset = d.end_offset
    AND d.canonical_lemma IS NOT DISTINCT FROM sqlc.narg('expected_corrected_lemma')
    AND d.excluded = sqlc.arg('expected_excluded')
  AND t.surface = sqlc.arg('expected_surface') AND t.raw_lemma = sqlc.arg('expected_raw_lemma')
  AND t.canonical_lemma = sqlc.arg('expected_canonical_lemma') AND t.upos = sqlc.arg('expected_upos');

-- name: ListSelectedSentences :many
SELECT id, owner_id, corpus_id, sentence_key, sentence_text, source_location, language, canonical_lemma, upos, selection_rank, selection_score, selection_reasons, is_chosen, created_at
FROM example_sentences
WHERE owner_id = sqlc.arg('owner') AND corpus_id = sqlc.arg('corpus')
  AND language = sqlc.arg('language') AND canonical_lemma = sqlc.arg('canonical_lemma') AND upos = sqlc.arg('upos')
ORDER BY selection_rank;

-- name: ListReviewSentences :many
WITH latest AS (
  SELECT corpus_id FROM example_sentences
  WHERE owner_id = sqlc.arg('owner') AND language = sqlc.arg('language')
    AND canonical_lemma = sqlc.arg('canonical_lemma') AND upos = sqlc.arg('upos')
  GROUP BY corpus_id ORDER BY max(created_at) DESC, corpus_id DESC LIMIT 1
)
SELECT e.id, e.owner_id, e.corpus_id, e.sentence_key, e.sentence_text, e.source_location, e.language, e.canonical_lemma, e.upos, e.selection_rank, e.selection_score, e.selection_reasons, e.is_chosen, e.created_at
FROM example_sentences e
WHERE e.owner_id = sqlc.arg('owner') AND e.language = sqlc.arg('language')
  AND e.canonical_lemma = sqlc.arg('canonical_lemma') AND e.upos = sqlc.arg('upos')
  AND e.corpus_id = (SELECT corpus_id FROM latest)
ORDER BY e.is_chosen DESC, e.selection_rank, e.id;

-- name: ListReviewSentencesForBook :many
SELECT e.id, e.owner_id, e.corpus_id, e.sentence_key, e.sentence_text, e.source_location, e.language, e.canonical_lemma, e.upos, e.selection_rank, e.selection_score, e.selection_reasons, e.is_chosen, e.created_at
FROM example_sentences e
JOIN corpora c ON c.owner_id = e.owner_id AND c.id = e.corpus_id
WHERE e.owner_id = sqlc.arg('owner') AND c.source_material_id = sqlc.arg('book')
  AND e.language = sqlc.arg('language') AND e.canonical_lemma = sqlc.arg('canonical_lemma') AND e.upos = sqlc.arg('upos')
ORDER BY e.is_chosen DESC, e.selection_rank, e.id;

