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
       -- A token headed by an unprinted node is self-headed, so the join below
       -- would return the token itself. Its head_surface is empty instead.
       CASE WHEN t.head_unprinted_node IS NULL THEN COALESCE(h.surface, '') ELSE '' END::text AS head_surface,
       COALESCE(t.head_unprinted_node, '')::text AS head_unprinted_node,
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

-- name: GetSentenceStudyUnprintedNodes :many
-- The Unprinted syntax nodes of one sentence of a native_agdt run. A run of any
-- other syntax state stores none, and the caller does not ask.
SELECT n.source_id,
       n.dependency,
       n.head_token_ordinal,
       COALESCE(n.head_unprinted_node, '')::text AS head_unprinted_node
  FROM corpus_unprinted_syntax_nodes n
 WHERE n.owner_id=sqlc.arg('owner') AND n.analysis_run_id=sqlc.arg('analysis_run')
   AND n.corpus_id=sqlc.arg('corpus') AND n.sentence_ordinal=sqlc.arg('sentence_ordinal')
 ORDER BY n.source_id;

-- name: GetSentenceStudyConstructions :many
-- One sentence's Construction occurrences, read only when the run's marker
-- carries the current detector version. No marker, or an older one, returns no
-- rows.
SELECT c.construction_id,
       COALESCE(c.subtype_id, '')::text AS subtype_id,
       c.anchor_token_ordinal,
       c.extent_token_ordinals::bigint[] AS extent_token_ordinals
  FROM construction_occurrences c
  JOIN construction_run_markers m ON m.analysis_run_id=c.analysis_run_id
       AND m.owner_id=c.owner_id AND m.detector_version=sqlc.arg('detector_version')
 WHERE c.owner_id=sqlc.arg('owner') AND c.analysis_run_id=sqlc.arg('analysis_run')
   AND c.corpus_id=sqlc.arg('corpus') AND c.sentence_ordinal=sqlc.arg('sentence_ordinal')
 ORDER BY c.extent_token_ordinals[1], c.anchor_token_ordinal, c.construction_id;

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
-- One page of the concordance_occurrences model, read selectively (#2595).
-- Filtering the view's output joins and sorts the whole corpus before the
-- lemma or form filter applies. This query states the view's eligibility over
-- the base tables instead, in fenced stages whose plans stay small even when
-- the planner has no statistics (small tables such as analysis_runs are never
-- auto-analyzed, so each looks like one row):
--   candidates  token keys that can match: the indexed analyzer lemma, the
--               decisions that name the lemma, or one pass over the owner's
--               tokens for a form. Each branch is gated by the match mode, so
--               a cached generic plan never runs another mode's branches.
--   located     each candidate's sentence, giving its unit.
--   eligible    the owner's current analyses of chosen-language Books.
--   current     candidates in an eligible analysis, with their Book.
--   ranked      the candidate's current-snapshot unit and occurrence
--               decision, then the filter, order and page as narrow rows.
--   (final)     the page's at most 27 rows fetch the wide text.
-- Each stage joins one relation to the previous stage. Later stages restrict
-- big and unit tables only through join keys, never a literal owner, so the
-- planner cannot mistake them for one row and rescan the candidates per row.
-- The same eligibility is stated three times: in the concordance_occurrences
-- view (whose migration is immutable), here across located, eligible, current
-- and the units join in ranked, and in ConcordanceLemmaEvidenced. Change all
-- three together; TestConcordanceLookupMatchesTheOccurrenceView compares them.
WITH candidates AS MATERIALIZED (
  SELECT t.owner_id, t.analysis_run_id, t.corpus_id, t.sentence_ordinal, t.token_ordinal,
         t.canonical_lemma, t.start_offset, t.end_offset
    FROM corpus_tokens t
   WHERE sqlc.arg('match')::text = 'lemma'
     AND t.owner_id = sqlc.arg('owner')::uuid AND t.language = sqlc.arg('language')::text
     AND t.canonical_lemma = sqlc.arg('term')::text
     AND (sqlc.arg('upos')::text = '' OR t.upos = sqlc.arg('upos')::text)
  UNION ALL
  -- A learner decision can attribute an occurrence to the lemma although its
  -- analyzer lemma differs. Decisions are few, so start from them; the
  -- analyzer-lemma branch above already holds every other occurrence.
  SELECT DISTINCT t.owner_id, t.analysis_run_id, t.corpus_id, t.sentence_ordinal, t.token_ordinal,
         t.canonical_lemma, t.start_offset, t.end_offset
    FROM occurrence_lemma_corrections dc
    JOIN corpus_sentences ds ON ds.analysis_run_id = dc.analysis_run_id
      AND ds.owner_id = dc.owner_id AND ds.corpus_id = dc.corpus_id
      AND ds.unit_id = dc.source_document_id
    JOIN corpus_tokens t ON t.analysis_run_id = ds.analysis_run_id
      AND t.sentence_ordinal = ds.sentence_ordinal AND t.owner_id = ds.owner_id
      AND t.corpus_id = ds.corpus_id
      AND t.start_offset = dc.start_offset AND t.end_offset = dc.end_offset
   WHERE sqlc.arg('match')::text = 'lemma'
     AND dc.owner_id = sqlc.arg('owner')::uuid AND dc.canonical_lemma = sqlc.arg('term')::text
     AND t.language = sqlc.arg('language')::text
     AND t.canonical_lemma <> sqlc.arg('term')::text
     AND (sqlc.arg('upos')::text = '' OR t.upos = sqlc.arg('upos')::text)
  UNION ALL
  SELECT t.owner_id, t.analysis_run_id, t.corpus_id, t.sentence_ordinal, t.token_ordinal,
         t.canonical_lemma, t.start_offset, t.end_offset
    FROM corpus_tokens t
   WHERE sqlc.arg('match')::text = 'form'
     AND t.owner_id = sqlc.arg('owner')::uuid AND t.language = sqlc.arg('language')::text
     -- A dedicated-profile language (the Go side's DedicatedProfileLanguage)
     -- matches its stored Go-computed ancient-greek/1 search key, never SQL
     -- lower(), so NFC/NFD, case and final-sigma variants unify while
     -- accents stay significant. Every other language keeps today's
     -- case-and-final-sigma form matching.
     AND ((sqlc.arg('form_on_key')::boolean AND t.search_key = sqlc.arg('term')::text)
       OR (NOT sqlc.arg('form_on_key')::boolean
           AND translate(lower(t.surface), 'ς', 'σ') = sqlc.arg('term')::text))
),
located AS MATERIALIZED (
  SELECT c.*, s.unit_id
    FROM candidates c
    JOIN corpus_sentences s ON s.analysis_run_id = c.analysis_run_id
      AND s.sentence_ordinal = c.sentence_ordinal
      AND s.owner_id = c.owner_id AND s.corpus_id = c.corpus_id
),
eligible AS MATERIALIZED (
  SELECT ca.owner_id, ca.analysis_run_id, ca.corpus_id, ca.book_id,
         ca.source_material_id, ca.snapshot_id, b.title AS book_title
    FROM current_analysis_identity ca
    JOIN books b ON b.owner_id = ca.owner_id AND b.id = ca.book_id
      AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
   WHERE ca.owner_id = sqlc.arg('owner')::uuid
),
current AS MATERIALIZED (
  SELECT l.*, e.book_id, e.source_material_id, e.snapshot_id, e.book_title
    FROM located l
    JOIN eligible e ON e.owner_id = l.owner_id
      AND e.analysis_run_id = l.analysis_run_id AND e.corpus_id = l.corpus_id
),
ranked AS MATERIALIZED (
  SELECT c.analysis_run_id, c.corpus_id, c.sentence_ordinal, c.token_ordinal,
         c.book_id, c.source_material_id, c.snapshot_id, c.book_title,
         c.unit_id, u.unit_order,
         d.canonical_lemma AS decision_lemma, d.excluded AS decision_excluded,
         -- Computed over the whole matched set, before paging.
         COALESCE(bool_or(c.book_id::text = sqlc.arg('priority_book')::text) OVER (), false) AS priority_matched
    FROM current c
    JOIN source_material_units u ON u.owner_id = c.owner_id
      AND u.source_material_id = c.source_material_id
      AND u.snapshot_id = c.snapshot_id AND u.unit_id = c.unit_id
    LEFT JOIN occurrence_lemma_corrections d ON d.owner_id = c.owner_id
      AND d.book_id = c.book_id AND d.corpus_id = c.corpus_id
      AND d.analysis_run_id = c.analysis_run_id AND d.source_document_id = c.unit_id
      AND d.start_offset = c.start_offset AND d.end_offset = c.end_offset
   WHERE NOT COALESCE(d.excluded, false)
     AND (sqlc.arg('match')::text <> 'lemma'
       OR COALESCE(d.canonical_lemma, c.canonical_lemma) = sqlc.arg('term')::text)
   ORDER BY (c.book_id::text = sqlc.arg('priority_book')::text) DESC,
            lower(c.book_title), c.book_title, c.book_id::text,
            u.unit_order, c.sentence_ordinal, c.token_ordinal
   -- The caller starts one row early on later pages and reads one row past
   -- the page: the first row decides "continued", the last decides "next".
   LIMIT 27 OFFSET sqlc.arg('offset')::bigint
)
SELECT t.surface,
       t.canonical_lemma,
       t.upos,
       t.dependency,
       t.head AS head_ordinal,
       head.surface AS head_surface,
       s.sentence_text,
       (t.start_offset - s.start_offset)::bigint AS sentence_start_offset,
       (t.end_offset - s.start_offset)::bigint AS sentence_end_offset,
       t.start_offset AS unit_start_offset,
       t.end_offset AS unit_end_offset,
       (u.start_offset + t.start_offset)::bigint AS book_start_offset,
       (u.start_offset + t.end_offset)::bigint AS book_end_offset,
       r.book_id::text AS book_id,
       r.book_title,
       r.source_material_id::text AS source_material_id,
       r.analysis_run_id::text AS analysis_run_id,
       r.corpus_id::text AS corpus_id,
       r.unit_id,
       u.title AS chapter_title,
       r.unit_order,
       r.sentence_ordinal,
       r.token_ordinal,
       r.snapshot_id::text AS snapshot_id,
       -- The sentence's exact unit text and unit-relative range let Go derive
       -- speaker restrictions and quiet attribution from the same snapshot's
       -- role spans. SQL never decides either itself.
       u.text AS unit_text,
       s.start_offset::bigint AS sentence_unit_start_offset,
       s.end_offset::bigint AS sentence_unit_end_offset,
       t.raw_lemma,
       COALESCE(t.headword, '')::text AS headword,
       COALESCE(r.decision_lemma, t.canonical_lemma)::text AS effective_lemma,
       (r.decision_lemma IS NOT NULL AND NOT COALESCE(r.decision_excluded, false))::boolean AS corrected,
       COALESCE(r.decision_excluded, false)::boolean AS excluded,
       r.priority_matched::boolean AS priority_matched
  FROM ranked r
  JOIN corpus_tokens t ON t.analysis_run_id = r.analysis_run_id
    AND t.sentence_ordinal = r.sentence_ordinal AND t.token_ordinal = r.token_ordinal
  JOIN corpus_sentences s ON s.analysis_run_id = r.analysis_run_id
    AND s.sentence_ordinal = r.sentence_ordinal
  JOIN source_material_units u ON u.owner_id = t.owner_id
    AND u.source_material_id = r.source_material_id
    AND u.snapshot_id = r.snapshot_id AND u.unit_id = r.unit_id
  LEFT JOIN corpus_tokens head ON head.owner_id = t.owner_id AND head.language = t.language
    AND head.analysis_run_id = t.analysis_run_id AND head.corpus_id = t.corpus_id
    AND head.sentence_ordinal = t.sentence_ordinal AND head.token_ordinal = t.head
 ORDER BY (r.book_id::text = sqlc.arg('priority_book')::text) DESC,
          lower(r.book_title), r.book_title, r.book_id::text,
          r.unit_order, r.sentence_ordinal, r.token_ordinal;

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
-- concordance_occurrences' eligibility is stated over the base tables and
-- entered through the term (the indexed analyzer lemma, or a decision that
-- names it), so a term that is not a lemma never reads the whole corpus
-- (#2595). Keep the eligibility identical to the concordance_occurrences view
-- and to ListVocabularyConcordance's located, eligible, current and ranked stages.
-- The decision branch omits the listing's DISTINCT and analyzer-lemma
-- exclusion: a duplicate cannot change EXISTS.
SELECT EXISTS (
  SELECT 1
    FROM (SELECT t.analysis_run_id, t.corpus_id, t.sentence_ordinal,
                 t.canonical_lemma, t.start_offset, t.end_offset
            FROM corpus_tokens t
           WHERE t.owner_id = sqlc.arg('owner')::uuid AND t.language = sqlc.arg('language')::text
             AND t.canonical_lemma = sqlc.arg('term')::text
          UNION ALL
          SELECT t.analysis_run_id, t.corpus_id, t.sentence_ordinal,
                 t.canonical_lemma, t.start_offset, t.end_offset
            FROM occurrence_lemma_corrections dc
            JOIN corpus_sentences ds ON ds.analysis_run_id = dc.analysis_run_id
              AND ds.owner_id = dc.owner_id AND ds.corpus_id = dc.corpus_id
              AND ds.unit_id = dc.source_document_id
            JOIN corpus_tokens t ON t.analysis_run_id = ds.analysis_run_id
              AND t.sentence_ordinal = ds.sentence_ordinal AND t.owner_id = ds.owner_id
              AND t.corpus_id = ds.corpus_id
              AND t.start_offset = dc.start_offset AND t.end_offset = dc.end_offset
           WHERE dc.owner_id = sqlc.arg('owner')::uuid AND dc.canonical_lemma = sqlc.arg('term')::text
             AND t.language = sqlc.arg('language')::text) c
    JOIN corpus_sentences s ON s.analysis_run_id = c.analysis_run_id
      AND s.sentence_ordinal = c.sentence_ordinal
      AND s.owner_id = sqlc.arg('owner')::uuid AND s.corpus_id = c.corpus_id
    JOIN current_analysis_identity ca ON ca.owner_id = s.owner_id
      AND ca.analysis_run_id = c.analysis_run_id AND ca.corpus_id = c.corpus_id
    JOIN books b ON b.owner_id = ca.owner_id AND b.id = ca.book_id
      AND b.language_state = 'chosen' AND b.language_tag = sqlc.arg('language')::text
    JOIN source_material_units u ON u.owner_id = ca.owner_id
      AND u.source_material_id = ca.source_material_id
      AND u.snapshot_id = ca.snapshot_id AND u.unit_id = s.unit_id
    LEFT JOIN occurrence_lemma_corrections d ON d.owner_id = ca.owner_id
      AND d.book_id = ca.book_id AND d.corpus_id = c.corpus_id
      AND d.analysis_run_id = c.analysis_run_id AND d.source_document_id = u.unit_id
      AND d.start_offset = c.start_offset AND d.end_offset = c.end_offset
   WHERE NOT COALESCE(d.excluded, false)
     AND COALESCE(d.canonical_lemma, c.canonical_lemma) = sqlc.arg('term')::text
)::boolean AS evidenced;
