-- Owner: the application schema migration deployment. This data-only
-- backfill is idempotent and leaves campaign rows untouched for compatibility
-- and recovery. It runs in the migration transaction; a failed deployment is
-- detected by migration failure and is safely retried or forward-fixed by the
-- same INSERT/UPDATE statements.
--
-- Preserve historical campaign snapshots before the campaign is retired by a
-- later migration.
INSERT INTO deck_preparation_vocabulary(owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at, graduated_at)
 SELECT cv.owner_id, c.deck_preparation_id, cv.language, cv.canonical_lemma, cv.upos, cv.generated_at, cv.graduated_at
 FROM learning_campaign_vocabulary cv
 JOIN learning_campaigns c ON c.owner_id=cv.owner_id AND c.id=cv.campaign_id
 ON CONFLICT DO NOTHING;

-- Only an unfinished active campaign remains a reservation. Completed decks
-- retain review and graduation history without holding the owner-wide lease.
UPDATE deck_preparations p
 SET studying_at=CASE WHEN c.status='active' THEN COALESCE(c.activated_at,c.created_at) ELSE NULL END,
     reviewed_at=CASE WHEN c.deck_status='reviewed' THEN COALESCE(c.deck_reviewed_at,c.updated_at) ELSE NULL END,
     graduated_at=c.vocabulary_graduated_at,
     released_at=CASE WHEN c.status='abandoned' THEN COALESCE(c.abandoned_at,c.updated_at) ELSE NULL END,
     updated_at=GREATEST(p.updated_at,c.updated_at)
 FROM learning_campaigns c
 WHERE c.owner_id=p.owner_id AND c.deck_preparation_id=p.id;
