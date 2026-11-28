DROP TRIGGER source_content_revisions_immutable ON public.source_content_revisions;
CREATE TRIGGER source_content_revisions_immutable BEFORE UPDATE ON public.source_content_revisions
    FOR EACH ROW EXECUTE FUNCTION public.reject_source_content_revision_mutation();
DROP FUNCTION public.reject_source_content_revision_change();

DROP INDEX public.source_content_revisions_owner_document_digest_idx;

ALTER TABLE public.source_content_revisions
    DROP CONSTRAINT source_content_revisions_document_digest_check,
    DROP COLUMN document_digest;
