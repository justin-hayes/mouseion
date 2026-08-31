CREATE TABLE primary_goals (
 owner_id   uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 book_id    uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (owner_id, book_id) REFERENCES books(owner_id, id) ON DELETE CASCADE
);

CREATE INDEX primary_goals_book_idx ON primary_goals(book_id);
