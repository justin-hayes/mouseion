ALTER TABLE source_materials
  ADD CONSTRAINT source_materials_owner_id_id_book_id_key
  UNIQUE (owner_id, id, book_id);

CREATE TABLE book_current_analyses (
  owner_id uuid NOT NULL,
  book_id uuid NOT NULL,
  source_material_id uuid NOT NULL,
  analysis_run_id uuid NOT NULL,
  promoted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, book_id),
  UNIQUE (owner_id, analysis_run_id),
  FOREIGN KEY (owner_id, book_id) REFERENCES books(owner_id, id) ON DELETE CASCADE,
  FOREIGN KEY (owner_id, source_material_id, book_id) REFERENCES source_materials(owner_id, id, book_id),
  FOREIGN KEY (owner_id, analysis_run_id, source_material_id) REFERENCES analysis_runs(owner_id, id, source_material_id)
);

-- Only an unambiguous, complete, current scoped result may become current during
-- rollout. Ambiguous groups are deliberately left for operator review rather
-- than being selected by freshness.
WITH eligible AS (
  SELECT s.owner_id, s.book_id, s.id AS source_material_id, r.id AS analysis_run_id
  FROM source_materials s
  JOIN analysis_runs r
    ON r.owner_id=s.owner_id AND r.source_material_id=s.id AND r.state='completed'
  JOIN corpora c
    ON c.owner_id=r.owner_id AND c.id=r.corpus_id
   AND c.source_material_id=r.source_material_id
   AND c.analysis_run_id=r.id AND c.reviewed_scope_id=r.scope_id
   AND c.status='complete'
  JOIN epub_reviewed_scopes scope
    ON scope.scope_id=r.scope_id AND scope.owner_id=r.owner_id
   AND scope.source_material_id=r.source_material_id
   AND scope.content_revision_id=r.content_revision_id
   AND scope.snapshot_id=r.snapshot_id
  WHERE s.book_id IS NOT NULL
    AND s.current_content_revision_id=r.content_revision_id
    AND s.current_snapshot_id=r.snapshot_id
), unique_books AS (
  SELECT owner_id, book_id
  FROM eligible
  GROUP BY owner_id, book_id
  HAVING count(*)=1
)
INSERT INTO book_current_analyses(owner_id, book_id, source_material_id, analysis_run_id)
SELECT e.owner_id, e.book_id, e.source_material_id, e.analysis_run_id
FROM eligible e
JOIN unique_books u ON u.owner_id=e.owner_id AND u.book_id=e.book_id;

-- PostgreSQL migration runners expose these notices in their migration log so
-- ambiguous groups can be re-reviewed or reanalyzed without inventing a
-- current result. No historical rows are modified.
DO $$
DECLARE
  skipped record;
BEGIN
  FOR skipped IN
    WITH eligible AS (
      SELECT s.owner_id, s.book_id, r.id AS analysis_run_id
      FROM source_materials s
      JOIN analysis_runs r
        ON r.owner_id=s.owner_id AND r.source_material_id=s.id AND r.state='completed'
      JOIN corpora c
        ON c.owner_id=r.owner_id AND c.id=r.corpus_id
       AND c.source_material_id=r.source_material_id
       AND c.analysis_run_id=r.id AND c.reviewed_scope_id=r.scope_id
       AND c.status='complete'
      JOIN epub_reviewed_scopes scope
        ON scope.scope_id=r.scope_id AND scope.owner_id=r.owner_id
       AND scope.source_material_id=r.source_material_id
       AND scope.content_revision_id=r.content_revision_id
       AND scope.snapshot_id=r.snapshot_id
      WHERE s.book_id IS NOT NULL
        AND s.current_content_revision_id=r.content_revision_id
        AND s.current_snapshot_id=r.snapshot_id
    )
    SELECT owner_id, book_id, count(*) AS candidate_count
    FROM eligible
    GROUP BY owner_id, book_id
    HAVING count(*) > 1
  LOOP
    RAISE NOTICE '000044 skipped ambiguous current-analysis backfill owner=% book=% candidates=%', skipped.owner_id, skipped.book_id, skipped.candidate_count;
  END LOOP;
END $$;
