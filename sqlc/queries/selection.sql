-- Sentence selection and review queries. SelectAcquisitionCandidate is the
-- static replacement for the runtime string-concatenated acquisition query:
-- an unconditional predicate of the form ($book = '' OR source = $book) keeps
-- the statement statically analyzable.

-- name: SelectAcquisitionCandidate :one
SELECT sc.corpus_id, sc.eligible_sentence_refs
FROM selection_candidates sc
JOIN corpora c ON c.owner_id = sc.owner_id AND c.id::text = sc.corpus_id
WHERE sc.owner_id = sqlc.arg('owner')
  AND sc.language = sqlc.arg('language')
  AND sc.canonical_lemma = sqlc.arg('canonical_lemma')
  AND sc.upos = sqlc.arg('upos')
   AND (sqlc.arg('book')::text = '' OR c.source_material_id::text = sqlc.arg('book')::text)
ORDER BY sc.selected_at DESC, sc.corpus_id DESC
LIMIT 1;

-- name: ListLemmaReviewOccurrences :many
SELECT c.id::text AS corpus_id, c.analysis_run_id::text AS analysis_run_id,
       t.sentence_ordinal, t.token_ordinal, t.surface, t.raw_lemma,
       t.canonical_lemma, t.upos, t.start_offset, t.end_offset,
       s.sentence_text, s.unit_id,
       COALESCE(d.canonical_lemma, '')::text AS corrected_lemma,
       COALESCE(d.excluded, false)::boolean AS excluded,
       COALESCE(f.reason, '')::text AS review_flag_reason,
       COALESCE(f.evidence_provenance::text, '{}')::text AS review_flag_provenance,
       COALESCE(f.resolution, '')::text AS review_flag_resolution
FROM book_current_analyses cai
JOIN source_materials source ON source.owner_id = cai.owner_id
  AND source.id = cai.source_material_id AND source.book_id = cai.book_id
JOIN analysis_runs r ON r.owner_id = cai.owner_id AND r.id = cai.analysis_run_id
  AND r.source_material_id = cai.source_material_id AND r.state = 'completed'
JOIN corpora c ON c.owner_id = r.owner_id AND c.id = r.corpus_id
  AND c.source_material_id = r.source_material_id AND c.analysis_run_id = r.id
  AND c.status = 'complete'
JOIN corpus_tokens t ON t.owner_id = c.owner_id AND t.corpus_id = c.id
  AND t.analysis_run_id = c.analysis_run_id
JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.corpus_id = t.corpus_id
  AND s.analysis_run_id = t.analysis_run_id AND s.sentence_ordinal = t.sentence_ordinal
LEFT JOIN occurrence_lemma_corrections d ON d.owner_id = cai.owner_id AND d.book_id = cai.book_id
  AND d.corpus_id = c.id AND d.analysis_run_id = c.analysis_run_id
   AND d.source_document_id = s.unit_id AND d.start_offset = t.start_offset AND d.end_offset = t.end_offset
LEFT JOIN occurrence_lemma_review_flags f ON f.owner_id = cai.owner_id AND f.book_id = cai.book_id
  AND f.analysis_run_id = c.analysis_run_id AND f.source_document_id = s.unit_id
  AND f.start_offset = t.start_offset AND f.end_offset = t.end_offset
WHERE cai.owner_id = sqlc.arg('owner') AND cai.book_id = sqlc.arg('book')
  AND source.current_content_revision_id = r.content_revision_id
  AND source.current_snapshot_id = r.snapshot_id
   AND (sqlc.arg('surface')::text = '' OR t.surface = sqlc.arg('surface'))
  AND t.upos IN ('NOUN', 'VERB', 'ADJ', 'ADV')
ORDER BY s.sentence_ordinal, t.token_ordinal;

-- name: ListOccurrenceLemmaCorrections :many
SELECT source_document_id, start_offset, end_offset, canonical_lemma, excluded
FROM occurrence_lemma_corrections
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book')
  AND corpus_id = sqlc.arg('corpus') AND analysis_run_id = sqlc.arg('analysis_run')
ORDER BY source_document_id, start_offset, end_offset;

