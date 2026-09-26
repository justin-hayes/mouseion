-- Ordered Journey membership has been replaced by book_dispositions. Preserve
-- the current concordance identity and ordering while removing its obsolete
-- optional Journey-position projection.
DROP VIEW public.concordance_occurrences;

CREATE VIEW public.concordance_occurrences AS
 SELECT t.owner_id,
    t.language,
    t.surface,
    t.canonical_lemma,
    t.upos,
    t.dependency,
    t.head AS head_ordinal,
    head.surface AS head_surface,
    s.sentence_text,
    (t.start_offset - s.start_offset) AS sentence_start_offset,
    (t.end_offset - s.start_offset) AS sentence_end_offset,
    t.start_offset AS unit_start_offset,
    t.end_offset AS unit_end_offset,
    (u.start_offset + t.start_offset) AS book_start_offset,
    (u.start_offset + t.end_offset) AS book_end_offset,
    (ca.book_id)::text AS book_id,
    b.title AS book_title,
    (ca.source_material_id)::text AS source_material_id,
    (ca.analysis_run_id)::text AS analysis_run_id,
    (ca.corpus_id)::text AS corpus_id,
    u.unit_id,
    u.title AS chapter_title,
    u.unit_order,
    s.sentence_ordinal,
    t.token_ordinal
   FROM (((((public.corpus_tokens t
     JOIN public.corpus_sentences s ON (((s.owner_id = t.owner_id) AND (s.analysis_run_id = t.analysis_run_id) AND (s.corpus_id = t.corpus_id) AND (s.sentence_ordinal = t.sentence_ordinal))))
     LEFT JOIN public.corpus_tokens head ON (((head.owner_id = t.owner_id) AND (head.language = t.language) AND (head.analysis_run_id = t.analysis_run_id) AND (head.corpus_id = t.corpus_id) AND (head.sentence_ordinal = t.sentence_ordinal) AND (head.token_ordinal = t.head))))
     JOIN public.current_analysis_identity ca ON (((ca.owner_id = t.owner_id) AND (ca.analysis_run_id = t.analysis_run_id) AND (ca.corpus_id = t.corpus_id))))
     JOIN public.books b ON (((b.owner_id = ca.owner_id) AND (b.id = ca.book_id) AND (b.language_state = 'chosen'::text) AND (b.language_tag = t.language))))
     JOIN public.source_material_units u ON (((u.owner_id = ca.owner_id) AND (u.source_material_id = ca.source_material_id) AND (u.snapshot_id = ca.snapshot_id) AND (u.unit_id = s.unit_id))));

-- The backfill from migration 16 is the durable To Read state. Do not rewrite
-- an existing learner-selected disposition during contraction.
INSERT INTO public.book_dispositions (owner_id, book_id, disposition)
SELECT owner_id, book_id, 'to_read'
FROM public.reading_journey_membership
ON CONFLICT (owner_id, book_id) DO NOTHING;

DROP TABLE public.reading_journey_membership;
DROP TABLE public.reading_journeys;
