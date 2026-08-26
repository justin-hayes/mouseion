-- Keep immutable EPUB bytes separate from mutable bibliographic metadata. The
-- legacy source_materials columns remain as a compatibility projection for
-- full-text callers; revision rows are the authority for EPUB content.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE source_materials
 DROP CONSTRAINT IF EXISTS source_materials_owner_id_content_hash_key;

CREATE TABLE source_content_revisions (
 revision_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 digest_version integer NOT NULL CHECK (digest_version IN (0,1)),
 content_digest text NOT NULL CHECK (content_digest ~ '^(sha256:[0-9a-f]{64}|legacy:[^[:space:]]+)$'),
 content bytea NOT NULL,
 full_text text NOT NULL,
 CHECK (digest_version = 0 OR content_digest = 'sha256:'||encode(digest(content,'sha256'),'hex')),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, revision_id),
 UNIQUE(owner_id, source_material_id, revision_id),
 UNIQUE(owner_id, source_material_id, content_digest),
 FOREIGN KEY(owner_id, source_material_id)
  REFERENCES source_materials(owner_id, id) ON DELETE CASCADE
);

INSERT INTO source_content_revisions(owner_id,source_material_id,digest_version,content_digest,content,full_text)
SELECT owner_id,id,0,'legacy:'||encode(digest(content_hash,'sha256'),'hex'),content,full_text
FROM source_materials;
-- Existing content_hash values may identify an analysis artifact rather than
-- the downloaded EPUB bytes. Marking these rows as legacy preserves readability
-- without falsely claiming cryptographic provenance; new imports use version 1.

ALTER TABLE source_materials ADD COLUMN current_content_revision_id uuid;
UPDATE source_materials s
SET current_content_revision_id = r.revision_id
FROM source_content_revisions r
WHERE r.owner_id=s.owner_id AND r.source_material_id=s.id;
ALTER TABLE source_materials
 ADD CONSTRAINT source_materials_current_content_revision_fkey
 FOREIGN KEY(owner_id,current_content_revision_id)
 REFERENCES source_content_revisions(owner_id,revision_id) DEFERRABLE INITIALLY DEFERRED;

-- Turn the extracted-unit tables into append-only snapshot history.
ALTER TABLE source_material_unit_classifications
 DROP CONSTRAINT IF EXISTS source_material_unit_classifications_owner_id_source_material_id_unit_id_fkey;
ALTER TABLE source_material_units
 DROP CONSTRAINT IF EXISTS source_material_units_owner_id_source_material_id_fkey;
ALTER TABLE source_material_unit_snapshots
 ADD COLUMN content_revision_id uuid;
UPDATE source_material_unit_snapshots s
SET content_revision_id = r.revision_id
FROM source_content_revisions r
WHERE r.owner_id=s.owner_id AND r.source_material_id=s.source_material_id;
ALTER TABLE source_material_unit_snapshots
 ALTER COLUMN content_revision_id SET NOT NULL;
ALTER TABLE source_material_unit_snapshots
 DROP CONSTRAINT source_material_unit_snapshots_pkey;
ALTER TABLE source_material_unit_snapshots
 ADD PRIMARY KEY(owner_id,source_material_id,snapshot_id),
 ADD CONSTRAINT source_material_unit_snapshots_content_revision_fkey
 FOREIGN KEY(owner_id,source_material_id,content_revision_id)
 REFERENCES source_content_revisions(owner_id,source_material_id,revision_id);

ALTER TABLE source_material_units ADD COLUMN snapshot_id uuid;
UPDATE source_material_units u
SET snapshot_id=s.snapshot_id
FROM source_material_unit_snapshots s
WHERE s.owner_id=u.owner_id AND s.source_material_id=u.source_material_id;
ALTER TABLE source_material_units
 ALTER COLUMN snapshot_id SET NOT NULL,
 DROP CONSTRAINT source_material_units_pkey,
 DROP CONSTRAINT source_material_units_owner_id_source_material_id_unit_order_key;
ALTER TABLE source_material_units
 ADD PRIMARY KEY(owner_id,source_material_id,snapshot_id,unit_id),
 ADD CONSTRAINT source_material_units_snapshot_order_key UNIQUE(owner_id,source_material_id,snapshot_id,unit_order),
 ADD CONSTRAINT source_material_units_snapshot_fkey
 FOREIGN KEY(owner_id,source_material_id,snapshot_id)
 REFERENCES source_material_unit_snapshots(owner_id,source_material_id,snapshot_id) ON DELETE CASCADE;
ALTER TABLE source_material_unit_classifications
 ADD CONSTRAINT source_material_unit_classifications_source_unit_fkey
 FOREIGN KEY(owner_id,source_material_id,snapshot_id,unit_id)
 REFERENCES source_material_units(owner_id,source_material_id,snapshot_id,unit_id) ON DELETE CASCADE;

