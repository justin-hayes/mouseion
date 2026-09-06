package persistence

// currentAnalysisCTE is the single identity chain used by learner-facing
// readers. A pointer is current only while its source, completed run, and
// complete corpus still agree on owner, source, revision, and snapshot.
const currentAnalysisCTE = `
WITH current_analysis AS (
  SELECT p.owner_id,p.book_id,p.source_material_id,p.analysis_run_id,
         r.content_revision_id,r.snapshot_id,
          c.id AS corpus_id
  FROM book_current_analyses p
  JOIN source_materials source
    ON source.owner_id=p.owner_id AND source.id=p.source_material_id
   AND source.book_id=p.book_id
  JOIN analysis_runs r
    ON r.owner_id=p.owner_id AND r.id=p.analysis_run_id
   AND r.source_material_id=p.source_material_id AND r.state='completed'
  JOIN corpora c
    ON c.owner_id=r.owner_id AND c.id=r.corpus_id
   AND c.source_material_id=r.source_material_id
    AND c.analysis_run_id=r.id
   AND c.status='complete'
  WHERE source.current_content_revision_id=r.content_revision_id
    AND source.current_snapshot_id=r.snapshot_id
)`
