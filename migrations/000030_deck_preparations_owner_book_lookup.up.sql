CREATE INDEX deck_preparations_owner_book_state
    ON public.deck_preparations(owner_id, book_id, state)
    WHERE book_id IS NOT NULL;
