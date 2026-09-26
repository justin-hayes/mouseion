-- Concordance occurrence queries read from the shared occurrence model.

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
