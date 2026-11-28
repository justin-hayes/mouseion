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

-- name: BookEverCurrentReading :one
-- The Book holds the Current reading role now. An ended reading leaves no
-- trace (ADR 0136 section 4).
SELECT EXISTS(
  SELECT 1 FROM primary_goals g
  WHERE g.owner_id = sqlc.arg('owner') AND g.book_id = sqlc.arg('book')
)::boolean;

-- name: BookHasActiveCurrentReading :one
SELECT EXISTS(
  SELECT 1 FROM primary_goals g
  WHERE g.owner_id = sqlc.arg('owner') AND g.book_id = sqlc.arg('book')
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
      , COALESCE(f.failure_code, '')::text AS analysis_failure_code
      , lower(e.book_title)::text AS sort_title
      , d.disposition::text AS disposition
     , d.revision AS disposition_revision
     , COALESCE(history.completion_count, 0)::bigint AS completion_count
     , history.latest_completed_at
      , COALESCE(history.latest_completion_source, '')::text AS latest_completion_source
     , EXISTS (
         SELECT 1 FROM primary_goals g
         WHERE g.owner_id::text = e.book_owner_id AND g.book_id::text = e.book_id
       ) AS is_current_reading
     , (EXISTS (
         SELECT 1 FROM primary_goals g
         WHERE g.owner_id::text = e.book_owner_id AND g.book_id::text = e.book_id
       ))::boolean AS ever_current_reading
     , COALESCE(ru.definition_key, '')::text AS reading_unit_key
     , COALESCE(w.definition_key, '')::text AS work_key
     , COALESCE(w.title, '')::text AS work_title
     , COALESCE(w.author, '')::text AS work_author
     , COALESCE(w.greek_title, '')::text AS greek_title
     , COALESCE(ed.label, '')::text AS edition_label
     , COALESCE(ru.constituent_book, 0)::integer AS constituent_book
     , COALESCE(ru.constituent_books, 0)::integer AS constituent_books
     , COALESCE(ru.revision, 0)::integer AS revision
     , COALESCE(ru.withdrawn, false)::boolean AS withdrawn
FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
LEFT JOIN latest_analysis_failure f ON f.owner_id::text = e.book_owner_id AND f.source_material_id::text = e.source_id
LEFT JOIN book_reading_units bru ON bru.owner_id::text = e.book_owner_id AND bru.book_id::text = e.book_id
LEFT JOIN reading_units ru ON ru.id = bru.reading_unit_id
LEFT JOIN editions ed ON ed.id = ru.edition_id
LEFT JOIN works w ON w.id = ed.work_id
LEFT JOIN LATERAL (
    SELECT count(*) AS completion_count,
           COALESCE((SELECT h.completed_at FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1), 'epoch'::timestamptz) AS latest_completed_at,
           (SELECT h.completion_source FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1) AS latest_completion_source
    FROM reading_history h
    WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id
 ) history ON true
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (
    sqlc.arg('language')::text = ''
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
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (
    sqlc.arg('language')::text = ''
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  );

-- name: CountMyBooksAll :one
-- Counts only chosen Books, the same rule BrowseMyBooksEvidence lists by.
SELECT count(*) FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner');

-- name: GetMyBookDetail :one
SELECT e.*
     , (pos.id IS NOT NULL)::boolean AS has_reading_position
     , COALESCE(pos.fraction, 0)::numeric(7,6) AS position_fraction
     , COALESCE(pos.source, '')::text AS position_source
     , COALESCE(pos.recorded_at, 'epoch'::timestamptz)::timestamptz AS position_recorded_at
FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
LEFT JOIN LATERAL (
    SELECT r.id, r.fraction, r.source, r.recorded_at
    FROM reading_position_reports r
    WHERE r.owner_id::text = e.book_owner_id AND r.book_id::text = e.book_id
    ORDER BY r.recorded_at DESC, r.id DESC
    LIMIT 1
) pos ON true
WHERE e.book_owner_id = sqlc.arg('owner') AND (e.book_id = sqlc.arg('id') OR EXISTS (
  SELECT 1 FROM source_materials requested_source
  WHERE requested_source.owner_id::text = e.book_owner_id
    AND requested_source.book_id::text = e.book_id
    AND requested_source.id::text = sqlc.arg('id')
))
ORDER BY CASE WHEN e.book_id = sqlc.arg('id') THEN 0 ELSE 1 END
LIMIT 1;

-- name: GetLatestAnalysisFailureCode :one
SELECT COALESCE((
    SELECT failure_code FROM latest_analysis_failure
    WHERE owner_id = sqlc.arg('owner') AND source_material_id = sqlc.arg('source')
), '')::text AS failure_code;

-- name: ListMyBooksCoverage :many
-- Coverage comes from the durable Vocabulary Browse inventory counts joined to
-- the learner's Study progress (ADR 0136 §3): mature_tokens counts occurrences
-- of words that are Mature (in review with a scheduled interval of at least
-- mature_days), and in_study_tokens also counts words introduced but not yet
-- Mature. Only word items count and a withdrawn word counts as neither. A Book
-- without ready counts for its current analysis produces no row, so it never
-- shows a stale or scanned figure.
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
      AND r.builder_version = 5
    JOIN corpora co ON co.owner_id = r.owner_id AND co.id = r.corpus_id
      AND co.analysis_run_id = r.analysis_run_id
), counted AS (
    SELECT ready.book_id, c.occurrence_count,
           sp.id IS NOT NULL AS in_study,
           COALESCE(sp.state = 'review' AND sp.scheduled_days >= sqlc.arg('mature_days')::bigint, false) AS is_mature
    FROM ready
    JOIN vocabulary_browse_counts c ON c.owner_id = ready.owner_id AND c.book_id = ready.book_id
      AND c.analysis_run_id = ready.analysis_run_id AND c.corpus_id = ready.corpus_id
    LEFT JOIN study_progress sp ON sp.owner_id = c.owner_id AND sp.item_kind = 'word'
      AND sp.language = c.language
      AND sp.canonical_lemma = btrim(c.canonical_lemma)
      AND sp.upos = upper(btrim(c.upos))
      AND sp.lexical_discriminator = btrim(c.lexical_discriminator)
      AND sp.withdrawn_at IS NULL
)
SELECT ready.book_id::text AS book_id,
       ready.total_tokens,
       COALESCE((SELECT sum(counted.occurrence_count) FROM counted
                  WHERE counted.book_id = ready.book_id AND counted.is_mature), 0)::bigint AS mature_tokens,
       COALESCE((SELECT sum(counted.occurrence_count) FROM counted
                  WHERE counted.book_id = ready.book_id AND counted.in_study), 0)::bigint AS in_study_tokens
FROM ready;

-- name: GetMyBookCompletionSummary :one
SELECT count(*)::bigint AS completion_count,
       COALESCE((SELECT h.completed_at FROM reading_history h WHERE h.owner_id = sqlc.arg('owner') AND h.book_id = sqlc.arg('book') ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1), 'epoch'::timestamptz) AS latest_completed_at,
       COALESCE((SELECT h.completion_source FROM reading_history h WHERE h.owner_id = sqlc.arg('owner') AND h.book_id = sqlc.arg('book') ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1), '')::text AS latest_completion_source
FROM reading_history
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');

-- name: CatalogueIndexBackfillBlocked :one
SELECT EXISTS (
  SELECT 1
  FROM opds_connections c
  LEFT JOIN catalogue_sync_status s
    ON s.owner_id = c.owner_id AND s.connection_id = c.id
  WHERE s.connection_id IS NULL
     OR s.last_synced_at IS NULL
);

-- name: ListLanguageLessReadBooks :many
SELECT b.id, b.owner_id, b.title, b.author, b.metadata_provenance,
       b.language_state, COALESCE(b.language_tag, '')::text AS language_tag,
       b.created_at, b.updated_at
FROM books b
JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id
WHERE m.state = 'active'
  AND b.language_state <> 'chosen'
  AND EXISTS (
    SELECT 1 FROM reading_history h
    WHERE h.owner_id = b.owner_id AND h.book_id = b.id
  )
ORDER BY b.owner_id, b.title, b.id;

-- name: RemoveUnchosenBookMembership :execrows
UPDATE book_membership m
SET state = 'removed', removed_at = now()
WHERE m.state = 'active'
  AND NOT EXISTS (
    SELECT 1 FROM primary_goals g
    WHERE g.owner_id = m.owner_id AND g.book_id = m.book_id
  )
  AND (
    -- A Book with no chosen language is listed nowhere, so it leaves My Books
    -- even when it has reading history; the completion is kept.
    EXISTS (
      SELECT 1 FROM books b
      WHERE b.owner_id = m.owner_id AND b.id = m.book_id
        AND b.language_state <> 'chosen'
    )
    OR (
      NOT EXISTS (
        SELECT 1 FROM book_dispositions d
        WHERE d.owner_id = m.owner_id AND d.book_id = m.book_id
          AND d.disposition = 'to_read'
      )
      AND NOT EXISTS (
        SELECT 1 FROM reading_history h
        WHERE h.owner_id = m.owner_id AND h.book_id = m.book_id
      )
    )
  );
