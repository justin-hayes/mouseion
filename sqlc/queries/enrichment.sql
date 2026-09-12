-- Legacy enrichment reads used by the immediate card-export path. Exact
-- prepared-deck enrichment is resolved separately from its manifest keys.

-- name: GetLegacyEnrichmentForSentence :one
SELECT translation, sentence_translation, sentence_translation_target
FROM enrichment_cache
WHERE language = sqlc.arg('language')
  AND target_language = 'en'
  AND canonical_lemma = sqlc.arg('canonical_lemma')
  AND upos = upper(sqlc.arg('upos'))
  AND sentence_hash = sqlc.arg('sentence_hash')
ORDER BY cached_at DESC
LIMIT 1;
