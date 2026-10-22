package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// StructureConventionSet identifies the initial versioned markup convention set
// whose roles a v2 snapshot records (ADR 0103). The identity is stored with the
// snapshot so a later convention set is a different extraction, never an
// in-place reinterpretation of old spans.
const StructureConventionSet = "epub-structure/1"

// SourceStructureRole is the structural role a convention set resolves from
// markup. A role is never a guessed speaker identity, and a heading is not a
// third vocabulary role.
type SourceStructureRole string

const (
	SourceStructureRoleSpeaker   SourceStructureRole = "speaker"
	SourceStructureRoleDirection SourceStructureRole = "stage_direction"
)

// Valid reports whether the role is part of the stored role vocabulary.
func (r SourceStructureRole) Valid() bool {
	return r == SourceStructureRoleSpeaker || r == SourceStructureRoleDirection
}

// UnitRoleSpan is one half-open Unicode-code-point span within the exact text of
// the named unit in the exact snapshot that owns it.
type UnitRoleSpan struct {
	UnitID      string
	Role        SourceStructureRole
	StartOffset uint64
	EndOffset   uint64
}

// UnitHeadingReset is one heading-reset offset within the exact text of the
// named unit in the exact snapshot that owns it.
type UnitHeadingReset struct {
	UnitID string
	Offset uint64
}

// SnapshotStructure is the structure a snapshot records, kept separate from its
// units so a v1 reader can say "structure not recorded" instead of inferring
// prose, speakers or source lines from text.
type SnapshotStructure struct {
	Recorded      bool
	ConventionSet string
	RoleSpans     []UnitRoleSpan
	HeadingResets []UnitHeadingReset
}

// ExtractedSnapshot pairs one immutable snapshot identity and content revision
// with its units and recorded structure. References always resolve through this
// exact snapshot; unit IDs may recur across snapshots.
type ExtractedSnapshot struct {
	SnapshotID        string
	ContentRevisionID string
	Units             ExtractedUnits
	Structure         SnapshotStructure
}

// SourceStructureOutcome is the successful outcome of a source structure check.
type SourceStructureOutcome string

const (
	SourceStructureUnchanged           SourceStructureOutcome = "unchanged"
	SourceStructureUpdateAvailable     SourceStructureOutcome = "update_available"
	SourceStructureOriginalUnavailable SourceStructureOutcome = "original_unavailable"
)

// Valid reports whether the outcome is one a successful check can record.
func (o SourceStructureOutcome) Valid() bool {
	return o == SourceStructureUnchanged || o == SourceStructureUpdateAvailable || o == SourceStructureOriginalUnavailable
}

// SourceStructureCheckKey identifies one durable structure check. The key
// includes the exact base revision and snapshot, target contract and convention
// set, so a retry or double confirmation converges on the same record.
type SourceStructureCheckKey struct {
	OwnerID               string
	SourceMaterialID      string
	ContentRevisionID     string
	SnapshotID            string
	TargetContractVersion int
	ConventionSet         string
}

// SourceStructureCheck is the durable, owner/source-scoped record of one
// successful check for a base revision and snapshot, target contract and
// convention set. ResultIdentity deterministically identifies the checked
// extraction so a later confirmation can verify the offered result.
type SourceStructureCheck struct {
	OwnerID               string
	SourceMaterialID      string
	ContentRevisionID     string
	SnapshotID            string
	TargetContractVersion int
	ConventionSet         string
	Outcome               SourceStructureOutcome
	ResultIdentity        string
	CheckedAt             time.Time
}

// Key returns the exact identity of the check.
func (c SourceStructureCheck) Key() SourceStructureCheckKey {
	return SourceStructureCheckKey{
		OwnerID: c.OwnerID, SourceMaterialID: c.SourceMaterialID,
		ContentRevisionID: c.ContentRevisionID, SnapshotID: c.SnapshotID,
		TargetContractVersion: c.TargetContractVersion, ConventionSet: c.ConventionSet,
	}
}

// Validate checks that structure references the snapshot's exact units, stays
// inside their text, uses known roles and never contradicts itself. A v1
// snapshot must carry no structure at all: empty role data means structure was
// not recorded, not that the source was prose or had no speakers.
func (s SnapshotStructure) Validate(schemaVersion int, units []ExtractedUnit) error {
	if schemaVersion < ExtractedUnitsSchemaVersionV2 {
		return s.validateNotRecorded()
	}
	if err := s.validateRecorded(units); err != nil {
		return err
	}
	return validateRoleSpanLayout(s.RoleSpans)
}