-- name: HasCurrentLemmaCorrections :one
SELECT EXISTS (
  SELECT 1 FROM occurrence_lemma_corrections d
  JOIN book_current_analyses cai ON cai.owner_id = d.owner_id
    AND cai.book_id = d.book_id AND cai.analysis_run_id = d.analysis_run_id
  JOIN source_materials source ON source.owner_id = cai.owner_id
    AND source.id = cai.source_material_id AND source.book_id = cai.book_id
  JOIN analysis_runs r ON r.owner_id = cai.owner_id AND r.id = cai.analysis_run_id
    AND r.source_material_id = cai.source_material_id AND r.state = 'completed'
  JOIN corpora c ON c.owner_id = r.owner_id AND c.id = r.corpus_id
    AND c.id = d.corpus_id AND c.source_material_id = r.source_material_id
    AND c.analysis_run_id = r.id AND c.status = 'complete'
  WHERE d.owner_id = sqlc.arg('owner') AND d.book_id = sqlc.arg('book')
    AND source.current_content_revision_id = r.content_revision_id
    AND source.current_snapshot_id = r.snapshot_id
);

-- name: HasCurrentLemmaCorrectionsForAnalysis :one
SELECT EXISTS (
  SELECT 1 FROM occurrence_lemma_corrections d
  JOIN current_analysis_identity cai ON cai.owner_id = d.owner_id AND cai.book_id = d.book_id
    AND cai.corpus_id = d.corpus_id AND cai.analysis_run_id = d.analysis_run_id
  WHERE d.owner_id = sqlc.arg('owner') AND d.book_id = sqlc.arg('book')
    AND d.corpus_id = sqlc.arg('corpus') AND d.analysis_run_id = sqlc.arg('analysis_run')
);

-- name: ListAnalysisTokenEvidence :many
SELECT t.language, t.sentence_ordinal, t.token_ordinal, t.surface, t.raw_lemma,
       t.canonical_lemma, t.upos, t.dependency, t.head, t.morphology,
       t.start_offset, t.end_offset, s.unit_id, s.sentence_text
FROM corpora c
JOIN corpus_tokens t ON t.owner_id = c.owner_id AND t.corpus_id = c.id
  AND t.analysis_run_id = c.analysis_run_id
JOIN corpus_sentences s ON s.owner_id = t.owner_id AND s.corpus_id = t.corpus_id
  AND s.analysis_run_id = t.analysis_run_id AND s.sentence_ordinal = t.sentence_ordinal
WHERE c.owner_id = sqlc.arg('owner') AND c.id = sqlc.arg('corpus')
  AND c.analysis_run_id = sqlc.arg('analysis_run')
ORDER BY t.sentence_ordinal, t.token_ordinal;

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
   AND t.upos IN ('NOUN', 'VERB', 'ADJ', 'ADV')
   AND NOT EXISTS (
     SELECT 1 FROM primary_goals pg
     WHERE pg.owner_id = cai.owner_id AND pg.book_id = cai.book_id
   )
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

-- name: UpsertReviewSentenceFromAnalysis :one
INSERT INTO example_sentences(owner_id, corpus_id, sentence_key, sentence_text, source_location, language, canonical_lemma, upos, selection_rank, selection_score, selection_reasons, is_chosen)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, 0, '[]', true)
ON CONFLICT(owner_id, corpus_id, sentence_key) DO UPDATE SET
  sentence_text = excluded.sentence_text,
  source_location = excluded.source_location,
  language = excluded.language,
  canonical_lemma = excluded.canonical_lemma,
  upos = excluded.upos,
  selection_rank = excluded.selection_rank,
  selection_score = excluded.selection_score,
  selection_reasons = excluded.selection_reasons,
  is_chosen = true
RETURNING id, owner_id, corpus_id, sentence_key, sentence_text, source_location, language, canonical_lemma, upos, selection_rank, selection_score, selection_reasons, is_chosen, created_at;

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

-- name: ListSelectionCandidatesForBook :many
SELECT sc.owner_id::text AS owner_id,
       sc.corpus_id,
       sc.language,
       sc.canonical_lemma,
       sc.upos,
       sc.occurrence_count,
       sc.observed_forms,
       sc.eligible_sentence_refs,
       sc.provenance,
       sc.selected_at,
       COALESCE(first_seen.start_offset, 9223372036854775807::bigint)::bigint AS first_encounter
FROM selection_candidates sc
JOIN corpora c ON c.owner_id = sc.owner_id AND c.id::text = sc.corpus_id
LEFT JOIN LATERAL (
  SELECT MIN(COALESCE(ref->'location'->>'start_offset', ref->'location'->>'StartOffset', ref->'Location'->>'StartOffset')::bigint) AS start_offset
  FROM jsonb_array_elements(sc.eligible_sentence_refs) ref
) first_seen ON true
WHERE sc.owner_id = sqlc.arg('owner') AND c.source_material_id = sqlc.arg('book')
ORDER BY sc.language, sc.canonical_lemma, sc.upos;

