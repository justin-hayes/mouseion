-- Forward-only in production (ADR 0038, ADR 0070). Development convenience
-- only: dropping the installation-wide table also drops parsed translation
-- sections that 000051 cannot restore without the operator's pinned file.
DROP TABLE IF EXISTS public.reference_translations_v2;
