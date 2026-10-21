-- Reverse 000045_source_structure_v2. Dropping the structure tables discards
-- every recorded v2 role span, heading reset and structure check. Existing v2
-- snapshot rows are deliberately left in place, so a downgraded v1 reader will
-- treat them as an unsupported contract rather than silently reading them as
-- text-only v1 evidence. As with every down migration here, this is not assumed
-- safe for destructive production rollback; ADR 0103 unsupports downgrade after
-- v2 adoption, and recovery is retry and forward fixes.

DROP TRIGGER IF EXISTS source_structure_checks_immutable ON public.source_structure_checks;
DROP TRIGGER IF EXISTS source_material_unit_heading_resets_immutable ON public.source_material_unit_heading_resets;
DROP TRIGGER IF EXISTS source_material_unit_role_spans_immutable ON public.source_material_unit_role_spans;
DROP TRIGGER IF EXISTS source_material_unit_heading_resets_validate ON public.source_material_unit_heading_resets;
DROP TRIGGER IF EXISTS source_material_unit_role_spans_validate ON public.source_material_unit_role_spans;

DROP TABLE IF EXISTS public.source_structure_checks;
DROP TABLE IF EXISTS public.source_material_unit_heading_resets;
DROP TABLE IF EXISTS public.source_material_unit_role_spans;

DROP FUNCTION IF EXISTS public.validate_source_material_unit_heading_reset();
DROP FUNCTION IF EXISTS public.validate_source_material_unit_role_span();
DROP FUNCTION IF EXISTS public.reject_source_structure_mutation();

DROP INDEX IF EXISTS public.source_material_unit_snapshots_v2_extraction_identity;

ALTER TABLE public.source_material_unit_snapshots
    DROP CONSTRAINT IF EXISTS source_material_unit_snapshots_revision_identity;

ALTER TABLE public.source_material_unit_snapshots
    DROP CONSTRAINT IF EXISTS source_material_unit_snapshots_structure_convention_check;

ALTER TABLE public.source_material_unit_snapshots
    DROP COLUMN IF EXISTS structure_convention_set;
