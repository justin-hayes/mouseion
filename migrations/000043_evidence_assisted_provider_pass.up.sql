-- Evidence-assisted provider pass on Ancient Greek deck specifications: the
-- deck specification records whether a Greek preparation opts into the
-- provider pass that adds a labelled contextual Gloss, sentence translation
-- and English target alignment on top of the source-evidence card (ADR 0101,
-- ticket #2171).
--
-- The pass is off by default and is offered only while a matching provider
-- payload assessment is recorded, so the additive column defaults to false and
-- every existing specification keeps its providerless, source-evidence deck.
-- The choice is a deck-specification fact, not a run fact: it survives
-- re-preparation through the existing roll-forward (ADR 0076).

ALTER TABLE public.deck_preparations
    ADD COLUMN evidence_assisted_provider_pass boolean NOT NULL DEFAULT false;
