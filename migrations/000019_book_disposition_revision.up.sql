ALTER TABLE public.book_dispositions
    ADD COLUMN revision bigint NOT NULL DEFAULT 1;
