CREATE UNIQUE INDEX deck_preparations_one_current_per_book
  ON deck_preparations(owner_id, book_id)
  WHERE book_id IS NOT NULL AND retired_at IS NULL;
