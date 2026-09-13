ALTER TABLE deck_preparation_manifest_items
 ADD COLUMN quality_gdex_score double precision NOT NULL DEFAULT 0
 CHECK (quality_gdex_score BETWEEN 0 AND 1);

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
   'no obvious structural noise','no finite verb and subject',
   'target in subordinate clause','deictic context','named-entity density',
   'optimal length','outside optimal length'
  ]::text[]);
