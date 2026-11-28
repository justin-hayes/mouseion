-- The Catalog file's Document digest on content revisions (ADR 0142, ADR 0038).
-- document_digest is KOReader's partial MD5 of the Catalog file the revision was
-- acquired from: 32 lower-case hex digits, or NULL when unknown (a slimmed
-- revision acquired before this migration). It is not unique, because two
-- revisions may legitimately share a Catalog file. The index serves the
-- owner-scoped lookup by digest that Document links add later.
--
-- Structural DDL only; the backfill of existing revisions is the next
-- migration. Nothing reads the column yet.
--
-- Revisions stay immutable in content (ADR 0035, ADR 0103). The immutability
-- trigger now permits exactly one change: document_digest going from NULL to a
-- value, which acquisition and the backfill use to fill an empty digest. Every
-- other column must be unchanged, a recorded digest is never overwritten or
-- cleared, and DELETE is still rejected by its own trigger.
--
-- Forward-only in production (ADR 0038, ADR 0070). The down migration drops the
-- column and restores the original trigger function.
ALTER TABLE public.source_content_revisions
    ADD COLUMN document_digest text,
    ADD CONSTRAINT source_content_revisions_document_digest_check
        CHECK (document_digest IS NULL OR document_digest ~ '^[0-9a-f]{32}$');

CREATE INDEX source_content_revisions_owner_document_digest_idx
    ON public.source_content_revisions (owner_id, document_digest);

CREATE OR REPLACE FUNCTION public.reject_source_content_revision_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP = 'UPDATE'
    AND OLD.document_digest IS NULL
    AND NEW.document_digest IS NOT NULL
    AND NEW.revision_id = OLD.revision_id
    AND NEW.owner_id = OLD.owner_id
    AND NEW.source_material_id = OLD.source_material_id
    AND NEW.digest_version = OLD.digest_version
    AND NEW.content_digest = OLD.content_digest
    AND NEW.content = OLD.content
    AND NEW.full_text = OLD.full_text
    AND NEW.created_at = OLD.created_at THEN
  RETURN NEW;
 END IF;
 RAISE EXCEPTION 'source content revisions are immutable';
END;
$$;
