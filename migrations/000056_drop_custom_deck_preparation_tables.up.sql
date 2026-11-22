-- The custom deck path was retired by ADR 0085 and its worker was removed, so
-- nothing reads or writes these tables any more. Issue #2514 is the separately
-- reviewed deletion plan that ADR 0085 and the reading-owned-book-vocabulary
-- feature contract require: its inventory found both tables empty on the only
-- deployment (2026-11-21), and the maintainer repeats that count check
-- immediately before merge.
--
-- Recovery: take the usual pre-migration database backup. Because both tables
-- are empty, the drop removes no rows and no APKG bytes; the down migration
-- recreates only the empty schema, so that backup is the sole recovery path for
-- anything the inventory missed.
--
-- DROP TABLE removes each table's indexes and constraints with it. There are no
-- data statements and no code changes here; the custom decks and their
-- identities are deliberately kept.
--
-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
DROP TABLE public.custom_vocabulary_deck_preparation_identities;
DROP TABLE public.custom_vocabulary_deck_preparations;
