-- Per-entry cache for the provider-written Ancient Greek LSJ sense summary
-- (ADR 0101, ticket #2172). Its identity is the LSJ pin, the entry's native
-- identity and the provider prompt version; it carries no learner identity and
-- no Book data.

-- name: GetLSJSenseSummary :one
SELECT summary_lines
FROM lsj_sense_summaries
WHERE lsj_pin = $1 AND lsj_entry_id = $2 AND prompt_version = $3;

-- name: UpsertLSJSenseSummary :exec
INSERT INTO lsj_sense_summaries(lsj_pin, lsj_entry_id, prompt_version, summary_lines, created_at)
VALUES($1, $2, $3, $4, $5)
ON CONFLICT (lsj_pin, lsj_entry_id, prompt_version) DO NOTHING;

-- name: ListLSJSenseSummariesByPins :many
SELECT lsj_pin, lsj_entry_id, prompt_version, summary_lines
FROM lsj_sense_summaries
WHERE lsj_pin = ANY(sqlc.arg('pins')::text[]);
