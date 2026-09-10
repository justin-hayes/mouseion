-- Data-only, retry-safe correction for ADR 0053. The deployment owner runs
-- this migration before 000061 removes the campaign source tables.
--
-- golang-migrate applies each migration transactionally. The statements take
-- row locks on the affected deck preparations and snapshots; the deployment
-- owner should observe migration logs and database lock duration. A failed
-- transaction rolls back and is safe to retry. After a committed run, use a
-- forward fix from backup rather than a down migration to recover data.
-- No generated vocabulary is marked known by this migration.
INSERT INTO deck_preparation_vocabulary(owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at, graduated_at)
 SELECT cv.owner_id, c.deck_preparation_id, cv.language, cv.canonical_lemma, cv.upos, cv.generated_at, cv.graduated_at
 FROM learning_campaign_vocabulary cv
 JOIN learning_campaigns c ON c.owner_id=cv.owner_id AND c.id=cv.campaign_id
 ON CONFLICT (owner_id, deck_preparation_id, language, canonical_lemma, upos) DO UPDATE
   SET graduated_at=COALESCE(deck_preparation_vocabulary.graduated_at, EXCLUDED.graduated_at);

-- Complete campaigns, and active campaigns whose deck was already reviewed,
-- are graduated. Other active campaigns are studying. Queued and abandoned
-- campaigns are released without a study lease.
UPDATE deck_preparations p
 SET studying_at=CASE
       WHEN c.status='active' AND c.deck_status <> 'reviewed' THEN COALESCE(p.studying_at,c.activated_at,c.created_at)
       ELSE NULL
     END,
     reviewed_at=CASE
       WHEN c.status='complete' OR (c.status='active' AND c.deck_status='reviewed')
         THEN COALESCE(p.reviewed_at,c.deck_reviewed_at,c.completed_at,c.updated_at)
       ELSE NULL
     END,
     graduated_at=CASE
       WHEN c.status='complete' OR (c.status='active' AND c.deck_status='reviewed')
         THEN COALESCE(p.graduated_at,c.vocabulary_graduated_at,c.deck_reviewed_at,c.completed_at,c.updated_at)
       ELSE NULL
     END,
     released_at=CASE
       WHEN c.status IN ('queued','abandoned')
         THEN COALESCE(p.released_at,CASE WHEN c.status='abandoned' THEN COALESCE(c.abandoned_at,c.updated_at) ELSE c.updated_at END)
       ELSE NULL
     END,
     updated_at=GREATEST(p.updated_at,c.updated_at)
 FROM learning_campaigns c
 WHERE c.owner_id=p.owner_id AND c.deck_preparation_id=p.id;
