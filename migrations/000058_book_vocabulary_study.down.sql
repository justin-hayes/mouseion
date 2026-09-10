DROP TABLE deck_preparation_vocabulary;
DROP INDEX deck_preparations_one_studying_per_owner;
ALTER TABLE deck_preparations
 DROP COLUMN studying_at,
 DROP COLUMN reviewed_at,
 DROP COLUMN graduated_at,
 DROP COLUMN released_at;
