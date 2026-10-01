-- Concordance occurrence queries read from the shared occurrence model.

-- name: GetVocabularySentenceStudy :many
SELECT COALESCE(t.surface, '')::text AS surface,
       COALESCE(t.raw_lemma, '')::text AS raw_lemma,
       COALESCE(t.upos, '')::text AS upos,
       COALESCE(t.dependency, '')::text AS dependency,
       COALESCE(t.head, 0)::bigint AS head,
       COALESCE(h.surface, '')::text AS head_surface,
       COALESCE(t.token_ordinal, -1)::bigint AS token_ordinal,
       COALESCE(d.canonical_lemma, t.canonical_lemma, '')::text AS effective_lemma,
       (d.canonical_lemma IS NOT NULL AND NOT COALESCE(d.excluded, false))::boolean AS corrected,
       COALESCE(d.excluded, false)::boolean AS excluded,
       s.sentence_text,
       s.sentence_ordinal,
       sqlc.arg('target_ordinal')::bigint AS target_ordinal,
       ca.book_id::text AS book_id,
       b.title AS book_title,
       u.title AS chapter_title
  FROM current_analysis_identity ca
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
  LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=ca.owner_id AND d.book_id=ca.book_id
       AND d.corpus_id=ca.corpus_id AND d.analysis_run_id=ca.analysis_run_id
       AND d.source_document_id=s.unit_id AND d.start_offset=t.start_offset AND d.end_offset=t.end_offset
 WHERE ca.owner_id=sqlc.arg('owner') AND ca.book_id=sqlc.arg('book')
   AND ca.analysis_run_id=sqlc.arg('analysis_run') AND ca.corpus_id=sqlc.arg('corpus')
   AND s.sentence_ordinal=sqlc.arg('sentence_ordinal')
 ORDER BY t.token_ordinal;

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
       t.raw_lemma,
       COALESCE(d.canonical_lemma, o.canonical_lemma)::text AS effective_lemma,
       (d.canonical_lemma IS NOT NULL AND NOT COALESCE(d.excluded, false))::boolean AS corrected,
       COALESCE(d.excluded, false)::boolean AS excluded
  FROM concordance_occurrences o
  JOIN corpus_tokens t ON t.owner_id=o.owner_id AND t.language=o.language
    AND t.analysis_run_id::text=o.analysis_run_id AND t.corpus_id::text=o.corpus_id
    AND t.sentence_ordinal=o.sentence_ordinal AND t.token_ordinal=o.token_ordinal
  LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=o.owner_id
    AND d.book_id::text=o.book_id AND d.corpus_id::text=o.corpus_id
    AND d.analysis_run_id::text=o.analysis_run_id AND d.source_document_id=o.unit_id
    AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
 WHERE o.owner_id=sqlc.arg('owner') AND o.language=sqlc.arg('language')
   AND (sqlc.arg('grammar_direction')::text='governor' OR sqlc.arg('mode')::text <> 'surface' OR o.surface=sqlc.arg('term'))
   AND (sqlc.arg('grammar_direction')::text='governor' OR sqlc.arg('mode')::text <> 'effective' OR
        (NOT COALESCE(d.excluded, false)
         AND COALESCE(d.canonical_lemma, o.canonical_lemma)=sqlc.arg('term')
         AND o.upos=sqlc.arg('upos')))
   AND (sqlc.arg('grammar_direction')::text='governor' OR sqlc.arg('mode')::text <> 'analyzer' OR
        (t.raw_lemma=sqlc.arg('term') AND o.upos=sqlc.arg('upos')))
   AND (sqlc.arg('book_ids')::text = '' OR o.book_id::text = ANY(string_to_array(sqlc.arg('book_ids'), ',')))
   AND (sqlc.arg('grammar_direction')::text = '' OR
        (sqlc.arg('grammar_direction')::text = 'own' AND o.dependency=sqlc.arg('relation')) OR
        (sqlc.arg('grammar_direction')::text = 'governor' AND o.dependency=sqlc.arg('relation') AND EXISTS (
          SELECT 1
            FROM corpus_tokens g
            LEFT JOIN occurrence_lemma_corrections gd ON gd.owner_id=o.owner_id
              AND gd.book_id::text=o.book_id AND gd.corpus_id::text=g.corpus_id::text
              AND gd.analysis_run_id::text=g.analysis_run_id::text
              AND gd.source_document_id=o.unit_id
              AND gd.start_offset=g.start_offset AND gd.end_offset=g.end_offset
           WHERE g.owner_id=o.owner_id AND g.language=o.language
             AND g.analysis_run_id::text=o.analysis_run_id AND g.corpus_id::text=o.corpus_id
             AND g.sentence_ordinal=o.sentence_ordinal AND g.token_ordinal=o.head_ordinal
             AND NOT COALESCE(gd.excluded, false)
             AND ((sqlc.arg('mode')::text='effective'
                   AND COALESCE(gd.canonical_lemma, g.canonical_lemma)=sqlc.arg('term')
                   AND g.upos=sqlc.arg('upos'))
               OR (sqlc.arg('mode')::text='analyzer' AND g.raw_lemma=sqlc.arg('term') AND g.upos=sqlc.arg('upos'))
               OR (sqlc.arg('mode')::text='surface' AND g.surface=sqlc.arg('term')))
        )))
 ORDER BY lower(o.book_title), o.book_title, o.book_id,
          o.unit_order, o.sentence_ordinal, o.token_ordinal
 LIMIT 26 OFFSET sqlc.arg('offset')::bigint;