ALTER TABLE source_materials ADD COLUMN current_snapshot_id uuid;
UPDATE source_materials s
SET current_snapshot_id = snap.snapshot_id
FROM source_material_unit_snapshots snap
WHERE snap.owner_id=s.owner_id AND snap.source_material_id=s.id;
ALTER TABLE source_materials
 ADD CONSTRAINT source_materials_current_snapshot_fkey
 FOREIGN KEY(owner_id,id,current_snapshot_id)
 REFERENCES source_material_unit_snapshots(owner_id,source_material_id,snapshot_id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE epub_reviewed_scopes
 ADD COLUMN content_revision_id uuid,
 ADD COLUMN confirmation_key text;
UPDATE epub_reviewed_scopes s
SET content_revision_id = snap.content_revision_id,
    confirmation_key = 'legacy:'||s.scope_id::text
FROM source_material_unit_snapshots snap
WHERE snap.owner_id=s.owner_id AND snap.source_material_id=s.source_material_id AND snap.snapshot_id=s.snapshot_id;
ALTER TABLE epub_reviewed_scopes
 ALTER COLUMN content_revision_id SET NOT NULL,
 ALTER COLUMN confirmation_key SET NOT NULL,
 ADD CONSTRAINT epub_reviewed_scopes_content_revision_fkey
 FOREIGN KEY(owner_id,source_material_id,content_revision_id)
 REFERENCES source_content_revisions(owner_id,source_material_id,revision_id),
 ADD CONSTRAINT epub_reviewed_scopes_snapshot_fkey
 FOREIGN KEY(owner_id,source_material_id,snapshot_id)
 REFERENCES source_material_unit_snapshots(owner_id,source_material_id,snapshot_id);
CREATE UNIQUE INDEX epub_reviewed_scopes_confirmation_key
 ON epub_reviewed_scopes(owner_id,confirmation_key);

ALTER TABLE epub_reviewed_scope_units ADD COLUMN snapshot_id uuid;
UPDATE epub_reviewed_scope_units u
SET snapshot_id=s.snapshot_id
FROM epub_reviewed_scopes s
WHERE s.scope_id=u.scope_id;
ALTER TABLE epub_reviewed_scope_units
 ALTER COLUMN snapshot_id SET NOT NULL,
 ADD CONSTRAINT epub_reviewed_scope_units_snapshot_fkey
 FOREIGN KEY(scope_id,snapshot_id)
 REFERENCES epub_reviewed_scopes(scope_id,snapshot_id),
 ADD CONSTRAINT epub_reviewed_scope_units_source_unit_fkey
 FOREIGN KEY(owner_id,source_material_id,snapshot_id,unit_id)
 REFERENCES source_material_units(owner_id,source_material_id,snapshot_id,unit_id),
 ADD CONSTRAINT epub_reviewed_scope_units_source_order_fkey
 FOREIGN KEY(owner_id,source_material_id,snapshot_id,unit_order)
 REFERENCES source_material_units(owner_id,source_material_id,snapshot_id,unit_order);

CREATE FUNCTION reject_source_content_revision_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'source content revisions are immutable';
END;
$$;
CREATE TRIGGER source_content_revisions_immutable
 BEFORE UPDATE ON source_content_revisions
 FOR EACH ROW EXECUTE FUNCTION reject_source_content_revision_mutation();
CREATE TRIGGER source_material_unit_snapshots_immutable
 BEFORE UPDATE ON source_material_unit_snapshots
 FOR EACH ROW EXECUTE FUNCTION reject_source_content_revision_mutation();
CREATE TRIGGER source_material_units_immutable
 BEFORE UPDATE ON source_material_units
 FOR EACH ROW EXECUTE FUNCTION reject_source_content_revision_mutation();

CREATE FUNCTION reject_source_material_content_replacement() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.content IS DISTINCT FROM NEW.content
    OR OLD.full_text IS DISTINCT FROM NEW.full_text
    OR OLD.content_hash IS DISTINCT FROM NEW.content_hash THEN
  RAISE EXCEPTION 'source content is immutable; create a new content revision';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER source_materials_content_immutable
 BEFORE UPDATE ON source_materials
 FOR EACH ROW EXECUTE FUNCTION reject_source_material_content_replacement();

CREATE FUNCTION reject_reviewed_scope_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'reviewed scope revisions are immutable';
END;
$$;
CREATE TRIGGER epub_reviewed_scopes_immutable
 BEFORE UPDATE ON epub_reviewed_scopes
 FOR EACH ROW EXECUTE FUNCTION reject_reviewed_scope_mutation();
CREATE TRIGGER epub_reviewed_scope_units_immutable
 BEFORE UPDATE ON epub_reviewed_scope_units
 FOR EACH ROW EXECUTE FUNCTION reject_reviewed_scope_mutation();
