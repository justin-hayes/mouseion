-- Finish no longer freezes a vocabulary snapshot (ADR 0136 §4, #2740), so a
-- completion carries no goal_snapshot_id. The partial unique index below gave a
-- snapshot-less completion a durable identity per (owner, language, book),
-- which made a Book's second Finish a duplicate. Reading history is append-only
-- and every Finish is a new row, so the index is relaxed away. Nothing is
-- dropped from the table: the goal_snapshot_id column and every row stay.
--
-- Structural DDL only; no data is backfilled. Replaying the migration is safe
-- because of IF EXISTS.
DROP INDEX IF EXISTS public.reading_history_null_snapshot_identity_idx;
