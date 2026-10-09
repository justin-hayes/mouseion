-- Retire the Set Aside disposition (ADR 0086). No production Book was ever
-- Set Aside, so no data conversion is needed: the constraint is simply
-- tightened. If a legacy row exists anywhere, this fails loudly rather than
-- converting or repairing it silently.
ALTER TABLE public.book_dispositions
    DROP CONSTRAINT book_dispositions_disposition_check;
ALTER TABLE public.book_dispositions
    ADD CONSTRAINT book_dispositions_disposition_check
    CHECK (disposition = ANY (ARRAY['inbox'::text, 'to_read'::text]));
