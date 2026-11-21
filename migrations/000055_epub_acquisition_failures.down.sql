-- Development-only rollback convenience (ADR 0038, ADR 0070). Production
-- migrations are forward-only; this down is not a supported production
-- rollback.
DROP TABLE public.epub_acquisition_failures;

ALTER TABLE public.opds_connections
    DROP CONSTRAINT opds_connections_owner_id_id_key;
