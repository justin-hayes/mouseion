-- Book identity and My Books evidence queries. The read models select from
-- the my_books_evidence view (migration 000066, rebuilt over the shared
-- source_material_evidence view in 000068) so the composed projection and its
-- analysis status/state classification are one SQL artifact instead of Go
-- string stitching.

-- name: BookExists :one
SELECT EXISTS(
  SELECT 1 FROM books
  WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('book')
);

-- name: ResolveLinkedSourceBook :one
SELECT (COALESCE(sm.book_id::text, b.id::text, ''))::text AS linked_book_id
FROM source_materials sm
LEFT JOIN book_aliases a
  ON a.owner_id = sm.owner_id
 AND a.namespace = sqlc.arg('namespace')
 AND a.value = sm.source_identifier
LEFT JOIN books b ON b.owner_id = a.owner_id AND b.id = a.book_id
WHERE sm.owner_id = sqlc.arg('owner') AND sm.id = sqlc.arg('source');

-- name: ListActiveBooks :many
SELECT b.id::text,
       b.owner_id::text,
       b.title,
       b.author,
       b.metadata_provenance,
       b.language_state,
       COALESCE(b.language_tag, '') AS language_tag,
       b.created_at,
       b.updated_at
FROM books b
JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id AND m.state = 'active'
WHERE b.owner_id = sqlc.arg('owner')
ORDER BY b.title, b.id;

-- name: GetBookForUpdate :one
SELECT id::text
FROM books
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: BrowseMyBooksEvidence :many
SELECT e.*
      , lower(e.book_title)::text AS sort_title
      , d.disposition::text AS disposition
     , d.revision AS disposition_revision
     , COALESCE(v.hidden, false)::boolean AS hidden
     , COALESCE(v.revision, 0)::bigint AS visibility_revision
     , COALESCE(history.completion_count, 0)::bigint AS completion_count
     , history.latest_completed_at
      , COALESCE(history.latest_completion_source, '')::text AS latest_completion_source
      , COALESCE(deck.state, '')::text AS deck_state
      , COALESCE(deck.total_cards, 0)::bigint AS deck_total_cards
      , deck.completed_at AS deck_completed_at
     , EXISTS (
         SELECT 1 FROM primary_goals g
         WHERE g.owner_id::text = e.book_owner_id AND g.book_id::text = e.book_id
       ) AS is_current_reading
FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
LEFT JOIN book_visibility v ON v.owner_id::text = e.book_owner_id AND v.book_id::text = e.book_id
LEFT JOIN LATERAL (
    SELECT count(*) AS completion_count,
           COALESCE((SELECT h.completed_at FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1), 'epoch'::timestamptz) AS latest_completed_at,
           (SELECT h.completion_source FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1) AS latest_completion_source
    FROM reading_history h
    WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id
 ) history ON true
 LEFT JOIN LATERAL (
     SELECT p.state::text AS state, p.total_cards::bigint AS total_cards, p.completed_at
     FROM deck_preparations p
      WHERE p.owner_id::text = e.book_owner_id AND p.book_id::text = e.book_id
        AND p.retired_at IS NULL AND p.analysis_run_id::text = e.analysis_run_id
        AND EXISTS (SELECT 1 FROM primary_goals g WHERE g.owner_id::text = e.book_owner_id AND g.book_id::text = e.book_id)
     ORDER BY p.updated_at DESC, p.id DESC LIMIT 1
 ) deck ON true
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND book_language_state = 'unknown')
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  )
  AND (
    NOT sqlc.arg('has_cursor')::boolean
    OR ROW(lower(e.book_title), e.book_title, e.book_id) > ROW(
      sqlc.arg('after_sort_title')::text,
      sqlc.arg('after_book_title')::text,
      sqlc.arg('after_book_id')::text
    )
  )
ORDER BY lower(e.book_title), e.book_title, e.book_id
LIMIT sqlc.arg('limit');

-- name: CountMyBooksScope :one
SELECT count(*) FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
LEFT JOIN book_visibility v ON v.owner_id::text = e.book_owner_id AND v.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (sqlc.arg('show_hidden')::boolean OR NOT COALESCE(v.hidden, false))
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND book_language_state = 'unknown')
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  );

-- name: CountHiddenMyBooksScope :one
SELECT count(*) FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
JOIN book_visibility v ON v.owner_id::text = e.book_owner_id AND v.book_id::text = e.book_id AND v.hidden
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND book_language_state = 'unknown')
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  );

-- name: CountMyBooksAll :one
SELECT count(*) FROM my_books_evidence
WHERE book_owner_id = sqlc.arg('owner');

-- name: CountMyBooksByLanguage :many
SELECT (CASE WHEN book_language_state = 'unknown' THEN 'unknown' ELSE book_language_tag END)::text AS language_tag,
       count(*) AS book_count
