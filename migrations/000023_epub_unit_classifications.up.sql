ALTER TABLE source_material_unit_snapshots
 ADD COLUMN snapshot_id uuid NOT NULL DEFAULT gen_random_uuid(),
 ADD CONSTRAINT source_material_unit_snapshots_identity UNIQUE(owner_id, source_material_id, snapshot_id);

CREATE TABLE source_material_unit_classifications (
 owner_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 snapshot_id uuid NOT NULL,
 unit_id text NOT NULL,
 schema_version integer NOT NULL CHECK (schema_version = 1),
 extracted_units_schema_version integer NOT NULL CHECK (extracted_units_schema_version = 1),
 classifier_name text NOT NULL CHECK (classifier_name ~ '^[[:alnum:]_.-]+$'),
 classifier_version text NOT NULL CHECK (classifier_version ~ '^[[:alnum:]_.-]+$'),
 category text NOT NULL CHECK (category IN ('front_matter','main_matter','back_matter','unknown')),
 confidence integer NOT NULL CHECK (confidence BETWEEN 0 AND 100),
 recommended_inclusion boolean NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_id, source_material_id, snapshot_id, classifier_name, classifier_version, unit_id),
 FOREIGN KEY(owner_id, source_material_id, snapshot_id) REFERENCES source_material_unit_snapshots(owner_id, source_material_id, snapshot_id) ON DELETE CASCADE,
 CONSTRAINT source_material_unit_classifications_source_unit_fkey
  FOREIGN KEY(owner_id, source_material_id, unit_id)
  REFERENCES source_material_units(owner_id, source_material_id, unit_id) ON DELETE CASCADE,
 CHECK (category <> 'unknown' OR confidence <= 49),
 CHECK (confidence < 80 OR category <> 'main_matter' OR recommended_inclusion),
 CHECK (confidence < 80 OR category NOT IN ('front_matter','back_matter') OR NOT recommended_inclusion)
);

CREATE TABLE source_material_unit_classification_reasons (
 owner_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 snapshot_id uuid NOT NULL,
 classifier_name text NOT NULL,
 classifier_version text NOT NULL,
 unit_id text NOT NULL,
 reason_order integer NOT NULL CHECK (reason_order >= 0),
 signal text NOT NULL CHECK (signal ~ '^[[:alnum:]_.-]+$'),
 message text NOT NULL CHECK (message <> '' AND message = btrim(message) AND message !~ '[[:cntrl:]]'),
 PRIMARY KEY(owner_id, source_material_id, snapshot_id, classifier_name, classifier_version, unit_id, reason_order),
 UNIQUE(owner_id, source_material_id, snapshot_id, classifier_name, classifier_version, unit_id, signal),
 FOREIGN KEY(owner_id, source_material_id, snapshot_id, classifier_name, classifier_version, unit_id)
  REFERENCES source_material_unit_classifications(owner_id, source_material_id, snapshot_id, classifier_name, classifier_version, unit_id) ON DELETE CASCADE
);
