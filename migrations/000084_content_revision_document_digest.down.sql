CREATE OR REPLACE FUNCTION public.reject_source_content_revision_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'source content revisions are immutable';
END;
$$;

DROP INDEX public.source_content_revisions_owner_document_digest_idx;

ALTER TABLE public.source_content_revisions
    DROP CONSTRAINT source_content_revisions_document_digest_check,
    DROP COLUMN document_digest;
