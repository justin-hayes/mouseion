-- Copy the owner-scoped reference translations into the installation-wide key
-- introduced by 000050. Data movement only; the structural DDL is reviewed in
-- the previous migration.
--
-- Ownership: an installation-wide row is shared reference data. It belongs to
-- the deployment, not to any learner, and the old owner column is dropped.
-- Idempotency: DISTINCT ON keeps exactly one row per (pin SHA-256, citation).
-- Identical rows imported for several learners collapse to the newest one;
-- different pins for the same citation stay separate. The insert names the new
-- primary key as its conflict target and does nothing on a conflict, so a
-- retried copy cannot duplicate a row.
-- Retry safety: each migration file runs in its own migration transaction. If
-- this file fails, the copy, drop and rename roll back together, leaving the
-- installation at version 50 with the old owner-scoped table intact; the next
-- run retries this file. If it succeeds, version 51 is recorded.
-- Recovery: forward-only in production (ADR 0038, ADR 0070). The down
-- migration can restore the empty old shape for development, but per-learner
-- ownership is not recoverable from the installation-wide rows; restore a
-- pre-migration backup or re-import the pinned translation instead.

-- A legacy pin is the reviewed "sha256:<hex>" form. The raw 64-hex form is
-- also accepted so a row written directly by an operator tool is not lost. A
-- row whose pin is not a SHA-256 cannot satisfy the new key's check, so it is
-- left behind rather than silently trying to become a different pin.
INSERT INTO public.reference_translations_v2(
    translation_pin_sha256, language, citation, translation, updated_at)
SELECT DISTINCT ON (CASE
        WHEN source_pin ~ '^sha256:[0-9a-f]{64}$' THEN substring(source_pin FROM 8)
        ELSE source_pin
    END, citation)
    CASE
        WHEN source_pin ~ '^sha256:[0-9a-f]{64}$' THEN substring(source_pin FROM 8)
        ELSE source_pin
    END,
    language,
    citation,
    translation,
    updated_at
FROM public.reference_translations
WHERE source_pin ~ '^(sha256:)?[0-9a-f]{64}$'
ORDER BY CASE
        WHEN source_pin ~ '^sha256:[0-9a-f]{64}$' THEN substring(source_pin FROM 8)
        ELSE source_pin
    END, citation, updated_at DESC
ON CONFLICT (translation_pin_sha256, citation) DO NOTHING;

DROP TABLE public.reference_translations;

ALTER TABLE public.reference_translations_v2 RENAME TO reference_translations;
