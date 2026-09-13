ALTER TABLE deck_preparation_manifest_items
 DROP CONSTRAINT deck_preparation_manifest_items_quality_reasons_check;

ALTER TABLE deck_preparation_manifest_items
 ADD CONSTRAINT deck_preparation_manifest_items_quality_reasons_check CHECK(
  cardinality(quality_reasons) <= 16 AND array_position(quality_reasons, NULL) IS NULL AND
  quality_reasons <@ ARRAY[
   'too short or fragmented','too long','usable length','useful context window',
   'target not present as a word','target present','invalid source location',
   'valid source location','complete sentence boundaries',
   'incomplete sentence boundaries','structural noise or boilerplate',
   'no obvious structural noise'
  ]::text[]);

ALTER TABLE deck_preparation_manifest_items
 DROP COLUMN quality_gdex_score;