-- name: ListBookOccurrencesByLemma :many
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
       o.token_ordinal
 FROM concordance_occurrences o
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND o.canonical_lemma = sqlc.arg('canonical_lemma')
   AND o.upos = sqlc.arg('upos')
   AND o.book_id = sqlc.arg('book')
 ORDER BY o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListBookOccurrencesBySurface :many
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
       o.token_ordinal
 FROM concordance_occurrences o
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND o.surface = sqlc.arg('surface')
   AND o.book_id = sqlc.arg('book')
 ORDER BY o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListBookOccurrencesByLemmaAndDependency :many
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
       o.token_ordinal
  FROM concordance_occurrences o
  WHERE o.owner_id = sqlc.arg('owner')
    AND o.language = sqlc.arg('language')
    AND o.canonical_lemma = sqlc.arg('canonical_lemma')
    AND o.upos = sqlc.arg('upos')
    AND o.dependency = sqlc.arg('dependency')
    AND o.book_id = sqlc.arg('book')
  ORDER BY o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListBookOccurrencesBySurfaceAndDependency :many
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
       o.token_ordinal
  FROM concordance_occurrences o
  WHERE o.owner_id = sqlc.arg('owner')
    AND o.language = sqlc.arg('language')
    AND o.surface = sqlc.arg('surface')
    AND o.dependency = sqlc.arg('dependency')
    AND o.book_id = sqlc.arg('book')
  ORDER BY o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListStudyLanguageOccurrencesByLemma :many
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
       o.token_ordinal
 FROM concordance_occurrences o
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND o.canonical_lemma = sqlc.arg('canonical_lemma')
   AND o.upos = sqlc.arg('upos')
 ORDER BY lower(o.book_title), o.book_title, o.book_id,
          o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListBookDependentsByGovernorLemma :many
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
       o.token_ordinal
  FROM concordance_occurrences o
  JOIN corpus_tokens governor
    ON governor.owner_id = o.owner_id
   AND governor.language = o.language
   AND governor.analysis_run_id::text = o.analysis_run_id
   AND governor.corpus_id::text = o.corpus_id
   AND governor.sentence_ordinal = o.sentence_ordinal
   AND governor.token_ordinal = o.head_ordinal
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND o.book_id = sqlc.arg('book')
   AND governor.canonical_lemma = sqlc.arg('governor_canonical_lemma')
   AND governor.upos = sqlc.arg('governor_upos')
   AND o.dependency = sqlc.arg('dependency')
 ORDER BY o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListStudyLanguageDependentsByGovernorLemma :many
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
       o.token_ordinal
  FROM concordance_occurrences o
  JOIN corpus_tokens governor
    ON governor.owner_id = o.owner_id
   AND governor.language = o.language
   AND governor.analysis_run_id::text = o.analysis_run_id
   AND governor.corpus_id::text = o.corpus_id
   AND governor.sentence_ordinal = o.sentence_ordinal
   AND governor.token_ordinal = o.head_ordinal
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND governor.canonical_lemma = sqlc.arg('governor_canonical_lemma')
   AND governor.upos = sqlc.arg('governor_upos')
   AND o.dependency = sqlc.arg('dependency')
 ORDER BY lower(o.book_title), o.book_title, o.book_id,
          o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListStudyLanguageOccurrencesBySurface :many
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
       o.token_ordinal
 FROM concordance_occurrences o
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND o.surface = sqlc.arg('surface')
 ORDER BY lower(o.book_title), o.book_title, o.book_id,
          o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListStudyLanguageOccurrencesByLemmaAndDependency :many
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
       o.token_ordinal
  FROM concordance_occurrences o
  WHERE o.owner_id = sqlc.arg('owner')
    AND o.language = sqlc.arg('language')
    AND o.canonical_lemma = sqlc.arg('canonical_lemma')
    AND o.upos = sqlc.arg('upos')
    AND o.dependency = sqlc.arg('dependency')
  ORDER BY lower(o.book_title), o.book_title, o.book_id,
           o.unit_order, o.sentence_ordinal, o.token_ordinal;

-- name: ListStudyLanguageOccurrencesBySurfaceAndDependency :many
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
       o.token_ordinal
  FROM concordance_occurrences o
  WHERE o.owner_id = sqlc.arg('owner')
    AND o.language = sqlc.arg('language')
    AND o.surface = sqlc.arg('surface')
    AND o.dependency = sqlc.arg('dependency')
  ORDER BY lower(o.book_title), o.book_title, o.book_id,
           o.unit_order, o.sentence_ordinal, o.token_ordinal;
