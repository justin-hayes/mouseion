-- Data-only backfill for ADR 0051. Ownership: application startup runs this
-- migration through golang-migrate. It is idempotent because every Goal is
-- assigned from its owner-scoped Book (or removed if that Book has no
-- language), and a retry repeats the same update.
-- Recovery: a failed migration transaction rolls back atomically and is safe
-- to retry. A Goal whose Book has no chosen language cannot belong to any
-- study-language Journey, so it is discarded rather than assigned a fake
-- language. Rollback removes language-partitioned rows before restoring the
-- temporary legacy marker; production rollback should use a database backup.
-- PostgreSQL holds row locks for the affected Goals while reading the matching
-- Books; the expected impact is one brief write lock per changed Goal. The
-- migration runner reports a non-zero failure and rolls back the transaction;
-- inspect the migration error, remediate the data, then retry. The next
-- migration owns the nonempty-language constraint; rollback is supported only
-- as the ordered 000057 -> 000056 -> 000055 sequence.
DELETE FROM primary_goals g
USING books b
WHERE b.owner_id = g.owner_id
  AND b.id = g.book_id
  AND (b.language_state <> 'chosen' OR btrim(COALESCE(b.language_tag, '')) = '');

UPDATE primary_goals g
SET language = b.language_tag
FROM books b
WHERE b.owner_id = g.owner_id
  AND b.id = g.book_id
  AND g.language = ''
  AND b.language_state = 'chosen'
  AND btrim(COALESCE(b.language_tag, '')) <> '';
