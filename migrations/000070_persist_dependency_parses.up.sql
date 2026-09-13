-- Dependency structure is required for every normalized corpus token. The
-- database is rolled forward by dropping it, so no legacy rows need a
-- backfill.
ALTER TABLE corpus_tokens
 ADD COLUMN dependency text NOT NULL CHECK (dependency <> ''),
 ADD COLUMN head bigint NOT NULL CHECK (head >= 0);

-- Dependency-aware concordance queries constrain the target relation first.
CREATE INDEX corpus_tokens_owner_language_dependency_idx
 ON corpus_tokens(owner_id, language, dependency);