// validateNotRecorded requires a v1 snapshot to carry no structure at all.
func (s SnapshotStructure) validateNotRecorded() error {
	if s.Recorded || s.ConventionSet != "" || len(s.RoleSpans) > 0 || len(s.HeadingResets) > 0 {
		return errors.New("epub: v1 snapshot must not record source structure")
	}
	return nil
}

// validateRecorded checks a v2 snapshot's convention set and every reference
// against the exact units it owns.
func (s SnapshotStructure) validateRecorded(units []ExtractedUnit) error {
	if !s.Recorded {
		return errors.New("epub: v2 snapshot must record source structure")
	}
	if strings.TrimSpace(s.ConventionSet) == "" {
		return errors.New("epub: v2 snapshot must name its structure convention set")
	}
	unitText := make(map[string][]rune, len(units))
	for _, unit := range units {
		unitText[unit.ID] = []rune(unit.Text)
	}
	for _, span := range s.RoleSpans {
		if err := validateUnitRoleSpan(span, unitText); err != nil {
			return err
		}
	}
	for _, reset := range s.HeadingResets {
		if err := validateUnitHeadingReset(reset, unitText); err != nil {
			return err
		}
	}
	return nil
}

// validateUnitHeadingReset checks one heading reset against the exact unit text.
func validateUnitHeadingReset(reset UnitHeadingReset, unitText map[string][]rune) error {
	text, ok := unitText[reset.UnitID]
	if !ok {
		return fmt.Errorf("epub: heading reset references unit %q outside the snapshot", reset.UnitID)
	}
	if reset.Offset > uint64(len(text)) {
		return fmt.Errorf("epub: heading reset for unit %q is outside its text", reset.UnitID)
	}
	return nil
}

// validateUnitRoleSpan checks one role span's vocabulary and bounds against the
// exact unit text.
func validateUnitRoleSpan(span UnitRoleSpan, unitText map[string][]rune) error {
	if !span.Role.Valid() {
		return fmt.Errorf("epub: unknown source structure role %q", span.Role)
	}
	text, ok := unitText[span.UnitID]
	if !ok {
		return fmt.Errorf("epub: role span references unit %q outside the snapshot", span.UnitID)
	}
	if span.EndOffset <= span.StartOffset || span.EndOffset > uint64(len(text)) {
		return fmt.Errorf("epub: role span for unit %q is outside its text", span.UnitID)
	}
	return nil
}

// validateRoleSpanLayout accepts disjoint spans and proper nesting. It rejects
// crossing spans, duplicate role spans and contradictory nesting, including the
// same bounds carrying two roles or a speaker label wrapped in a direction. A
// direction nested inside a speaker label remains valid.
func validateRoleSpanLayout(spans []UnitRoleSpan) error {
	for i, first := range spans {
		for _, second := range spans[i+1:] {
			if first.UnitID != second.UnitID {
				continue
			}
			if err := validateRoleSpanPair(first, second); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateRoleSpanPair checks one pair of spans in the same unit.
func validateRoleSpanPair(first, second UnitRoleSpan) error {
	if first.StartOffset == second.StartOffset && first.EndOffset == second.EndOffset {
		if first.Role != second.Role {
			return fmt.Errorf("epub: unit %q has contradictory role spans over the same text", first.UnitID)
		}
		return fmt.Errorf("epub: unit %q has a duplicate role span", first.UnitID)
	}
	if first.StartOffset >= second.EndOffset || second.StartOffset >= first.EndOffset {
		return nil
	}
	outer, inner := first, second
	if second.StartOffset <= first.StartOffset && second.EndOffset >= first.EndOffset {
		outer, inner = second, first
	}
	if outer.StartOffset > inner.StartOffset || outer.EndOffset < inner.EndOffset {
		return fmt.Errorf("epub: unit %q has role spans that overlap without nesting", first.UnitID)
	}
	if outer.Role == SourceStructureRoleDirection && inner.Role == SourceStructureRoleSpeaker {
		return fmt.Errorf("epub: unit %q nests a speaker role inside a direction", first.UnitID)
	}
	return nil
}
