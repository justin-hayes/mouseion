-- Development convenience only. Production applies the baseline Up migration
-- and recreates databases for the ADR 0070 cutover; down migrations are not
-- assumed safe for production rollback.
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
