-- Adopt a legacy active deck study only when its owner, language, Book,
-- analysis run, source, and current-analysis provenance all agree with the
-- active Goal. The operation is idempotent: released snapshots are retained,
-- and the Goal is only populated when it has no snapshot. Unmatched studies
-- are released without graduation; their deck and generated-vocabulary
-- history remain intact. The migration runner executes this transactionally,
-- so retrying the migration is the recovery path after failure.
-- Ownership and observation belong to the release operator. Observe migration
-- completion, snapshot counts, and released legacy-study counts in the
-- migration log before enabling new Goal selection. Expected impact is one
-- snapshot header per active Goal with trustworthy current analysis, one row
-- per adopted or eligible identity, and release timestamps on only unmatched
-- active studies. Failure detection is a non-zero migration result or any
-- post-migration count that violates those expectations; rollback/recovery is
-- restoring the pre-migration backup and rerunning after correction, not
-- manually editing partially migrated rows.
WITH matching_goals AS (
    SELECT DISTINCT ON (g.owner_id, g.language)
           g.owner_id, g.language, g.book_id,
           ca.source_material_id, ca.analysis_run_id, ca.content_revision_id,
           ca.snapshot_id AS content_snapshot_id, ca.corpus_id
    FROM public.primary_goals g
    JOIN public.current_analysis_identity ca
      ON ca.owner_id = g.owner_id AND ca.book_id = g.book_id
    WHERE g.snapshot_id IS NULL
    ORDER BY g.owner_id, g.language
), inserted AS (
    INSERT INTO public.primary_goal_snapshots(
        owner_id, language, book_id, source_material_id, analysis_run_id,
        content_revision_id, content_snapshot_id, corpus_id
    )
    SELECT owner_id, language, book_id, source_material_id, analysis_run_id,
           content_revision_id, content_snapshot_id, corpus_id
    FROM matching_goals
    RETURNING owner_id, id, language, book_id, source_material_id, analysis_run_id, corpus_id
)
UPDATE public.primary_goals g
SET snapshot_id = inserted.id, updated_at = now()
FROM inserted
WHERE g.owner_id = inserted.owner_id
  AND g.language = inserted.language
  AND g.book_id = inserted.book_id;

INSERT INTO public.primary_goal_snapshot_vocabulary(
    owner_id, snapshot_id, corpus_id, language, canonical_lemma, upos,
    occurrence_count, observed_forms, eligible_sentence_refs, provenance, first_encounter, selected_at
)
SELECT DISTINCT ON (p.owner_id, g.snapshot_id, dv.language, dv.canonical_lemma, dv.upos)
       p.owner_id, g.snapshot_id, COALESCE(sc.corpus_id, ca.corpus_id::text), dv.language,
       dv.canonical_lemma, dv.upos, COALESCE(sc.occurrence_count, 1),
       COALESCE(sc.observed_forms, '{}'::jsonb), COALESCE(sc.eligible_sentence_refs, '[]'::jsonb),
       COALESCE(sc.provenance, '{}'::jsonb),
       COALESCE(first_seen.start_offset, 9223372036854775807::bigint),
       COALESCE(sc.selected_at, dv.generated_at, now())
FROM public.primary_goals g
JOIN public.current_analysis_identity ca
  ON ca.owner_id = g.owner_id AND ca.book_id = g.book_id
JOIN public.deck_preparations p
  ON p.owner_id = g.owner_id
 AND p.book_id = g.book_id
 AND p.source_material_id = ca.source_material_id
 AND p.analysis_run_id = ca.analysis_run_id
 AND p.studying_at IS NOT NULL
 AND p.released_at IS NULL
 AND p.retired_at IS NULL
 AND p.graduated_at IS NULL
JOIN public.deck_preparation_vocabulary dv
  ON dv.owner_id = p.owner_id AND dv.deck_preparation_id = p.id
 AND dv.language = g.language AND dv.graduated_at IS NULL
LEFT JOIN public.selection_candidates sc
  ON sc.owner_id = p.owner_id AND sc.corpus_id = ca.corpus_id::text
 AND sc.language = dv.language AND sc.canonical_lemma = dv.canonical_lemma AND sc.upos = dv.upos
LEFT JOIN LATERAL (
  SELECT MIN(COALESCE(ref->'location'->>'start_offset', ref->'location'->>'StartOffset', ref->'Location'->>'StartOffset')::bigint) AS start_offset
  FROM jsonb_array_elements(sc.eligible_sentence_refs) ref
) first_seen ON true
WHERE g.snapshot_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM public.primary_goal_snapshot_vocabulary existing
      WHERE existing.owner_id = g.owner_id AND existing.snapshot_id = g.snapshot_id
        AND existing.language = dv.language
        AND existing.canonical_lemma = dv.canonical_lemma
        AND existing.upos = dv.upos
  )
ORDER BY p.owner_id, g.snapshot_id, dv.language, dv.canonical_lemma, dv.upos, p.created_at DESC;

