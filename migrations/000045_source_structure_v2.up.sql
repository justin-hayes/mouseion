-- Snapshot extraction contract v2 storage foundation (ADR 0103, ticket #2189).
--
-- This migration is purely additive. Existing v1 snapshots, units and frozen
-- evidence keep their rows and text; no backfill rewrites them, and the writer
-- still emits v1 until a later slice. The new tables only describe structure a
-- v2 extraction records, scoped to the exact owner/source/snapshot/unit that
-- owns it, so unit identities that recur across snapshots never resolve to the
-- wrong evidence.

-- The versioned markup convention-set identity that produced a snapshot's role
-- spans and heading resets. v1 never recorded structure, so its column stays
-- null; v2 must name the convention set.
ALTER TABLE public.source_material_unit_snapshots
    ADD COLUMN structure_convention_set text;

ALTER TABLE public.source_material_unit_snapshots
    ADD CONSTRAINT source_material_unit_snapshots_structure_convention_check
    CHECK (
        (schema_version < 2 AND structure_convention_set IS NULL)
        OR (schema_version >= 2 AND structure_convention_set IS NOT NULL
            AND btrim(structure_convention_set) <> '')
    );

-- A snapshot identity is not sufficient to bind its content revision on its
-- own, so expose the composite key that the durable check record references.
ALTER TABLE public.source_material_unit_snapshots
    ADD CONSTRAINT source_material_unit_snapshots_revision_identity
    UNIQUE (owner_id, source_material_id, snapshot_id, content_revision_id);

-- At most one v2 snapshot per content revision, extraction contract version and
-- convention set within an owner/source scope. Historical v1 snapshots are
-- deliberately outside the predicate: duplicate-shaped v1 rows stay readable and
-- are never deduplicated or rewritten.
CREATE UNIQUE INDEX source_material_unit_snapshots_v2_extraction_identity
    ON public.source_material_unit_snapshots (
        owner_id, source_material_id, content_revision_id, schema_version,
        structure_convention_set
    )
    WHERE schema_version >= 2;

-- Immutable, nestable role spans over one snapshot unit. Offsets are half-open
-- Unicode-code-point positions in that unit's exact text. The role is the
-- structural role resolved from the convention set, never a guessed speaker
-- identity or copied label text.
CREATE TABLE public.source_material_unit_role_spans (
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    unit_id text NOT NULL,
    role text NOT NULL,
    start_offset integer NOT NULL,
    end_offset integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT source_material_unit_role_spans_role_check
        CHECK (role IN ('speaker', 'stage_direction')),
    CONSTRAINT source_material_unit_role_spans_bounds_check
        CHECK (start_offset >= 0 AND end_offset > start_offset),
    PRIMARY KEY (owner_id, source_material_id, snapshot_id, unit_id, role, start_offset, end_offset)
);

-- Immutable per-unit heading-reset offsets. A heading resets derived speaker
-- attribution; it is not a third vocabulary structural role.
CREATE TABLE public.source_material_unit_heading_resets (
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    unit_id text NOT NULL,
    "offset" integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT source_material_unit_heading_resets_offset_check
        CHECK ("offset" >= 0),
    PRIMARY KEY (owner_id, source_material_id, snapshot_id, unit_id, "offset")
);

ALTER TABLE ONLY public.source_material_unit_role_spans
    ADD CONSTRAINT source_material_unit_role_spans_unit_fkey
    FOREIGN KEY (owner_id, source_material_id, snapshot_id, unit_id)
    REFERENCES public.source_material_units(owner_id, source_material_id, snapshot_id, unit_id)
    ON DELETE CASCADE;

ALTER TABLE ONLY public.source_material_unit_heading_resets
    ADD CONSTRAINT source_material_unit_heading_resets_unit_fkey
    FOREIGN KEY (owner_id, source_material_id, snapshot_id, unit_id)
    REFERENCES public.source_material_units(owner_id, source_material_id, snapshot_id, unit_id)
    ON DELETE CASCADE;

-- Resolve the exact unit text length for a structure reference, rejecting a
-- unit from another snapshot and a v1 contract. Both structure triggers share
-- this ownership and bounds precondition.
CREATE FUNCTION public.source_material_unit_structure_length(
    p_owner_id uuid, p_source_material_id uuid, p_snapshot_id uuid, p_unit_id text
) RETURNS integer
    LANGUAGE plpgsql
    AS $$
DECLARE
    unit_length integer;
    snapshot_schema integer;
BEGIN
    SELECT snap.schema_version, char_length(u.text)
      INTO snapshot_schema, unit_length
      FROM public.source_material_unit_snapshots snap
      JOIN public.source_material_units u
        ON u.owner_id = snap.owner_id
       AND u.source_material_id = snap.source_material_id
       AND u.snapshot_id = snap.snapshot_id
     WHERE snap.owner_id = p_owner_id
       AND snap.source_material_id = p_source_material_id
       AND snap.snapshot_id = p_snapshot_id
       AND u.unit_id = p_unit_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'structure reference unit does not belong to snapshot';
    END IF;
    IF snapshot_schema < 2 THEN
        RAISE EXCEPTION 'structure references require an extraction contract v2 snapshot';
    END IF;
    RETURN unit_length;
END;
$$;

-- Reject role spans that leave the unit text, cross another span without proper
-- nesting, or contradict another role by giving the same bounds two roles or by
-- wrapping a speaker label inside a direction. A direction nested inside a
-- speaker label is explicitly allowed.
CREATE FUNCTION public.validate_source_material_unit_role_span() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    unit_length integer;
BEGIN
    unit_length := public.source_material_unit_structure_length(
        NEW.owner_id, NEW.source_material_id, NEW.snapshot_id, NEW.unit_id);
    IF NEW.start_offset < 0 OR NEW.end_offset <= NEW.start_offset OR NEW.end_offset > unit_length THEN
        RAISE EXCEPTION 'role span offsets are outside the unit text';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM public.source_material_unit_role_spans span
         WHERE span.owner_id = NEW.owner_id
           AND span.source_material_id = NEW.source_material_id
           AND span.snapshot_id = NEW.snapshot_id
           AND span.unit_id = NEW.unit_id
           AND span.start_offset < NEW.end_offset
           AND span.end_offset > NEW.start_offset
           AND NOT (span.start_offset <= NEW.start_offset AND span.end_offset >= NEW.end_offset)
           AND NOT (NEW.start_offset <= span.start_offset AND NEW.end_offset >= span.end_offset)
    ) THEN
        RAISE EXCEPTION 'role spans overlap without nesting';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM public.source_material_unit_role_spans span
         WHERE span.owner_id = NEW.owner_id
           AND span.source_material_id = NEW.source_material_id
           AND span.snapshot_id = NEW.snapshot_id
           AND span.unit_id = NEW.unit_id
           AND span.role <> NEW.role
           AND span.start_offset = NEW.start_offset
           AND span.end_offset = NEW.end_offset
    ) THEN
        RAISE EXCEPTION 'role spans contradict with equal bounds';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM public.source_material_unit_role_spans span
         WHERE span.owner_id = NEW.owner_id
           AND span.source_material_id = NEW.source_material_id
           AND span.snapshot_id = NEW.snapshot_id
           AND span.unit_id = NEW.unit_id
           AND (
               (span.role = 'stage_direction' AND NEW.role = 'speaker'
                   AND span.start_offset <= NEW.start_offset AND span.end_offset >= NEW.end_offset)
               OR (span.role = 'speaker' AND NEW.role = 'stage_direction'
                   AND NEW.start_offset <= span.start_offset AND NEW.end_offset >= span.end_offset)
           )
    ) THEN
        RAISE EXCEPTION 'role spans contradict by nesting a speaker inside a direction';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER source_material_unit_role_spans_validate
    BEFORE INSERT ON public.source_material_unit_role_spans
    FOR EACH ROW EXECUTE FUNCTION public.validate_source_material_unit_role_span();

