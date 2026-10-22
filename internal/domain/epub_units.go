package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// ExtractedUnitsSchemaVersionV1 is the historical extraction contract. It
	// carries text only; structure is not recorded, which is not the same as
	// prose or the absence of speakers.
	ExtractedUnitsSchemaVersionV1 = 1
	// ExtractedUnitsSchemaVersionV2 records immutable per-unit roles and
	// heading resets alongside the same unit text.
	ExtractedUnitsSchemaVersionV2 = 2
	// ExtractedUnitsSchemaVersion is the default writer version. A v2 extraction
	// exists without being the default; readers accept v1 and v2 independently.
	ExtractedUnitsSchemaVersion = ExtractedUnitsSchemaVersionV1
)

var ErrExtractedUnitsUnavailable = errors.New("epub: extracted units unavailable")

const (
	UnitTitleHeading    = "heading"
	UnitTitleManifestID = "manifest_id"
)

type ExtractedUnits struct {
	SchemaVersion int             `json:"schema_version"`
	Units         []ExtractedUnit `json:"units"`
}
type ExtractedUnit struct {
	ID               string   `json:"id"`
	Order            uint64   `json:"order"`
	SpineIndex       uint64   `json:"spine_index"`
	Title            string   `json:"title"`
	TitleSource      string   `json:"title_source"`
	Text             string   `json:"text"`
	StartOffset      uint64   `json:"start_offset"`
	EndOffset        uint64   `json:"end_offset"`
	PackagePath      string   `json:"package_path"`
	ManifestID       string   `json:"manifest_id"`
	SourceHref       string   `json:"source_href"`
	ResolvedHref     string   `json:"resolved_href"`
	MediaType        string   `json:"media_type"`
	Properties       []string `json:"properties"`
	Linear           bool     `json:"linear"`
	NavigationLabels []string `json:"navigation_labels"`
	LandmarkTypes    []string `json:"landmark_types"`
}

func EPUBUnitID(spineIndex uint64, manifestID string) string {
	return "epub-unit-v1:" + strconv.FormatUint(spineIndex, 10) + ":" + manifestID
}

// ClassicalUnitID is the unit identity of an operator-imported classical text.
// One imported Reading unit is carried by one unit, so there is no EPUB spine
// index; the definition key names the Reading unit.
func ClassicalUnitID(definitionKey string) string {
	return "classical-unit-v1:" + definitionKey
}

// ValidateOffsets checks that e is a self-consistent snapshot of fullText: a
// supported schema version, at least one unit, and a contiguous sequence of
// units whose identity, offsets, separator and text all agree with fullText.
func (e ExtractedUnits) ValidateOffsets(fullText string) error {
	if err := e.validateSchema(); err != nil {
		return err
	}
	runes := []rune(fullText)
	seen := make(map[string]struct{}, len(e.Units))
	var previousEnd uint64
	for index, unit := range e.Units {
		if err := validateExtractedUnit(uint64(index), unit, previousEnd, runes, seen); err != nil {
			return err
		}
		previousEnd = unit.EndOffset
	}
	if previousEnd != uint64(len(runes)) {
		return errors.New("epub: final unit does not end at FullText length")
	}
	return nil
}

// validateSchema checks that e carries a supported, non-empty snapshot.
func (e ExtractedUnits) validateSchema() error {
	if e.SchemaVersion == 0 {
		return ErrExtractedUnitsUnavailable
	}
	if e.SchemaVersion != ExtractedUnitsSchemaVersionV1 && e.SchemaVersion != ExtractedUnitsSchemaVersionV2 {
		return fmt.Errorf("epub: unsupported extracted-unit schema version %d", e.SchemaVersion)
	}
	if len(e.Units) == 0 {
		return ErrExtractedUnitsUnavailable
	}
	return nil
}

// ReconstructText joins this snapshot's ordered unit texts with the established
// unit separator. A content revision's historical FullText belongs to a
// different extraction and cannot validate a later snapshot.
func (e ExtractedUnits) ReconstructText() string {
	var text strings.Builder
	for index, unit := range e.Units {
		if index > 0 {
			text.WriteString("\n\n")
		}
		text.WriteString(unit.Text)
	}
	return text.String()
}

// ValidateSnapshot checks that the snapshot's units agree with the text
// reconstructed from those same units. Readers use it instead of validating
// against a content revision's FullText.
func (e ExtractedUnits) ValidateSnapshot() error {
	return e.ValidateOffsets(e.ReconstructText())
}

// validateExtractedUnit checks one unit's order, identity, offsets and text
// against fullText, recording its identity in seen. index is the unit's
// zero-based position and previousEnd the preceding unit's end offset, used to
// verify the FullText separator.
func validateExtractedUnit(index uint64, unit ExtractedUnit, previousEnd uint64, runes []rune, seen map[string]struct{}) error {
	if unit.Order != index {
		return fmt.Errorf("epub: unit %d has non-contiguous order %d", index, unit.Order)
	}
	if unit.ID != EPUBUnitID(unit.SpineIndex, unit.ManifestID) && unit.ID != ClassicalUnitID(unit.ManifestID) {
		return fmt.Errorf("epub: unit %d has invalid identity", index)
	}
	if _, ok := seen[unit.ID]; ok {
		return fmt.Errorf("epub: duplicate unit identity %q", unit.ID)
	}
	seen[unit.ID] = struct{}{}
	if unit.EndOffset < unit.StartOffset || unit.EndOffset > uint64(len(runes)) {
		return fmt.Errorf("epub: unit %d has invalid offsets", index)
	}
	if index == 0 && unit.StartOffset != 0 {
		return fmt.Errorf("epub: first unit starts at offset %d", unit.StartOffset)
	}
	if index > 0 && unit.StartOffset != previousEnd+2 {
		return fmt.Errorf("epub: unit %d does not preserve the FullText separator", index)
	}
	if string(runes[unit.StartOffset:unit.EndOffset]) != unit.Text {
		return fmt.Errorf("epub: unit %d text does not match FullText span", index)
	}
	return nil
}
