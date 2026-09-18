package domain

import (
	"errors"
	"fmt"
	"strconv"
)

const ExtractedUnitsSchemaVersion = 1

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

func (e ExtractedUnits) ValidateOffsets(fullText string) error {
	if e.SchemaVersion == 0 {
		return ErrExtractedUnitsUnavailable
	}
	if e.SchemaVersion != ExtractedUnitsSchemaVersion {
		return fmt.Errorf("epub: unsupported extracted-unit schema version %d", e.SchemaVersion)
	}
	if len(e.Units) == 0 {
		return ErrExtractedUnitsUnavailable
	}
	runes := []rune(fullText)
	seen := make(map[string]struct{}, len(e.Units))
	var previousEnd uint64
	for i, unit := range e.Units {
		if unit.Order != uint64(i) {
			return fmt.Errorf("epub: unit %d has non-contiguous order %d", i, unit.Order)
		}
		if unit.ID != EPUBUnitID(unit.SpineIndex, unit.ManifestID) {
			return fmt.Errorf("epub: unit %d has invalid identity", i)
		}
		if _, ok := seen[unit.ID]; ok {
			return fmt.Errorf("epub: duplicate unit identity %q", unit.ID)
		}
		seen[unit.ID] = struct{}{}
		if unit.EndOffset < unit.StartOffset || unit.EndOffset > uint64(len(runes)) {
			return fmt.Errorf("epub: unit %d has invalid offsets", i)
		}
		if i == 0 && unit.StartOffset != 0 {
			return fmt.Errorf("epub: first unit starts at offset %d", unit.StartOffset)
		}
		if i > 0 && unit.StartOffset != previousEnd+2 {
			return fmt.Errorf("epub: unit %d does not preserve the FullText separator", i)
		}
		if string(runes[unit.StartOffset:unit.EndOffset]) != unit.Text {
			return fmt.Errorf("epub: unit %d text does not match FullText span", i)
		}
		previousEnd = unit.EndOffset
	}
	if previousEnd != uint64(len(runes)) {
		return errors.New("epub: final unit does not end at FullText length")
	}
	return nil
}
