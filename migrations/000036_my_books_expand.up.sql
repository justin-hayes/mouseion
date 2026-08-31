-- 000036 My Books expand (ADR 0035): additive, no shipped migration edits.
-- Governance (#449): shipped 000001..000035 immutable; this is expand (new tables + nullable FK).
-- Backfill in 000037 is idempotent/retry-safe; expand is safe to re-run via IF NOT EXISTS semantics where used.
-- Transaction: each migration file runs in its own transaction; failure rolls back this file only.
-- Locking/failure recovery documented in 000037; rollback/forward-fix: down drops new tables/column.
CREATE TABLE books (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 title text NOT NULL CHECK (title <> ''),
 metadata_provenance text NOT NULL,
 language_state text NOT NULL CHECK (language_state IN ('unknown','chosen')),
 language_tag text,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, id),
 CHECK (
   (language_state = 'chosen' AND language_tag IS NOT NULL AND language_tag <> '') OR
   (language_state = 'unknown' AND language_tag IS NULL)
 )
);

CREATE TABLE book_membership (
 owner_id uuid NOT NULL,
 book_id uuid NOT NULL,
 state text NOT NULL CHECK (state IN ('active','removed')),
 created_at timestamptz NOT NULL DEFAULT now(),
 activated_at timestamptz,
 removed_at timestamptz,
 PRIMARY KEY(owner_id, book_id),
 FOREIGN KEY(owner_id, book_id) REFERENCES books(owner_id, id) ON DELETE CASCADE,
 CHECK ((state = 'active') = (removed_at IS NULL))
);

CREATE TABLE book_aliases (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL,
 book_id uuid NOT NULL,
 alias_type text NOT NULL CHECK (alias_type IN ('catalog_entry','strong_bibliographic')),
 namespace text NOT NULL,
 value text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, namespace, value),
 FOREIGN KEY(owner_id, book_id) REFERENCES books(owner_id, id) ON DELETE CASCADE
);

ALTER TABLE source_materials
 ADD COLUMN book_id uuid,
 ADD CONSTRAINT source_materials_book_fkey
  FOREIGN KEY(owner_id, book_id) REFERENCES books(owner_id, id) ON DELETE SET NULL;

CREATE INDEX source_materials_owner_book_idx ON source_materials(owner_id, book_id);
