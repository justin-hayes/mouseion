CREATE TABLE epub_reviewed_scopes (
 scope_id uuid PRIMARY KEY,
 owner_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 snapshot_id uuid NOT NULL,
 schema_version integer NOT NULL CHECK (schema_version = 1),
 extracted_units_schema_version integer NOT NULL CHECK (extracted_units_schema_version = 1),
 classifier_name text NOT NULL CHECK (classifier_name ~ '^[[:alnum:]_.-]+$'),
 classifier_version text NOT NULL CHECK (classifier_version ~ '^[[:alnum:]_.-]+$'),
 selection_mode text NOT NULL CHECK (selection_mode IN ('recommended','overridden')),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(scope_id, owner_id, source_material_id),
 FOREIGN KEY(owner_id, source_material_id) REFERENCES source_materials(owner_id, id) ON DELETE CASCADE
);

CREATE TABLE epub_reviewed_scope_units (
 scope_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 unit_id text NOT NULL,
 unit_order bigint NOT NULL CHECK (unit_order >= 0),
 PRIMARY KEY(scope_id, unit_id),
 UNIQUE(scope_id, unit_order),
 FOREIGN KEY(scope_id, owner_id, source_material_id)
  REFERENCES epub_reviewed_scopes(scope_id, owner_id, source_material_id) ON DELETE CASCADE
);
