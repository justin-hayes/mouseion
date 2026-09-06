-- This data-only convergence is intentionally irreversible: the discarded
-- region subtags and duplicate rows cannot be reconstructed. Recovery requires
-- restoring a backup taken before 000049. Keeping down as a no-op allows
-- migration tooling to move past this version without inventing data.
DO $$
BEGIN
    NULL;
END
$$;
