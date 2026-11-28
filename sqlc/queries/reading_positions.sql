-- name: InsertManualPositionReport :execrows
-- Records a manual Position report for a Book still in the owner's Reading. A
-- Book outside Reading inserts nothing.
INSERT INTO reading_position_reports (owner_id, book_id, source, fraction, recorded_at)
SELECT d.owner_id, d.book_id, 'manual', sqlc.arg('fraction')::numeric(7,6), sqlc.arg('recorded_at')::timestamptz
FROM book_dispositions d
JOIN book_membership m ON m.owner_id = d.owner_id AND m.book_id = d.book_id AND m.state = 'active'
WHERE d.owner_id::text = sqlc.arg('owner') AND d.book_id::text = sqlc.arg('book');
