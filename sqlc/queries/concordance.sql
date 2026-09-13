-- Concordance occurrence queries read from the shared occurrence model. Book
-- position is the learner's Reading Journey position; analyzed Books outside
-- the Journey remain in study-language results with no position.

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
       o.token_ordinal,
       o.book_position
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
       o.token_ordinal,
       o.book_position
 FROM concordance_occurrences o
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND o.surface = sqlc.arg('surface')
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
       o.token_ordinal,
       o.book_position
 FROM concordance_occurrences o
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND o.canonical_lemma = sqlc.arg('canonical_lemma')
   AND o.upos = sqlc.arg('upos')
 ORDER BY o.book_position NULLS LAST,
          o.book_position_created_at NULLS LAST,
          lower(o.book_title), o.book_title, o.book_id,
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
       o.token_ordinal,
       o.book_position
 FROM concordance_occurrences o
 WHERE o.owner_id = sqlc.arg('owner')
   AND o.language = sqlc.arg('language')
   AND o.surface = sqlc.arg('surface')
 ORDER BY o.book_position NULLS LAST,
          o.book_position_created_at NULLS LAST,
          lower(o.book_title), o.book_title, o.book_id,
          o.unit_order, o.sentence_ordinal, o.token_ordinal;
