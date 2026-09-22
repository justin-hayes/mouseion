DROP INDEX public.deck_preparations_analysis_identity;

CREATE UNIQUE INDEX deck_preparations_analysis_identity
    ON public.deck_preparations USING btree (owner_id, source_material_id, analysis_run_id)
    WHERE analysis_run_id IS NOT NULL;