FROM my_books_evidence e
LEFT JOIN book_visibility v ON v.owner_id::text = e.book_owner_id AND v.book_id::text = e.book_id
WHERE book_owner_id = sqlc.arg('owner') AND book_language_state IN ('chosen', 'unknown')
  AND (sqlc.arg('show_hidden')::boolean OR NOT COALESCE(v.hidden, false))
GROUP BY 1;

-- name: GetMyBookDetail :one
SELECT e.*
FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner') AND (e.book_id = sqlc.arg('id') OR EXISTS (
  SELECT 1 FROM source_materials requested_source
  WHERE requested_source.owner_id::text = e.book_owner_id
    AND requested_source.book_id::text = e.book_id
    AND requested_source.id::text = sqlc.arg('id')
))
ORDER BY CASE WHEN e.book_id = sqlc.arg('id') THEN 0 ELSE 1 END
LIMIT 1;

-- name: GetMyBookMarginEvidence :one
SELECT COALESCE(deck.state, '')::text AS deck_state,
       COALESCE(deck.total_cards, 0)::bigint AS deck_total_cards,
       deck.completed_at AS deck_completed_at,
       EXISTS (SELECT 1 FROM primary_goals g WHERE g.owner_id = e.book_owner_id::uuid AND g.book_id = e.book_id::uuid) AS is_current_reading
FROM my_books_evidence e
LEFT JOIN LATERAL (
    SELECT p.state::text AS state, p.total_cards::bigint AS total_cards, p.completed_at
    FROM deck_preparations p
     WHERE p.owner_id::text = e.book_owner_id AND p.book_id::text = e.book_id
       AND p.retired_at IS NULL AND p.analysis_run_id::text = e.analysis_run_id
       AND EXISTS (SELECT 1 FROM primary_goals g WHERE g.owner_id::text = e.book_owner_id AND g.book_id::text = e.book_id)
    ORDER BY p.updated_at DESC, p.id DESC LIMIT 1
) deck ON true
WHERE e.book_owner_id = sqlc.arg('owner') AND e.book_id = sqlc.arg('book');

-- name: ListMyBooksCoverage :many
-- Coverage comes from the durable Vocabulary Browse inventory counts joined to
-- the learner's current Known vocabulary. A Book without ready counts for its
-- current analysis produces no row, so it never shows a stale or scanned figure.
WITH ready AS (
    SELECT r.owner_id, r.book_id, r.analysis_run_id, r.corpus_id,
           COALESCE(co.analyzable_token_count, 0)::bigint AS total_tokens
    FROM my_books_evidence e
    JOIN vocabulary_browse_count_readiness r ON r.owner_id = sqlc.arg('owner')::uuid
      AND r.book_id = ANY(sqlc.arg('book_ids')::uuid[])
      AND r.owner_id::text = e.book_owner_id
      AND r.book_id::text = e.book_id
      AND r.analysis_run_id::text = e.analysis_run_id
      AND r.corpus_id::text = e.corpus_id
      AND r.builder_version = 2
    JOIN corpora co ON co.owner_id = r.owner_id AND co.id = r.corpus_id
      AND co.analysis_run_id = r.analysis_run_id
), counted AS (
    SELECT ready.book_id, c.occurrence_count,
           EXISTS (
               SELECT 1 FROM known_vocabulary kv
               WHERE kv.owner_id = c.owner_id AND kv.language = c.language
                 AND kv.canonical_lemma = btrim(c.canonical_lemma)
                 AND (kv.upos = upper(btrim(c.upos)) OR kv.upos = '')
           ) AS is_known
    FROM ready
    JOIN vocabulary_browse_counts c ON c.owner_id = ready.owner_id AND c.book_id = ready.book_id
      AND c.analysis_run_id = ready.analysis_run_id AND c.corpus_id = ready.corpus_id
)
SELECT ready.book_id::text AS book_id,
       ready.total_tokens,
       COALESCE((SELECT sum(counted.occurrence_count) FROM counted
                 WHERE counted.book_id = ready.book_id AND counted.is_known), 0)::bigint AS known_tokens
FROM ready;

-- name: GetMyBookCompletionSummary :one
SELECT count(*)::bigint AS completion_count,
       COALESCE((SELECT h.completed_at FROM reading_history h WHERE h.owner_id = sqlc.arg('owner') AND h.book_id = sqlc.arg('book') ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1), 'epoch'::timestamptz) AS latest_completed_at,
       COALESCE((SELECT h.completion_source FROM reading_history h WHERE h.owner_id = sqlc.arg('owner') AND h.book_id = sqlc.arg('book') ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1), '')::text AS latest_completion_source
FROM reading_history
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');
