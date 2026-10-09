-- Not presumed safe for destructive production rollback (ADR 0070/0086): this
-- drops the conversion audit and restores the legacy three-value constraint.
-- Converted Inbox rows are not mapped back to Set Aside.
DROP TABLE public.book_disposition_cutover_changes;
DROP TABLE public.book_disposition_cutovers;
ALTER TABLE public.book_dispositions
    DROP CONSTRAINT book_dispositions_disposition_check;
ALTER TABLE public.book_dispositions
    ADD CONSTRAINT book_dispositions_disposition_check
    CHECK (disposition = ANY (ARRAY['inbox'::text, 'to_read'::text, 'set_aside'::text]));
