package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const EPUBReviewedScopeSchemaVersion = 1

var ErrEPUBReviewedScopeUnavailable = errors.New("epub: reviewed scope unavailable")

type EPUBScopeSelectionMode string

const (
	EPUBScopeSelectionRecommended EPUBScopeSelectionMode = "recommended"
	EPUBScopeSelectionOverridden  EPUBScopeSelectionMode = "overridden"
)

// EPUBReviewedScopeSnapshot is one immutable learner confirmation. ScopeID
// identifies this confirmation, not a mutable selection for a source. A later
// review or reanalysis must create another value with a new ScopeID.
//
// SelectedUnits deliberately contains identifiers and order metadata only.
// Consumers must reload unit text from the persisted SourceUnitSnapshot.
type EPUBReviewedScopeSnapshot struct {
	SchemaVersion      int                         `json:"schema_version"`
	ScopeID            string                      `json:"scope_id"`
	OwnerID            string                      `json:"owner_id"`
	SourceMaterialID   string                      `json:"source_material_id"`
	SourceUnitSnapshot EPUBUnitSnapshotIdentity    `json:"source_unit_snapshot"`
	Classifier         EPUBClassifierIdentity      `json:"classifier"`
	SelectionMode      EPUBScopeSelectionMode      `json:"selection_mode"`
	SelectedUnits      []EPUBSelectedUnitReference `json:"selected_units"`
	CreatedAt          time.Time                   `json:"-"`
}

// EPUBUnitSnapshotIdentity identifies the complete extracted-unit envelope.
// It is separate from EPUBSourceUnitSnapshotIdentity, which additionally
// identifies one classification's unit.
type EPUBUnitSnapshotIdentity struct {
	SnapshotID                  string `json:"snapshot_id"`
	ExtractedUnitsSchemaVersion int    `json:"extracted_units_schema_version"`
}

// EPUBSelectedUnitReference records deterministic source order without
// accepting browser-provided text.
type EPUBSelectedUnitReference struct {
	UnitID string `json:"unit_id"`
	Order  uint64 `json:"order"`
}

// EPUBScopeSourceSnapshot is the trusted, owner-scoped persisted snapshot used
// to validate a proposed reviewed scope. It is not serialized as browser input.
type EPUBScopeSourceSnapshot struct {
	OwnerID          string
	SourceMaterialID string
	SnapshotID       string
	ExtractedUnits   ExtractedUnits
}

// ValidateAgainst validates the contract and binds every selected reference
// to an exact trusted source snapshot. This is also the boundary at which the
// server proves readability; later consumers reload text from the same rows.
func (s EPUBReviewedScopeSnapshot) ValidateAgainst(source EPUBScopeSourceSnapshot) error {
	if source.SnapshotID == "" || source.ExtractedUnits.SchemaVersion == 0 || len(source.ExtractedUnits.Units) == 0 {
		return ErrEPUBReviewedScopeUnavailable
	}
	if s.SchemaVersion != EPUBReviewedScopeSchemaVersion {
		return fmt.Errorf("epub: unsupported reviewed-scope schema version %d", s.SchemaVersion)
	}
	if !stableIdentity(s.ScopeID) {
		return fmt.Errorf("epub: reviewed scope identity is required and must be stable")
	}
	if !stableIdentity(s.OwnerID) || !stableIdentity(s.SourceMaterialID) {
		return fmt.Errorf("epub: reviewed scope owner and source material identities are required and must be stable")
	}
	if s.OwnerID != source.OwnerID || s.SourceMaterialID != source.SourceMaterialID {
		return fmt.Errorf("epub: reviewed scope does not belong to the source owner and material")
	}
	if s.SourceUnitSnapshot.SnapshotID != source.SnapshotID || s.SourceUnitSnapshot.ExtractedUnitsSchemaVersion != source.ExtractedUnits.SchemaVersion {
		return fmt.Errorf("epub: reviewed scope does not reference the exact extracted-unit snapshot")
	}
	if !stableIdentity(s.SourceUnitSnapshot.SnapshotID) {
		return fmt.Errorf("epub: source snapshot identity is required and must be stable")
	}
	if source.ExtractedUnits.SchemaVersion != ExtractedUnitsSchemaVersion {
		return fmt.Errorf("epub: unsupported source extracted-unit schema version %d", source.ExtractedUnits.SchemaVersion)
	}
	if !stableToken(s.Classifier.Name) || !stableToken(s.Classifier.Version) {
		return fmt.Errorf("epub: classifier name and version are required stable tokens")
	}
	if s.SelectionMode != EPUBScopeSelectionRecommended && s.SelectionMode != EPUBScopeSelectionOverridden {
		return fmt.Errorf("epub: invalid scope selection mode %q", s.SelectionMode)
	}
	if len(s.SelectedUnits) == 0 {
		return fmt.Errorf("epub: reviewed scope must select at least one readable unit")
	}

	units := make(map[string]ExtractedUnit, len(source.ExtractedUnits.Units))
	for _, unit := range source.ExtractedUnits.Units {
		units[unit.ID] = unit
	}
	seen := make(map[string]struct{}, len(s.SelectedUnits))
	var previousOrder uint64
	for i, selected := range s.SelectedUnits {
		if !stableIdentity(selected.UnitID) {
			return fmt.Errorf("epub: selected unit %d has an invalid identity", i)
		}
		if _, exists := seen[selected.UnitID]; exists {
			return fmt.Errorf("epub: duplicate selected unit identity %q", selected.UnitID)
		}
		seen[selected.UnitID] = struct{}{}
		unit, exists := units[selected.UnitID]
		if !exists {
			return fmt.Errorf("epub: selected unit %q is not in the referenced snapshot", selected.UnitID)
		}
		if strings.TrimSpace(unit.Text) == "" {
			return fmt.Errorf("epub: selected unit %q is not readable", selected.UnitID)
		}
		if selected.Order != unit.Order {
			return fmt.Errorf("epub: selected unit %q order %d does not match snapshot order %d", selected.UnitID, selected.Order, unit.Order)
		}
		if i > 0 && selected.Order <= previousOrder {
			return fmt.Errorf("epub: selected units are not in stable snapshot order")
		}
		previousOrder = selected.Order
	}
	return nil
}

