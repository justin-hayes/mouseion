-- Reverse 000043_evidence_assisted_provider_pass. Dropping the column returns
-- every specification to the providerless default. As with every down
-- migration here, this is not assumed safe for destructive production
-- rollback: a specification that had opted into the pass would lose the choice.

ALTER TABLE public.deck_preparations
    DROP COLUMN evidence_assisted_provider_pass;
