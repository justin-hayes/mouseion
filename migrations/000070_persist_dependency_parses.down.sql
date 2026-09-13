DROP INDEX IF EXISTS corpus_tokens_owner_language_dependency_idx;

ALTER TABLE corpus_tokens
 DROP COLUMN IF EXISTS dependency,
 DROP COLUMN IF EXISTS head;
