-- Known vocabulary to Already-known Study progress (ADR 0136 §8, spec #2727,
-- ticket #2738). DATA ONLY: no table, column, constraint or index changes; the
-- table this writes to shipped in 000080 (#2730). Known vocabulary itself is
-- left untouched and is dropped by a later release (#2746), once this backfill
-- has been checked.
--
-- What it does. Every Known identity (owner, language, canonical lemma, UPOS,
-- lexical discriminator) that has no Study progress becomes Already-known
-- Study progress, as if the learner had answered Already known on its
-- introduction: state review, a scheduled interval of 21 days
-- (FluencyMaturityDays), so it is Mature at once, difficulty 5, one rep. The
-- discriminator is copied, so homographs (δέω 1, δέω 2) stay separate. Each
-- also gets one Already-known event with source 'backfill'. It is never a
-- grade, so study_review_log is not written.
--
-- First due dates. Per owner and language the identities are ordered by
-- (lemma, UPOS, discriminator) and the i-th of n is due (i-1)/n * 42 days after the
-- backfill time (the transaction's now()), so first reviews are spread evenly
-- and deterministically, with no random draw. last_review is due minus the 21
-- day interval, so the scheduling state stays consistent.
--
-- Study material. study_progress needs a material row and the word has no Study
-- material yet. The progress points at a superseded placeholder, an
-- 'unpreparable' record with the reason known_vocabulary_backfill, which is the
-- state a correction leaves a word in (ADR 0134 §6): the word's own material is
-- replaced, so Study prepares real material from a Corpus context
-- (enqueueReintroductions), moves the progress onto it and shows the word as
-- corrected. Until then the review is due with no card to show, and it never
-- grades against the placeholder. learned_meaning is a placeholder as well (the
-- lemma, the column may not be blank); the Gloss of the real material
-- replaces it.
--
-- No Corpus context. A word that no analysed Book in My Books of its language
-- evidences (no row in the Browse count projection of a current analysis) keeps
-- its progress and is withdrawn (withdrawn_at = the backfill time, no reason).
-- Study neither prepares nor shows a withdrawn word.
--
-- Skipped. A Known row with a blank language, lemma or UPOS (a lemma-wide
-- imported row, which cannot name a word) is not a Study word, and so is a row
-- whose identity already has Study progress: existing progress is never touched.
--
-- Ownership: Justin (the operator) runs it with the release and watches the
-- server's migration log; it applies at startup, in the release's one
-- migration pass. Rows are keyed by owner_id and never cross owners. Expected
-- impact: one INSERT per Known identity (three tables), a short write lock on
-- the study tables, no lock on Known vocabulary (read only). Check afterwards:
-- count(*) of study_already_known_event WHERE source = 'backfill' equals the
-- Known identities that had no progress.
--
-- Runtime consequences. Every migrated word that has a context is Reintroducing
-- at once, so the next Study preparation run queues one material job per word
-- (work that is spread over 42 days of reviews is done early). A withdrawn word
-- (no context) is re-checked on each Study page and returns by itself when the
-- Corpus gains a context: the placeholder has no source occurrence, so the
-- occurrence lookup falls back to a concordance search and the word is
-- re-introduced. A language with many withdrawn words makes that check slower.
-- Words due within 21 days of the backfill show the 'corrected' label after
-- re-introduction; later ones do not (last_review is after the backfill for
-- them, and is only used to make the interval consistent).
--
-- Idempotent and safe to retry, as below. Idempotent: the progress-existence guard makes a second run
-- select nothing, so it inserts and changes nothing. Retry-safe: it is one
-- statement in one transaction, so a failed run leaves no partial rows, and
-- re-running recovers. Recovery from a bad run: delete the rows with source
-- 'backfill' in study_already_known_event, their progress and placeholder
-- material (listed by the event), then re-run. The down migration does exactly
-- that. It is not assumed safe once a learner has reviewed a migrated word
-- (ADR 0038, ADR 0070): forward-only in production.
WITH backfill AS (
    SELECT now() AS at,
           ((now() AT TIME ZONE 'Europe/Berlin') - interval '5 hours')::date AS study_day
), todo AS (
    SELECT k.owner_id, k.language, k.canonical_lemma, k.upos, k.lexical_discriminator, k.headword,
           (row_number() OVER w - 1)::double precision / count(*) OVER (PARTITION BY k.owner_id, k.language) AS fraction,
           EXISTS (
               SELECT 1
                 FROM public.current_analysis_identity cai
                 JOIN public.books b ON b.owner_id = cai.owner_id AND b.id = cai.book_id
                  AND b.language_state = 'chosen' AND b.language_tag = k.language
                 JOIN public.book_membership m ON m.owner_id = cai.owner_id AND m.book_id = cai.book_id AND m.state = 'active'
                 JOIN public.vocabulary_browse_counts c ON c.owner_id = cai.owner_id AND c.book_id = cai.book_id
                  AND c.analysis_run_id = cai.analysis_run_id AND c.corpus_id = cai.corpus_id
                  AND c.language = k.language AND c.canonical_lemma = k.canonical_lemma
                  AND c.upos = k.upos AND c.lexical_discriminator = k.lexical_discriminator
                WHERE cai.owner_id = k.owner_id
           ) AS has_context
      FROM public.known_vocabulary k
     WHERE btrim(k.language) <> '' AND btrim(k.canonical_lemma) <> '' AND btrim(k.upos) <> ''
       AND NOT EXISTS (
           SELECT 1 FROM public.study_progress p
            WHERE p.owner_id = k.owner_id AND p.item_kind = 'word' AND p.language = k.language
              AND p.canonical_lemma = k.canonical_lemma AND p.upos = k.upos
              AND p.lexical_discriminator = k.lexical_discriminator)
    WINDOW w AS (PARTITION BY k.owner_id, k.language ORDER BY k.canonical_lemma, k.upos, k.lexical_discriminator)
), material AS (
    INSERT INTO public.study_material (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, state, omission_reason, superseded_at)
    SELECT t.owner_id, 'word', t.language, t.canonical_lemma, t.upos, t.lexical_discriminator, 'unpreparable', 'known_vocabulary_backfill', b.at
      FROM todo t CROSS JOIN backfill b
    RETURNING owner_id, id, language, canonical_lemma, upos, lexical_discriminator
), progress AS (
    INSERT INTO public.study_progress (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, material_id,
                                       introduced_at, learned_meaning, state, due, stability, difficulty, scheduled_days, reps, lapses,
                                       remaining_steps, last_review, withdrawn_at)
    SELECT t.owner_id, 'word', t.language, t.canonical_lemma, t.upos, t.lexical_discriminator, m.id,
           b.at, COALESCE(NULLIF(t.headword, ''), t.canonical_lemma), 'review',
           b.at + t.fraction * interval '42 days', 21, 5, 21, 1, 0,
           0, b.at + t.fraction * interval '42 days' - interval '21 days',
           CASE WHEN t.has_context THEN NULL ELSE b.at END
      FROM todo t CROSS JOIN backfill b
      JOIN material m ON m.owner_id = t.owner_id AND m.language = t.language AND m.canonical_lemma = t.canonical_lemma
       AND m.upos = t.upos AND m.lexical_discriminator = t.lexical_discriminator
    RETURNING owner_id, id, material_id, language, canonical_lemma, upos, lexical_discriminator
)
INSERT INTO public.study_already_known_event (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, progress_id, material_id, source, answered_at, study_day)
SELECT p.owner_id, 'word', p.language, p.canonical_lemma, p.upos, p.lexical_discriminator, p.id, p.material_id, 'backfill', b.at, b.study_day
  FROM progress p CROSS JOIN backfill b;
