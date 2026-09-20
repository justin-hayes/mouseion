-- Canonicalize legacy Known state, then give every active Goal with trustworthy
-- current analysis one immutable vocabulary snapshot. An exact active legacy
-- preparation is bound first; that binding is the sole classification used by
-- vocabulary adoption, candidate fallback, and unmatched-study release. Empty
-- active preparations are therefore ordinary bound snapshots with zero rows.
--
-- Ownership and observation belong to the release operator. The migration is
-- idempotent and runs once under the migration runner's advisory lock. It may
-- scan vocabulary state, Goals, preparations, and selection candidates while
-- updating matching rows in one migration transaction. Expected impact is one
-- canonical Known row per legacy Known identity, one snapshot per eligible
-- active Goal, exact preservation of matching active preparations, and release
-- of unmatched active preparations without deleting artifacts or provenance.
-- Failure is a non-zero migration result or a post-migration count outside
-- those expectations. Recover by restoring the pre-migration backup and
-- rerunning after correction; the down migration is intentionally non-
-- destructive and is not a production rollback.
INSERT INTO public.known_vocabulary(owner_id, language, canonical_lemma, upos, created_at)
SELECT owner_id, language, canonical_lemma, upos, updated_at
FROM public.vocabulary_states
WHERE state = 'known'
ON CONFLICT(owner_id, language, canonical_lemma, upos) DO NOTHING;

WITH matching_goals AS (
    SELECT DISTINCT ON (g.owner_id, g.language)
           g.owner_id, g.language, g.book_id,
           ca.source_material_id, ca.analysis_run_id, ca.content_revision_id,
           ca.snapshot_id AS content_snapshot_id, ca.corpus_id
    FROM public.primary_goals g
    JOIN public.current_analysis_identity ca
      ON ca.owner_id = g.owner_id AND ca.book_id = g.book_id
    JOIN public.source_materials sm
      ON sm.owner_id = ca.owner_id AND sm.id = ca.source_material_id
     AND sm.language = g.language
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
    RETURNING owner_id, id, language, book_id
)
UPDATE public.primary_goals g
SET snapshot_id = inserted.id, updated_at = now()
FROM inserted
WHERE g.owner_id = inserted.owner_id
  AND g.language = inserted.language
  AND g.book_id = inserted.book_id;

-- Bind exact active legacy preparations before deriving snapshot vocabulary.
-- Every later statement trusts this persisted relationship rather than
-- repeating a competing definition of "matching".
UPDATE public.deck_preparations p
SET goal_snapshot_id = g.snapshot_id, updated_at = now()
FROM public.primary_goals g
JOIN public.primary_goal_snapshots ps
  ON ps.owner_id = g.owner_id AND ps.id = g.snapshot_id
JOIN public.source_materials sm
  ON sm.owner_id = ps.owner_id AND sm.id = ps.source_material_id
WHERE p.goal_snapshot_id IS NULL
  AND p.owner_id = g.owner_id
  AND p.book_id = g.book_id
  AND p.source_material_id = ps.source_material_id
  AND p.analysis_run_id = ps.analysis_run_id
  AND sm.language = g.language
  AND p.studying_at IS NOT NULL
  AND p.released_at IS NULL
  AND p.retired_at IS NULL
  AND p.graduated_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM public.deck_preparation_vocabulary dv
      WHERE dv.owner_id = p.owner_id
        AND dv.deck_preparation_id = p.id
        AND dv.graduated_at IS NULL
        AND dv.language <> g.language
  );

-- A bound non-empty preparation contributes its exact historical identities.
-- A bound empty preparation naturally contributes no rows and still prevents
-- current candidates from widening its frozen snapshot.
INSERT INTO public.primary_goal_snapshot_vocabulary(
    owner_id, snapshot_id, corpus_id, language, canonical_lemma, upos,
    occurrence_count, observed_forms, eligible_sentence_refs, provenance, first_encounter, selected_at
)
SELECT DISTINCT ON (p.owner_id, p.goal_snapshot_id, dv.language, dv.canonical_lemma, dv.upos)
       p.owner_id, p.goal_snapshot_id, COALESCE(sc.corpus_id, ps.corpus_id::text), dv.language,
       dv.canonical_lemma, dv.upos, COALESCE(sc.occurrence_count, 1),
       COALESCE(sc.observed_forms, '{}'::jsonb), COALESCE(sc.eligible_sentence_refs, '[]'::jsonb),
       COALESCE(sc.provenance, '{}'::jsonb),
       COALESCE(first_seen.start_offset, 9223372036854775807::bigint),
       COALESCE(sc.selected_at, dv.generated_at, now())
FROM public.deck_preparations p
JOIN public.primary_goal_snapshots ps
  ON ps.owner_id = p.owner_id AND ps.id = p.goal_snapshot_id
JOIN public.deck_preparation_vocabulary dv
  ON dv.owner_id = p.owner_id AND dv.deck_preparation_id = p.id
 AND dv.language = ps.language AND dv.graduated_at IS NULL
LEFT JOIN public.selection_candidates sc
  ON sc.owner_id = p.owner_id AND sc.corpus_id = ps.corpus_id::text
 AND sc.language = dv.language AND sc.canonical_lemma = dv.canonical_lemma AND sc.upos = dv.upos
LEFT JOIN LATERAL (
  SELECT MIN(COALESCE(ref->'location'->>'start_offset', ref->'location'->>'StartOffset', ref->'Location'->>'StartOffset')::bigint) AS start_offset
  FROM jsonb_array_elements(sc.eligible_sentence_refs) ref
) first_seen ON true
WHERE NOT EXISTS (
    SELECT 1 FROM public.primary_goal_snapshot_vocabulary existing
    WHERE existing.owner_id = p.owner_id AND existing.snapshot_id = p.goal_snapshot_id
      AND existing.language = dv.language
      AND existing.canonical_lemma = dv.canonical_lemma
      AND existing.upos = dv.upos
)
ORDER BY p.owner_id, p.goal_snapshot_id, dv.language, dv.canonical_lemma, dv.upos, p.created_at DESC;

-- Goals without an exact active legacy preparation freeze current eligible
-- candidates. Inactive historical preparations do not affect current intent.
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
    WHERE p.owner_id = ps.owner_id AND p.goal_snapshot_id = ps.id
  )
  AND sc.occurrence_count >= 3
  AND NOT EXISTS (
    SELECT 1 FROM public.known_vocabulary kv
    WHERE kv.owner_id = sc.owner_id AND kv.language = sc.language
      AND kv.canonical_lemma = sc.canonical_lemma
      AND (kv.upos = sc.upos OR kv.upos = '')
  );

-- An active preparation that was not bound by the exact match above is not
-- current learner intent. Release it while retaining its artifact and history.
UPDATE public.deck_preparations p
SET studying_at = NULL, released_at = now(), updated_at = now()
WHERE p.studying_at IS NOT NULL
  AND p.released_at IS NULL
  AND p.goal_snapshot_id IS NULL
  AND p.graduated_at IS NULL
  AND p.retired_at IS NULL;
