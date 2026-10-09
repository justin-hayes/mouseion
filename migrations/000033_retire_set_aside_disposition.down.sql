ALTER TABLE public.book_dispositions
    DROP CONSTRAINT book_dispositions_disposition_check;
ALTER TABLE public.book_dispositions
    ADD CONSTRAINT book_dispositions_disposition_check
    CHECK (disposition = ANY (ARRAY['inbox'::text, 'to_read'::text, 'set_aside'::text]));
