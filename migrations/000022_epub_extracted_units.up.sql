CREATE TABLE source_material_unit_snapshots (
 owner_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 schema_version integer NOT NULL CHECK (schema_version > 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_id, source_material_id),
 FOREIGN KEY(owner_id, source_material_id) REFERENCES source_materials(owner_id, id) ON DELETE CASCADE
);

CREATE TABLE source_material_units (
 owner_id uuid NOT NULL, source_material_id uuid NOT NULL,
 unit_id text NOT NULL, unit_order bigint NOT NULL CHECK (unit_order >= 0),
 spine_index bigint NOT NULL CHECK (spine_index >= 0),
 title text NOT NULL, title_source text NOT NULL, text text NOT NULL,
 start_offset bigint NOT NULL CHECK (start_offset >= 0),
 end_offset bigint NOT NULL CHECK (end_offset >= start_offset),
 package_path text NOT NULL, manifest_id text NOT NULL, source_href text NOT NULL,
 resolved_href text NOT NULL, media_type text NOT NULL,
 properties jsonb NOT NULL DEFAULT '[]'::jsonb, linear boolean NOT NULL,
 navigation_labels jsonb NOT NULL DEFAULT '[]'::jsonb,
 landmark_types jsonb NOT NULL DEFAULT '[]'::jsonb,
 selected boolean NOT NULL DEFAULT true,
 PRIMARY KEY(owner_id, source_material_id, unit_id),
 CONSTRAINT source_material_units_source_order_key UNIQUE(owner_id, source_material_id, unit_order),
 FOREIGN KEY(owner_id, source_material_id) REFERENCES source_material_unit_snapshots(owner_id, source_material_id) ON DELETE CASCADE
);
