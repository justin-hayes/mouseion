-- Development convenience only. Production applies the baseline Up migration
-- and recreates databases for the ADR 0070 cutover; down migrations are not
-- assumed safe for production rollback.
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
-- Keep golang-migrate's bookkeeping table available so its Down operation can
-- record the nil version after this teardown completes.
CREATE TABLE public.schema_migrations (
    version bigint NOT NULL PRIMARY KEY,
    dirty boolean NOT NULL
);
