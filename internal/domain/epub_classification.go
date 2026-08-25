package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

const (
	EPUBClassificationSchemaVersion = 1
	EPUBConfidenceMaximum           = 100
	EPUBConfidenceLowMaximum        = 49
	EPUBConfidenceHighMinimum       = 80
)

type EPUBUnitCategory string

const (
	EPUBCategoryFrontMatter EPUBUnitCategory = "front_matter"
	EPUBCategoryMainMatter  EPUBUnitCategory = "main_matter"
	EPUBCategoryBackMatter  EPUBUnitCategory = "back_matter"
	EPUBCategoryUnknown     EPUBUnitCategory = "unknown"
)

// EPUBUnitClassification is the deterministic, versioned recommendation for
// one unit in an extracted-unit snapshot. Confidence measures only confidence
// in Category; RecommendedInclusion is the independently recorded policy
// result consumed by Phase 3.
type EPUBUnitClassification struct {
	SchemaVersion        int                            `json:"schema_version"`
	Classifier           EPUBClassifierIdentity         `json:"classifier"`
	SourceUnitSnapshot   EPUBSourceUnitSnapshotIdentity `json:"source_unit_snapshot"`
	Category             EPUBUnitCategory               `json:"category"`
	Confidence           uint8                          `json:"confidence"`
	Reasons              []EPUBClassificationReason     `json:"reasons"`
	RecommendedInclusion bool                           `json:"recommended_inclusion"`
}

type EPUBClassifierIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// EPUBSourceUnitSnapshotIdentity binds a result to one unit in one immutable
// source snapshot. SnapshotID is assigned by the snapshot producer; UnitID and
// ExtractedUnitsSchemaVersion prevent consumers from guessing either identity.
type EPUBSourceUnitSnapshotIdentity struct {
	SnapshotID                  string `json:"snapshot_id"`
	ExtractedUnitsSchemaVersion int    `json:"extracted_units_schema_version"`
	UnitID                      string `json:"unit_id"`
}

// EPUBClassificationReason is emitted in classifier precedence order. Signal
// is a stable machine-readable rule key; Message is its human-readable
// explanation. Consumers must preserve the array order.
type EPUBClassificationReason struct {
	Signal  string `json:"signal"`
	Message string `json:"message"`
}

func (c EPUBUnitClassification) Validate() error {
	if c.SchemaVersion != EPUBClassificationSchemaVersion {
		return fmt.Errorf("epub: unsupported classification schema version %d", c.SchemaVersion)
	}
	if !stableToken(c.Classifier.Name) || !stableToken(c.Classifier.Version) {
		return fmt.Errorf("epub: classifier name and version are required stable tokens")
	}
	if !stableIdentity(c.SourceUnitSnapshot.SnapshotID) {
		return fmt.Errorf("epub: source snapshot identity is required and must be stable")
	}
	if c.SourceUnitSnapshot.ExtractedUnitsSchemaVersion != ExtractedUnitsSchemaVersion {
		return fmt.Errorf("epub: unsupported source extracted-unit schema version %d", c.SourceUnitSnapshot.ExtractedUnitsSchemaVersion)
	}
	if !stableIdentity(c.SourceUnitSnapshot.UnitID) {
		return fmt.Errorf("epub: source unit identity is required and must be stable")
	}
	if !validEPUBCategory(c.Category) {
		return fmt.Errorf("epub: invalid classification category %q", c.Category)
	}
	if c.Confidence > EPUBConfidenceMaximum {
		return fmt.Errorf("epub: confidence %d is outside 0..%d", c.Confidence, EPUBConfidenceMaximum)
	}
	if c.Category == EPUBCategoryUnknown && c.Confidence > EPUBConfidenceLowMaximum {
		return fmt.Errorf("epub: unknown category confidence %d exceeds low-confidence maximum %d", c.Confidence, EPUBConfidenceLowMaximum)
	}
	if c.Confidence >= EPUBConfidenceHighMinimum {
		if c.Category == EPUBCategoryMainMatter && !c.RecommendedInclusion {
			return fmt.Errorf("epub: high-confidence main matter must be recommended for inclusion")
		}
		if (c.Category == EPUBCategoryFrontMatter || c.Category == EPUBCategoryBackMatter) && c.RecommendedInclusion {
			return fmt.Errorf("epub: high-confidence front or back matter must be recommended for exclusion")
		}
	}
	if c.Reasons == nil || len(c.Reasons) == 0 {
		return fmt.Errorf("epub: at least one ordered classification reason is required")
	}
	seen := make(map[string]struct{}, len(c.Reasons))
	for i, reason := range c.Reasons {
		if !stableToken(reason.Signal) {
			return fmt.Errorf("epub: reason %d has an unstable signal", i)
		}
		if !stableHumanText(reason.Message) {
			return fmt.Errorf("epub: reason %d has an unstable message", i)
		}
		if _, exists := seen[reason.Signal]; exists {
			return fmt.Errorf("epub: duplicate classification reason signal %q", reason.Signal)
		}
		seen[reason.Signal] = struct{}{}
	}
	return nil
}

// MarshalJSON rejects invalid contracts and otherwise uses struct field and
// reason slice order, making serialization stable for the same output.
func (c EPUBUnitClassification) MarshalJSON() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	type wire EPUBUnitClassification
	return json.Marshal(wire(c))
}

func validEPUBCategory(category EPUBUnitCategory) bool {
	switch category {
	case EPUBCategoryFrontMatter, EPUBCategoryMainMatter, EPUBCategoryBackMatter, EPUBCategoryUnknown:
		return true
	default:
		return false
	}
}

func stableToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func stableHumanText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func stableIdentity(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