-- Heading resets must stay inside their exact unit and only exist on v2.
CREATE FUNCTION public.validate_source_material_unit_heading_reset() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    unit_length integer;
BEGIN
    unit_length := public.source_material_unit_structure_length(
        NEW.owner_id, NEW.source_material_id, NEW.snapshot_id, NEW.unit_id);
    IF NEW."offset" < 0 OR NEW."offset" > unit_length THEN
        RAISE EXCEPTION 'heading reset offset is outside the unit text';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER source_material_unit_heading_resets_validate
    BEFORE INSERT ON public.source_material_unit_heading_resets
    FOR EACH ROW EXECUTE FUNCTION public.validate_source_material_unit_heading_reset();

-- Durable, owner/source-scoped check record for adopting a v2 extraction. It
-- names the exact base revision and snapshot, the target contract and convention
-- set, the check outcome and the deterministic identity of the checked result.
-- A failed check is never a successful record; there is at most one successful
-- record for an exact key, so retries and double confirmation reuse it.
CREATE TABLE public.source_structure_checks (
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    content_revision_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    target_contract_version integer NOT NULL,
    structure_convention_set text NOT NULL,
    outcome text NOT NULL,
    result_identity text NOT NULL,
    checked_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT source_structure_checks_target_contract_check
        CHECK (target_contract_version >= 2),
    CONSTRAINT source_structure_checks_convention_check
        CHECK (btrim(structure_convention_set) <> ''),
    CONSTRAINT source_structure_checks_outcome_check
        CHECK (outcome IN ('unchanged', 'update_available', 'original_unavailable')),
    CONSTRAINT source_structure_checks_result_identity_check
        CHECK (btrim(result_identity) <> ''),
    PRIMARY KEY (
        owner_id, source_material_id, content_revision_id, snapshot_id,
        target_contract_version, structure_convention_set
    )
);

ALTER TABLE ONLY public.source_structure_checks
    ADD CONSTRAINT source_structure_checks_snapshot_fkey
    FOREIGN KEY (owner_id, source_material_id, snapshot_id, content_revision_id)
    REFERENCES public.source_material_unit_snapshots(owner_id, source_material_id, snapshot_id, content_revision_id)
    ON DELETE CASCADE;

CREATE FUNCTION public.reject_source_structure_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'source structure is immutable';
END;
$$;

CREATE TRIGGER source_material_unit_role_spans_immutable
    BEFORE UPDATE ON public.source_material_unit_role_spans
    FOR EACH ROW EXECUTE FUNCTION public.reject_source_structure_mutation();

CREATE TRIGGER source_material_unit_heading_resets_immutable
    BEFORE UPDATE ON public.source_material_unit_heading_resets
    FOR EACH ROW EXECUTE FUNCTION public.reject_source_structure_mutation();

CREATE TRIGGER source_structure_checks_immutable
    BEFORE UPDATE ON public.source_structure_checks
    FOR EACH ROW EXECUTE FUNCTION public.reject_source_structure_mutation();
