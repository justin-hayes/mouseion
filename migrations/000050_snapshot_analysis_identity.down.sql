-- 000050 changes the identity of persisted analysis runs and cannot be safely
-- reversed after snapshot runs exist. Restore a pre-000050 backup instead.
DO $$
BEGIN
  RAISE EXCEPTION '000050 is not safely reversible; restore a pre-000050 backup';
END $$;
