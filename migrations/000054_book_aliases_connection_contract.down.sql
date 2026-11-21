-- Development-only: PostgreSQL cannot un-validate a constraint, so recreate it
-- NOT VALID. The validation scan already proved every existing row satisfies it.
ALTER TABLE public.book_aliases
    DROP CONSTRAINT book_aliases_connection_contract;

ALTER TABLE public.book_aliases
    ADD CONSTRAINT book_aliases_connection_contract
    CHECK ((((alias_type = 'catalog_entry'::text) AND (connection_id IS NOT NULL))
        OR ((alias_type = 'strong_bibliographic'::text) AND (connection_id IS NULL))))
    NOT VALID;
