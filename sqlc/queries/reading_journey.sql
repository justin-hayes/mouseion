-- Reading Journey read queries. Current analysis eligibility comes from the
-- current_analysis_identity view so the identity chain is not duplicated in
-- application SQL.

-- name: PrimaryGoalCandidateEligible :one
SELECT EXISTS(
  SELECT 1
  FROM reading_journey_membership jm
  JOIN books b
    ON b.owner_id = jm.owner_id AND b.id = jm.book_id
  JOIN source_materials s
    ON s.owner_id = jm.owner_id AND s.book_id = jm.book_id
  JOIN current_analysis_identity ca
    ON ca.owner_id = jm.owner_id
   AND ca.book_id = jm.book_id
   AND ca.source_material_id = s.id
  WHERE jm.owner_id = sqlc.arg('owner')
    AND jm.language = sqlc.arg('language')
    AND jm.book_id = sqlc.arg('book')
    AND b.language_state = 'chosen'
    AND b.language_tag = sqlc.arg('language')
    AND lower(s.media_type) = 'application/epub+zip'
);
