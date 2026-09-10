UPDATE deck_preparations p
 SET studying_at=NULL, reviewed_at=NULL, graduated_at=NULL, released_at=NULL
 WHERE EXISTS (SELECT 1 FROM learning_campaigns c WHERE c.owner_id=p.owner_id AND c.deck_preparation_id=p.id);
DELETE FROM deck_preparation_vocabulary dv
 WHERE EXISTS (SELECT 1 FROM learning_campaign_vocabulary cv JOIN learning_campaigns c ON c.owner_id=cv.owner_id AND c.id=cv.campaign_id WHERE cv.owner_id=dv.owner_id AND c.deck_preparation_id=dv.deck_preparation_id AND cv.language=dv.language AND cv.canonical_lemma=dv.canonical_lemma AND cv.upos=dv.upos);
