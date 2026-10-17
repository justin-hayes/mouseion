-- Reverse 000041_lsj_dictionary_entry. Both frozen LSJ columns are dropped.
-- Every pre-LSJ row keeps the empty string, so collapsing the columns cannot
-- change a modern-language deck's identity. As with every down migration here,
-- this is not assumed safe for destructive production rollback: an LSJ-era
-- Greek deck would lose its frozen entry id and pin.

ALTER TABLE public.deck_preparation_manifest_items
    DROP COLUMN lsj_pin,
    DROP COLUMN lsj_entry_id;
