-- Concordance occurrence queries read from the shared occurrence model.

-- name: GetVocabularySentenceStudy :many
-- Raw analyzer token evidence for one sentence. Occurrence decisions are not
-- joined here: the caller attributes them only after proving uniqueness over
-- every persisted word the decision addresses (GetSentenceStudyDecisions).
SELECT COALESCE(t.surface, '')::text AS surface,
       COALESCE(t.raw_lemma, '')::text AS raw_lemma,
       COALESCE(t.upos, '')::text AS upos,
       COALESCE(t.dependency, '')::text AS dependency,
       COALESCE(t.head, 0)::bigint AS head,
       COALESCE(h.surface, '')::text AS head_surface,
       COALESCE(t.token_ordinal, -1)::bigint AS token_ordinal,
       COALESCE(t.start_offset, -1)::bigint AS start_offset,
       COALESCE(t.end_offset, -1)::bigint AS end_offset,
       COALESCE(t.morphology, '{}'::jsonb)::jsonb AS morphology,
       COALESCE(t.canonical_lemma, '')::text AS canonical_lemma,
       COALESCE(t.headword, '')::text AS headword,
       ar.syntax_state,
       s.sentence_text,
       s.start_offset::bigint AS sentence_start_offset,
       s.end_offset::bigint AS sentence_end_offset,
       u.text AS unit_text,
       s.sentence_ordinal,
       sqlc.arg('target_ordinal')::bigint AS target_ordinal,
       s.unit_id,
       ca.source_material_id::text AS source_material_id,
       ca.snapshot_id::text AS snapshot_id,
       ca.book_id::text AS book_id,
       b.title AS book_title,
       u.title AS chapter_title
  FROM current_analysis_identity ca
  JOIN analysis_runs ar ON ar.owner_id=ca.owner_id AND ar.id=ca.analysis_run_id
  JOIN books b ON b.owner_id=ca.owner_id AND b.id=ca.book_id
  JOIN corpus_sentences s ON s.owner_id=ca.owner_id AND s.analysis_run_id=ca.analysis_run_id
       AND s.corpus_id=ca.corpus_id AND s.unit_id=sqlc.arg('unit_id')
  LEFT JOIN corpus_tokens t ON t.owner_id=s.owner_id AND t.analysis_run_id=s.analysis_run_id
       AND t.corpus_id=s.corpus_id AND t.sentence_ordinal=s.sentence_ordinal
  JOIN source_material_units u ON u.owner_id=ca.owner_id AND u.source_material_id=ca.source_material_id
       AND u.snapshot_id=ca.snapshot_id AND u.unit_id=s.unit_id
  LEFT JOIN corpus_tokens h ON h.owner_id=t.owner_id AND h.language=t.language
       AND h.analysis_run_id=t.analysis_run_id AND h.corpus_id=t.corpus_id
       AND h.sentence_ordinal=t.sentence_ordinal AND h.token_ordinal=t.head
 WHERE ca.owner_id=sqlc.arg('owner') AND ca.book_id=sqlc.arg('book')
   AND ca.analysis_run_id=sqlc.arg('analysis_run') AND ca.corpus_id=sqlc.arg('corpus')
   AND s.sentence_ordinal=sqlc.arg('sentence_ordinal')
 ORDER BY t.token_ordinal;

-- name: GetSentenceStudyDecisions :many
-- Stored occurrence decisions addressing the selected sentence, each with the
-- number of persisted analyzed words its exact owner/Book/analysis/
-- source-document/span addresses. The count spans the whole addressed scope,
-- not just the selected sentence, so a filtered-away sibling still proves the
-- decision is not unique. The decision is attributed only when this count is
-- exactly one and the addressed word is in the selected sentence.
SELECT d.start_offset::bigint AS start_offset,
       d.end_offset::bigint AS end_offset,
       COALESCE(d.canonical_lemma, '')::text AS canonical_lemma,
       d.excluded::boolean AS excluded,
       (SELECT count(*)
          FROM corpus_tokens w
          JOIN corpus_sentences ws ON ws.owner_id=w.owner_id
            AND ws.analysis_run_id=w.analysis_run_id AND ws.corpus_id=w.corpus_id
            AND ws.sentence_ordinal=w.sentence_ordinal
         WHERE w.owner_id=d.owner_id AND w.analysis_run_id=d.analysis_run_id
           AND w.corpus_id=d.corpus_id AND ws.unit_id=d.source_document_id
           AND w.start_offset=d.start_offset AND w.end_offset=d.end_offset)::bigint AS addressed_words
  FROM occurrence_lemma_corrections d
 WHERE d.owner_id=sqlc.arg('owner') AND d.book_id=sqlc.arg('book')
   AND d.analysis_run_id=sqlc.arg('analysis_run') AND d.corpus_id=sqlc.arg('corpus')
   AND d.source_document_id=sqlc.arg('unit_id')
   AND d.start_offset >= sqlc.arg('sentence_start')::bigint
   AND d.end_offset <= sqlc.arg('sentence_end')::bigint
 ORDER BY d.start_offset, d.end_offset;

