-- Keep the occurrence identity and structural context in one read model so
-- concordance queries only express their match and ordering rules.
CREATE VIEW concordance_occurrences AS
SELECT t.owner_id,
       t.language,
       t.surface,
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
       ca.book_id::text AS book_id,
       b.title AS book_title,
       ca.source_material_id::text AS source_material_id,
       ca.analysis_run_id::text AS analysis_run_id,
       ca.corpus_id::text AS corpus_id,
       u.unit_id,
       u.title AS chapter_title,
       u.unit_order,
       s.sentence_ordinal,
       t.token_ordinal,
       jm.position AS book_position,
       jm.created_at AS book_position_created_at
FROM corpus_tokens t
JOIN corpus_sentences s
  ON s.owner_id = t.owner_id
 AND s.analysis_run_id = t.analysis_run_id
 AND s.corpus_id = t.corpus_id
 AND s.sentence_ordinal = t.sentence_ordinal
LEFT JOIN corpus_tokens head
  ON head.owner_id = t.owner_id
 AND head.language = t.language
 AND head.analysis_run_id = t.analysis_run_id
 AND head.corpus_id = t.corpus_id
 AND head.sentence_ordinal = t.sentence_ordinal
 AND head.token_ordinal = t.head
JOIN current_analysis_identity ca
  ON ca.owner_id = t.owner_id
 AND ca.analysis_run_id = t.analysis_run_id
 AND ca.corpus_id = t.corpus_id
JOIN books b
  ON b.owner_id = ca.owner_id
 AND b.id = ca.book_id
 AND b.language_state = 'chosen'
 AND b.language_tag = t.language
JOIN source_material_units u
  ON u.owner_id = ca.owner_id
 AND u.source_material_id = ca.source_material_id
 AND u.snapshot_id = ca.snapshot_id
 AND u.unit_id = s.unit_id
LEFT JOIN reading_journey_membership jm
  ON jm.owner_id = ca.owner_id
 AND jm.language = t.language
 AND jm.book_id = ca.book_id;
