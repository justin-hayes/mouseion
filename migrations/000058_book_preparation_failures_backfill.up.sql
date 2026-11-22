-- Data-only backfill for the one Book preparation failure record (ADR 0121).
-- Deliberately separate from migration 57's structural DDL (ADR 0038, ADR 0070).
--
-- Ownership is with the release operator. Expected impact is one
-- book_preparation_failures row per existing Classical preparation failure and
-- per existing EPUB acquisition failure, with stage, code, failed_at and every
-- provenance value copied unchanged. The primary key and ON CONFLICT DO NOTHING
-- make this idempotent: a retry, or a second application after the application
-- has already written newer evidence, never overwrites a row. The migration
-- runner holds its database advisory lock and executes this file as one
-- transaction, so a failure rolls the whole copy back. Recovery is to retry the
-- migration; rollback is to read the old tables again, which stay in place until
-- the cleanup migration, or to restore the database backup because the copied
-- evidence is intentionally retained.
INSERT INTO public.book_preparation_failures(
    owner_id, book_id, reading_unit_key, definition_revision,
    connection_id, entry_identifier, stage, code, failed_at)
SELECT owner_id, book_id, reading_unit_key, definition_revision,
       NULL::uuid, NULL::text, stage, code, failed_at
FROM public.classical_preparation_failures
ON CONFLICT (owner_id, book_id) DO NOTHING;

INSERT INTO public.book_preparation_failures(
    owner_id, book_id, reading_unit_key, definition_revision,
    connection_id, entry_identifier, stage, code, failed_at)
SELECT owner_id, book_id, NULL::text, NULL::integer,
       connection_id, entry_identifier, stage, code, failed_at
FROM public.epub_acquisition_failures
ON CONFLICT (owner_id, book_id) DO NOTHING;
