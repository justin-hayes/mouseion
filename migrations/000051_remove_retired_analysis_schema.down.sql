-- This migration deletes data and cannot be reversed by DDL. Restore a backup
-- taken before 000051 instead; do not attempt to reconstruct the deleted
-- manual books, plain-text sources, or reviewed-scope records with defaults.
DO $$
BEGIN
  RAISE EXCEPTION '000051 is destructive and has no safe down migration; restore a pre-000051 backup';
END $$;