-- name: ListVocabularyConcordance :many
SELECT o.surface,
       o.canonical_lemma,
       o.upos,
       o.dependency,
       o.head_ordinal,
       o.head_surface,
       o.sentence_text,
       o.sentence_start_offset,
       o.sentence_end_offset,
       o.unit_start_offset,
       o.unit_end_offset,
       o.book_start_offset,
       o.book_end_offset,
       o.book_id,
       o.book_title,
       o.source_material_id,
       o.analysis_run_id,
       o.corpus_id,
       o.unit_id,
       o.chapter_title,
       o.unit_order,
       o.sentence_ordinal,
       o.token_ordinal,
       ca.snapshot_id::text AS snapshot_id,
       u.text AS unit_text,
       s.start_offset::bigint AS sentence_unit_start_offset,
       s.end_offset::bigint AS sentence_unit_end_offset,
       t.raw_lemma,
       COALESCE(t.headword, '')::text AS headword,
       COALESCE(d.canonical_lemma, o.canonical_lemma)::text AS effective_lemma,
       (d.canonical_lemma IS NOT NULL AND NOT COALESCE(d.excluded, false))::boolean AS corrected,
       COALESCE(d.excluded, false)::boolean AS excluded,
       -- Computed over the whole matched set, before paging.
       COALESCE(bool_or(o.book_id=sqlc.arg('priority_book')::text) OVER (), false)::boolean AS priority_matched
  FROM concordance_occurrences o
  -- The view exposes these UUIDs as text; keep the indexed token keys uncast.
  JOIN corpus_tokens t ON t.owner_id=o.owner_id AND t.language=o.language
    AND t.analysis_run_id=o.analysis_run_id::uuid AND t.corpus_id=o.corpus_id::uuid
    AND t.sentence_ordinal=o.sentence_ordinal AND t.token_ordinal=o.token_ordinal
  -- The exact snapshot and the sentence's unit-relative range let the caller
  -- supply structural facts to Go. SQL never decides role eligibility itself.
  JOIN current_analysis_identity ca ON ca.owner_id=o.owner_id AND ca.book_id=o.book_id::uuid
    AND ca.analysis_run_id=o.analysis_run_id::uuid AND ca.corpus_id=o.corpus_id::uuid
  JOIN corpus_sentences s ON s.owner_id=o.owner_id AND s.analysis_run_id=o.analysis_run_id::uuid
    AND s.corpus_id=o.corpus_id::uuid AND s.sentence_ordinal=o.sentence_ordinal
  -- The sentence's exact unit text, so Go can derive quiet attribution from
  -- the same snapshot's role spans. SQL never chooses a speaker itself.
  JOIN source_material_units u ON u.owner_id=o.owner_id
    AND u.source_material_id=o.source_material_id::uuid
    AND u.snapshot_id=ca.snapshot_id AND u.unit_id=o.unit_id
  LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=o.owner_id
    AND d.book_id=o.book_id::uuid AND d.corpus_id=o.corpus_id::uuid
    AND d.analysis_run_id=o.analysis_run_id::uuid AND d.source_document_id=o.unit_id
    AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
 WHERE o.owner_id=sqlc.arg('owner') AND o.language=sqlc.arg('language')
   AND NOT COALESCE(d.excluded, false)
   AND ((sqlc.arg('match')::text='lemma'
         AND COALESCE(d.canonical_lemma, o.canonical_lemma)=sqlc.arg('term')::text
         AND (sqlc.arg('upos')::text='' OR o.upos=sqlc.arg('upos')))
     OR (sqlc.arg('match')::text='form'
         -- A dedicated-profile language (the Go side's DedicatedProfileLanguage)
         -- matches its stored Go-computed ancient-greek/1 search key, never SQL
         -- lower(), so NFC/NFD, case and final-sigma variants unify while
         -- accents stay significant. Every other language keeps today's
         -- case-and-final-sigma form matching.
         AND ((sqlc.arg('form_on_key')::boolean AND t.search_key = sqlc.arg('term')::text)
           OR (NOT sqlc.arg('form_on_key')::boolean AND translate(lower(o.surface), 'ς', 'σ') = sqlc.arg('term')::text))))
 ORDER BY (o.book_id=sqlc.arg('priority_book')::text) DESC,
          lower(o.book_title), o.book_title, o.book_id,
          o.unit_order, o.sentence_ordinal, o.token_ordinal
 -- The caller starts one row early on later pages and reads one row past the
 -- page: the first row decides "continued", the last decides "next".
 LIMIT 27 OFFSET sqlc.arg('offset')::bigint;

-- name: GetConcordancePriorityBook :one
-- An owned Book in the study language, with whether it has a current analysis
-- that can contribute occurrences.
SELECT b.title,
       EXISTS (SELECT 1 FROM current_analysis_identity ca
                WHERE ca.owner_id=b.owner_id AND ca.book_id=b.id)::boolean AS analyzed
  FROM books b
 WHERE b.owner_id=sqlc.arg('owner') AND b.id=sqlc.arg('book')
   AND b.language_state='chosen' AND b.language_tag=sqlc.arg('language');

-- name: ConcordanceLemmaEvidenced :one
-- A lemma is recognized only from non-excluded effective identities in the
-- eligible owner/language corpus, across every evidenced part of speech.
SELECT EXISTS (
  SELECT 1
    FROM concordance_occurrences o
    LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=o.owner_id
      AND d.book_id=o.book_id::uuid AND d.corpus_id=o.corpus_id::uuid
      AND d.analysis_run_id=o.analysis_run_id::uuid AND d.source_document_id=o.unit_id
      AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
   WHERE o.owner_id=sqlc.arg('owner') AND o.language=sqlc.arg('language')
     AND NOT COALESCE(d.excluded, false)
     AND COALESCE(d.canonical_lemma, o.canonical_lemma)=sqlc.arg('term')::text
)::boolean AS evidenced;