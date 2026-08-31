CREATE TABLE reading_journeys (
  owner_id   uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  revision   bigint NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE reading_journey_membership (
  owner_id   uuid NOT NULL,
  book_id    uuid NOT NULL,
  position   integer NOT NULL CHECK (position >= 1),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, book_id),
  FOREIGN KEY (owner_id) REFERENCES reading_journeys(owner_id) ON DELETE CASCADE,
  FOREIGN KEY (owner_id, book_id) REFERENCES books(owner_id, id) ON DELETE CASCADE
);

CREATE INDEX reading_journey_membership_order_idx
  ON reading_journey_membership(owner_id, position, created_at, book_id);