-- name: ListSelectionCandidatesForCorpus :many
SELECT sc.owner_id::text AS owner_id,
       sc.corpus_id,
       sc.language,
       sc.canonical_lemma,
       sc.upos,
       sc.occurrence_count,
       sc.observed_forms,
       sc.eligible_sentence_refs,
       sc.provenance,
       sc.selected_at,
       COALESCE(first_seen.start_offset, 9223372036854775807::bigint)::bigint AS first_encounter
FROM selection_candidates sc
JOIN corpora c ON c.owner_id = sc.owner_id AND c.id::text = sc.corpus_id
LEFT JOIN LATERAL (
  SELECT MIN(COALESCE(ref->'location'->>'start_offset', ref->'location'->>'StartOffset', ref->'Location'->>'StartOffset')::bigint) AS start_offset
  FROM jsonb_array_elements(sc.eligible_sentence_refs) ref
) first_seen ON true
WHERE sc.owner_id = sqlc.arg('owner') AND sc.corpus_id = sqlc.arg('corpus')
ORDER BY sc.language, sc.canonical_lemma, sc.upos;

-- name: GetCoverageEntryForBook :one
SELECT sc.owner_id::text AS owner_id,
       sc.language,
       sc.canonical_lemma,
       sc.upos,
       COALESCE(e.sentence_text, '') AS sentence,
       ''::text AS translation,
       ''::text AS target_word,
       COALESCE(sl.morphologies::text, '[]'::varchar)::varchar AS morphology,
       sm.title AS source_document,
       ''::text AS notes,
       sqlc.arg('first_encounter')::bigint AS first_encounter
FROM selection_candidates sc
JOIN corpora c ON c.owner_id = sc.owner_id AND c.id::text = sc.corpus_id
JOIN source_materials sm ON sm.owner_id = c.owner_id AND sm.id = c.source_material_id
LEFT JOIN LATERAL (
  SELECT ex.*
  FROM example_sentences ex
  WHERE ex.owner_id = sc.owner_id AND ex.corpus_id = c.id
    AND ex.language = sc.language AND ex.canonical_lemma = sc.canonical_lemma AND ex.upos = sc.upos
  ORDER BY ex.is_chosen DESC, ex.selection_rank, ex.id
  LIMIT 1
) e ON true
LEFT JOIN LATERAL (
  SELECT jsonb_agg(morphology ORDER BY frequency DESC, morphology::text) AS morphologies
  FROM shared_lemmas
  WHERE content_hash = c.artifact_hash AND language = sc.language
    AND canonical_lemma = sc.canonical_lemma AND upos = sc.upos
) sl ON true
WHERE sc.owner_id = sqlc.arg('owner') AND sm.id = sqlc.arg('book')
  AND sc.corpus_id = sqlc.arg('corpus') AND sc.language = sqlc.arg('language')
  AND sc.canonical_lemma = sqlc.arg('canonical_lemma') AND sc.upos = sqlc.arg('upos');

-- name: GetCoverageEntryForCorpus :one
SELECT sc.owner_id::text AS owner_id,
       sc.language,
       sc.canonical_lemma,
       sc.upos,
       COALESCE(e.sentence_text, '') AS sentence,
       ''::text AS translation,
       ''::text AS target_word,
       COALESCE(sl.morphologies::text, '[]'::varchar)::varchar AS morphology,
       sm.title AS source_document,
       ''::text AS notes,
       sqlc.arg('first_encounter')::bigint AS first_encounter
FROM selection_candidates sc
JOIN corpora c ON c.owner_id = sc.owner_id AND c.id::text = sc.corpus_id
JOIN source_materials sm ON sm.owner_id = c.owner_id AND sm.id = c.source_material_id
LEFT JOIN LATERAL (
  SELECT ex.*
  FROM example_sentences ex
  WHERE ex.owner_id = sc.owner_id AND ex.corpus_id = c.id
    AND ex.language = sc.language AND ex.canonical_lemma = sc.canonical_lemma AND ex.upos = sc.upos
  ORDER BY ex.is_chosen DESC, ex.selection_rank, ex.id
  LIMIT 1
) e ON true
LEFT JOIN LATERAL (
  SELECT jsonb_agg(morphology ORDER BY frequency DESC, morphology::text) AS morphologies
  FROM shared_lemmas
  WHERE content_hash = c.artifact_hash AND language = sc.language
    AND canonical_lemma = sc.canonical_lemma AND upos = sc.upos
) sl ON true
WHERE sc.owner_id = sqlc.arg('owner') AND sc.corpus_id = sqlc.arg('corpus')
  AND sc.language = sqlc.arg('language') AND sc.canonical_lemma = sqlc.arg('canonical_lemma')
  AND sc.upos = sqlc.arg('upos');