// MarshalJSON preserves fixed field order and rejects structurally invalid
// snapshots that have not yet been checked against trusted persisted units.
func (s EPUBReviewedScopeSnapshot) MarshalJSON() ([]byte, error) {
	if err := s.validateEnvelope(); err != nil {
		return nil, err
	}
	type wire EPUBReviewedScopeSnapshot
	return json.Marshal(wire(s))
}

func (s EPUBReviewedScopeSnapshot) validateEnvelope() error {
	if s.SchemaVersion != EPUBReviewedScopeSchemaVersion {
		return fmt.Errorf("epub: unsupported reviewed-scope schema version %d", s.SchemaVersion)
	}
	if !stableIdentity(s.ScopeID) || !stableIdentity(s.OwnerID) || !stableIdentity(s.SourceMaterialID) || !stableIdentity(s.SourceUnitSnapshot.SnapshotID) {
		return fmt.Errorf("epub: reviewed scope identities are required and must be stable")
	}
	if s.SourceUnitSnapshot.ExtractedUnitsSchemaVersion != ExtractedUnitsSchemaVersion {
		return fmt.Errorf("epub: unsupported source extracted-unit schema version %d", s.SourceUnitSnapshot.ExtractedUnitsSchemaVersion)
	}
	if !stableToken(s.Classifier.Name) || !stableToken(s.Classifier.Version) {
		return fmt.Errorf("epub: classifier name and version are required stable tokens")
	}
	if s.SelectionMode != EPUBScopeSelectionRecommended && s.SelectionMode != EPUBScopeSelectionOverridden {
		return fmt.Errorf("epub: invalid scope selection mode %q", s.SelectionMode)
	}
	if len(s.SelectedUnits) == 0 {
		return fmt.Errorf("epub: reviewed scope must select at least one readable unit")
	}
	seen := make(map[string]struct{}, len(s.SelectedUnits))
	var previousOrder uint64
	for i, selected := range s.SelectedUnits {
		if !stableIdentity(selected.UnitID) {
			return fmt.Errorf("epub: selected unit %d has an invalid identity", i)
		}
		if _, exists := seen[selected.UnitID]; exists {
			return fmt.Errorf("epub: duplicate selected unit identity %q", selected.UnitID)
		}
		seen[selected.UnitID] = struct{}{}
		if i > 0 && selected.Order <= previousOrder {
			return fmt.Errorf("epub: selected units are not in stable snapshot order")
		}
		previousOrder = selected.Order
	}
	return nil
}
