-- Legacy enrichment reads used by the immediate card-export path. Exact
-- prepared-deck enrichment is resolved separately from its manifest keys.

-- name: GetEnrichmentCache :one
SELECT translation, fallback_gloss, sense_selection, sentence_translation, sentence_translation_target, cached_at
FROM enrichment_cache
WHERE language = $1 AND target_language = $2 AND canonical_lemma = $3 AND upos = $4
  AND provider = $5 AND provider_version = $6 AND sentence_hash = $7 AND dictionary_provider_version = $8
  AND meaning_evidence_hash = $9;

-- name: GetLegacyEnrichmentForSentence :one
SELECT translation, sentence_translation, sentence_translation_target
FROM enrichment_cache
WHERE language = sqlc.arg('language')
  AND target_language = 'en'
  AND canonical_lemma = sqlc.arg('canonical_lemma')
  AND upos = upper(sqlc.arg('upos')::text)
  AND sentence_hash = sqlc.arg('sentence_hash')
ORDER BY cached_at DESC
LIMIT 1;
