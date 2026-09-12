-- Analysis-insight vocabulary aggregation.

-- name: ListAnalysisCorpusVocabulary :many
SELECT sl.language,
       sl.canonical_lemma,
       sl.upos,
       SUM(sl.frequency)::bigint AS occurrence_count
FROM corpora co
JOIN shared_lemmas sl ON sl.content_hash = co.artifact_hash
WHERE co.owner_id = $1
  AND co.id = $2
  AND upper(sl.upos) IN ('NOUN', 'VERB', 'ADJ', 'ADV')
  AND btrim(sl.canonical_lemma) <> ''
GROUP BY sl.language, sl.canonical_lemma, sl.upos
ORDER BY sl.language, sl.canonical_lemma, sl.upos;
