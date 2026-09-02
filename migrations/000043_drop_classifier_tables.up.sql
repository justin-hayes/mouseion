-- Classifier persistence and its scope metadata are retired. Historical data
-- in these tables and columns is intentionally not preserved.
DROP TABLE source_material_unit_classification_reasons;
DROP TABLE source_material_unit_classifications;

ALTER TABLE epub_reviewed_scopes
 DROP COLUMN classifier_name,
 DROP COLUMN classifier_version,
 DROP COLUMN selection_mode,
 DROP COLUMN confirmation_key;
