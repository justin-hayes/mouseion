-- Per-Book Idiom candidate counts (ADR 0137). The Corpus sums the counts of
-- every Book whose counts are ready for its current analysis.

-- name: ListCorpusIdiomCandidateCounts :many
-- Each Idiom headword's candidate count across the owner's currently analyzed
-- Books in a language, with the number of Books containing it. A Book counts
-- only while it is in My Books (removing a Book leaves its rows but not the
-- sum; adding it back restores them) and its readiness row names its current analysis and the builder
-- version; a Book that was removed has no rows left. in_current_reading says
-- whether the Current reading contains the headword and current_reading_candidates
-- its candidate count within it.
SELECT c.headword, sum(c.candidate_count)::bigint AS candidates, count(*)::bigint AS books,
 COALESCE(bool_or(c.book_id = g.book_id), false)::boolean AS in_current_reading,
 COALESCE(sum(c.candidate_count) FILTER (WHERE c.book_id = g.book_id), 0)::bigint AS current_reading_candidates
FROM idiom_candidate_counts c
JOIN idiom_candidate_count_readiness r ON r.owner_id=c.owner_id AND r.book_id=c.book_id
 AND r.analysis_run_id=c.analysis_run_id AND r.corpus_id=c.corpus_id
 AND r.builder_version=sqlc.arg(builder_version)::int AND r.index_version=sqlc.arg(index_version)::text
JOIN books b ON b.owner_id=c.owner_id AND b.id=c.book_id
 AND b.language_state='chosen' AND b.language_tag=sqlc.arg(language)::text
JOIN book_membership m ON m.owner_id=c.owner_id AND m.book_id=c.book_id AND m.state='active'
JOIN current_analysis_identity cai ON cai.owner_id=c.owner_id AND cai.book_id=c.book_id
 AND cai.analysis_run_id=c.analysis_run_id AND cai.corpus_id=c.corpus_id
LEFT JOIN primary_goals g ON g.owner_id=c.owner_id AND g.language=sqlc.arg(language)::text
WHERE c.owner_id=sqlc.arg(owner_id)::uuid AND c.language=sqlc.arg(language)::text
GROUP BY c.headword
ORDER BY c.headword;

-- name: CountCorpusIdiomCountsPending :one
-- The Books in the Corpus whose Idiom counts are not ready for their current
-- analysis, the builder version and the idiom index version, so they add
-- nothing to the sum yet. A reader must not take a Headword's absence from the
-- sum as zero candidates while this is above zero.
SELECT count(*)::bigint AS pending
FROM current_analysis_identity cai
JOIN books b ON b.owner_id=cai.owner_id AND b.id=cai.book_id
 AND b.language_state='chosen' AND b.language_tag=sqlc.arg(language)::text
JOIN book_membership m ON m.owner_id=cai.owner_id AND m.book_id=cai.book_id AND m.state='active'
LEFT JOIN idiom_candidate_count_readiness r ON r.owner_id=cai.owner_id AND r.book_id=cai.book_id
 AND r.analysis_run_id=cai.analysis_run_id AND r.corpus_id=cai.corpus_id
 AND r.builder_version=sqlc.arg(builder_version)::int AND r.index_version=sqlc.arg(index_version)::text
WHERE cai.owner_id=sqlc.arg(owner_id)::uuid AND r.owner_id IS NULL;