-- Bind adopted legacy preparations to the same frozen snapshot. This is what
-- makes retries and rerenders reuse the migrated identities instead of
-- selecting from the current candidate pool.
UPDATE public.deck_preparations p
SET goal_snapshot_id = g.snapshot_id, updated_at = now()
FROM public.primary_goals g
JOIN public.current_analysis_identity ca
  ON ca.owner_id = g.owner_id AND ca.book_id = g.book_id
JOIN public.source_materials sm
  ON sm.owner_id = g.owner_id AND sm.id = ca.source_material_id
WHERE p.goal_snapshot_id IS NULL
  AND p.owner_id = g.owner_id
  AND p.book_id = g.book_id
  AND p.source_material_id = ca.source_material_id
  AND p.analysis_run_id = ca.analysis_run_id
  AND sm.language = g.language
  AND p.studying_at IS NOT NULL
  AND p.released_at IS NULL
  AND p.retired_at IS NULL
  AND p.graduated_at IS NULL
  AND g.snapshot_id IS NOT NULL
  AND (
      SELECT count(*)
      FROM public.primary_goals g2
      WHERE g2.owner_id = g.owner_id
        AND g2.book_id = g.book_id
        AND g2.snapshot_id IS NOT NULL
  ) = 1
  AND NOT EXISTS (
      SELECT 1
      FROM public.deck_preparation_vocabulary dv
      WHERE dv.owner_id = p.owner_id
        AND dv.deck_preparation_id = p.id
        AND dv.graduated_at IS NULL
        AND dv.language <> g.language
  );

-- Active Goals without a matching legacy study still receive the current
-- recurring-vocabulary snapshot. A matching legacy study intentionally keeps
-- its exact historical vocabulary instead of silently widening the reservation.
INSERT INTO public.primary_goal_snapshot_vocabulary(
    owner_id, snapshot_id, corpus_id, language, canonical_lemma, upos,
    occurrence_count, observed_forms, eligible_sentence_refs, provenance, first_encounter, selected_at
)
SELECT sc.owner_id, g.snapshot_id, sc.corpus_id, sc.language, sc.canonical_lemma, sc.upos,
       sc.occurrence_count, sc.observed_forms, sc.eligible_sentence_refs, sc.provenance,
       COALESCE(first_seen.start_offset, 9223372036854775807::bigint), sc.selected_at
FROM public.primary_goals g
JOIN public.primary_goal_snapshots ps
  ON ps.owner_id = g.owner_id AND ps.id = g.snapshot_id AND ps.released_at IS NULL
JOIN public.selection_candidates sc
  ON sc.owner_id = ps.owner_id AND sc.corpus_id = ps.corpus_id::text AND sc.language = g.language
LEFT JOIN LATERAL (
  SELECT MIN(COALESCE(ref->'location'->>'start_offset', ref->'location'->>'StartOffset', ref->'Location'->>'StartOffset')::bigint) AS start_offset
  FROM jsonb_array_elements(sc.eligible_sentence_refs) ref
) first_seen ON true
WHERE NOT EXISTS (
    SELECT 1 FROM public.primary_goal_snapshot_vocabulary existing
    WHERE existing.owner_id = g.owner_id AND existing.snapshot_id = g.snapshot_id
)
   AND NOT EXISTS (
     SELECT 1
     FROM public.deck_preparations p
     WHERE p.owner_id = g.owner_id AND p.book_id = g.book_id
       AND p.source_material_id = ps.source_material_id
       AND p.retired_at IS NULL AND p.graduated_at IS NULL
 )
  AND sc.occurrence_count >= 3
  AND NOT EXISTS (
    SELECT 1 FROM public.known_vocabulary kv
    WHERE kv.owner_id = sc.owner_id AND kv.language = sc.language
      AND kv.canonical_lemma = sc.canonical_lemma
      AND (kv.upos = sc.upos OR kv.upos = '')
)
  AND NOT EXISTS (
    SELECT 1 FROM public.vocabulary_states vs
    WHERE vs.owner_id = sc.owner_id AND vs.language = sc.language
      AND vs.canonical_lemma = sc.canonical_lemma AND vs.upos = sc.upos
      AND vs.state = 'known'
);

UPDATE public.deck_preparations p
SET studying_at = NULL, released_at = COALESCE(released_at, now()), updated_at = now()
WHERE p.studying_at IS NOT NULL
  AND p.goal_snapshot_id IS NULL
  AND p.graduated_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM public.primary_goals g
      JOIN public.current_analysis_identity ca
        ON ca.owner_id = g.owner_id AND ca.book_id = g.book_id
      WHERE g.owner_id = p.owner_id
        AND g.book_id = p.book_id
        AND p.source_material_id = ca.source_material_id
        AND p.analysis_run_id = ca.analysis_run_id
        AND g.snapshot_id IS NOT NULL
        AND EXISTS (
            SELECT 1 FROM public.deck_preparation_vocabulary dv
            WHERE dv.owner_id = p.owner_id AND dv.deck_preparation_id = p.id
              AND dv.language = g.language AND dv.graduated_at IS NULL
        )
  );
