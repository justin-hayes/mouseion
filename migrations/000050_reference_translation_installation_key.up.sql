-- Reference translations become installation-wide (ADR 0116, ticket #2412).
--
-- Structural DDL only, so review can compare this file with the data movement
-- in 000051. The final table is keyed by the reviewed translation pin SHA-256
-- and citation, with no owner: one imported file is shared by every learner and
-- every Book of a Work. Different pins coexist, so a new translation revision
-- never invalidates an older prepared deck.
--
-- Ownership: the server owns this table; it contains no learner rows and is
-- populated lazily by classical translation import.
-- Idempotency: this file creates the new shape once. It is not replayable
-- against an installation that already has the new table; the migration
-- runner's version table is what prevents a second run. The row copy in 000051
-- is the part that can safely retry with its conflict target.
-- Retry safety: this file runs in its own migration transaction. If it fails,
-- it rolls back and leaves the owner-scoped table untouched for a retry.
-- Recovery: forward-only in production (ADR 0038, ADR 0070). Restore the
-- database from the pre-migration backup or re-import translations; the old
-- per-owner rows cannot be reconstructed from the installation-wide rows.
CREATE TABLE public.reference_translations_v2 (
    translation_pin_sha256 text NOT NULL,
    language text NOT NULL,
    citation text NOT NULL,
    translation text NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT reference_translations_v2_pin_check CHECK ((translation_pin_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT reference_translations_v2_citation_check CHECK ((citation <> ''::text)),
    CONSTRAINT reference_translations_v2_language_check CHECK ((language <> ''::text)),
    CONSTRAINT reference_translations_v2_translation_check CHECK ((translation <> ''::text)),
    CONSTRAINT reference_translations_v2_pkey PRIMARY KEY (translation_pin_sha256, citation)
);
