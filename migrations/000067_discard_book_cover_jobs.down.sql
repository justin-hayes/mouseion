-- Development convenience only (ADR 0070): discarded queue rows are not
-- recreated; the retired per-Book retrieval worker could not run them.
SELECT 1;
