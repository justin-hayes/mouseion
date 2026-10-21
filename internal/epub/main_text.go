package epub

import (
	"strings"

	"github.com/justin-hayes/mouseion/internal/checked"
)

// DistributorConventionGutenberg identifies Project Gutenberg's published
// packaging convention (ADR 0108).
const DistributorConventionGutenberg = "project-gutenberg"

const gutenbergHeaderManifestID = "pg-header"

// MainTextSelection is the pure, structure-derived analysis scope decision.
// BodyMatterStart and BackMatterStart are zero-based positions in units.
// Identified distinguishes a valid no-op landmark decision from the fail-safe
// fallback; LandmarkApplies and Distributor record the two exclusion rules
// independently so a run's configuration identity can reproduce its scope.
// Applies is true only when the combined decision excludes at least one unit or
// trims Distributor text.
type MainTextSelection struct {
	Identified      bool
	Applies         bool
	BodyMatterStart int
	BackMatterStart int
	SelectedUnitIDs []string
	ExcludedUnitIDs []string

	LandmarkApplies bool
	Distributor     DistributorSelection
}

// DistributorSelection is the recognised Distributor text of a snapshot.
// ExcludedUnitIDs are the readable units removed whole and TrimmedRanges are the
// per-unit text ranges whose overlapping sentences are dropped.
type DistributorSelection struct {
	Recognised      bool
	Convention      string
	ExcludedUnitIDs []string
	TrimmedRanges   []UnitTextRange
}

// Excludes reports whether recognised Distributor text removes anything from
// the analysed text.
func (d DistributorSelection) Excludes() bool {
	return len(d.ExcludedUnitIDs) > 0 || len(d.TrimmedRanges) > 0
}

// UnitTextRange is a half-open span in Unicode code points inside one unit.
type UnitTextRange struct {
	UnitID      string `json:"unit_id"`
	StartOffset uint64 `json:"start_offset"`
	EndOffset   uint64 `json:"end_offset"`
}

var mainTextBackMatterLandmarks = map[string]struct{}{
	"backmatter":   {},
	"bibliography": {},
	"index":        {},
	"glossary":     {},
	"colophon":     {},
}

// IdentifyMainText identifies the declared main-text run and the recognised
// Distributor text in ordered readable units. Ambiguous declarations fall back
// to selecting the complete snapshot.
func IdentifyMainText(units []ExtractedUnit) MainTextSelection {
	landmark := identifyLandmarkMainText(units)
	distributor := identifyGutenbergDistributorText(units)
	return combineMainText(units, landmark, distributor)
}

// landmarkMainText is the ADR 0066 decision before Distributor text applies.
type landmarkMainText struct {
	identified           bool
	applies              bool
	bodyStart, backStart int
}

func identifyLandmarkMainText(units []ExtractedUnit) landmarkMainText {
	bodyMatterStart := -1
	for i, unit := range units {
		if !hasLandmark(unit.LandmarkTypes, "bodymatter") {
			continue
		}
		if bodyMatterStart >= 0 {
			return landmarkMainText{}
		}
		bodyMatterStart = i
	}
	if bodyMatterStart < 0 {
		return landmarkMainText{}
	}

	backMatterStart := len(units)
	for i := bodyMatterStart; i < len(units); i++ {
		if hasBackMatterLandmark(units[i].LandmarkTypes) {
			backMatterStart = i
			break
		}
	}
	if backMatterStart == bodyMatterStart {
		return landmarkMainText{}
	}
	excluded := len(units) - (backMatterStart - bodyMatterStart)
	return landmarkMainText{
		identified: true,
		applies:    excluded > 0,
		bodyStart:  bodyMatterStart,
		backStart:  backMatterStart,
	}
}

// gutenbergMainText is the recognised Project Gutenberg packaging.
type gutenbergMainText struct {
	recognised      bool
	headerUnit      int
	endUnit         int
	headerTrimEnd   uint64
	endLineStart    uint64
	endUnitTextSize uint64
}

func identifyGutenbergDistributorText(units []ExtractedUnit) gutenbergMainText {
	headerUnit, ok := singleGutenbergHeader(units)
	if !ok {
		return gutenbergMainText{}
	}
	start, end, ok := singleGutenbergMarkers(units)
	if !ok || start.unit != headerUnit || !markerAfterStart(start, end) {
		return gutenbergMainText{}
	}
	return gutenbergMainText{
		recognised:      true,
		headerUnit:      start.unit,
		endUnit:         end.unit,
		headerTrimEnd:   start.end,
		endLineStart:    end.start,
		endUnitTextSize: uint64(len([]rune(units[end.unit].Text))),
	}
}

func singleGutenbergHeader(units []ExtractedUnit) (int, bool) {
	headerUnit := -1
	for i, unit := range units {
		if unit.ManifestID != gutenbergHeaderManifestID {
			continue
		}
		if headerUnit >= 0 {
			return 0, false
		}
		headerUnit = i
	}
	return headerUnit, headerUnit >= 0
}

func singleGutenbergMarkers(units []ExtractedUnit) (markerLine, markerLine, bool) {
	var starts, ends []markerLine
	for i, unit := range units {
		unitStarts, unitEnds := markerLines(unit.Text)
		for _, line := range unitStarts {
			line.unit = i
			starts = append(starts, line)
		}
		for _, line := range unitEnds {
			line.unit = i
			ends = append(ends, line)
		}
	}
	if len(starts) != 1 || len(ends) != 1 {
		return markerLine{}, markerLine{}, false
	}
	return starts[0], ends[0], true
}

func markerAfterStart(start, end markerLine) bool {
	return end.unit > start.unit || (end.unit == start.unit && end.end >= start.end)
}

// markerLine is one Project Gutenberg marker line and its span in one unit.
type markerLine struct {
	unit       int
	start, end uint64
}

// markerLines returns the START and END marker lines of one unit's text.
func markerLines(text string) (starts, ends []markerLine) {
	runes := []rune(text)
	lineStart := 0
	for i := 0; i <= len(runes); i++ {
		if i < len(runes) && runes[i] != '\n' {
			continue
		}
		start, startErr := checked.Uint64FromInt(lineStart)
		end, endErr := checked.Uint64FromInt(i)
		if startErr != nil || endErr != nil {
			// Unreachable for a slice index; fail recognition rather than panic.
			return nil, nil
		}
		line := markerLine{start: start, end: end}
		switch markerKindOf(string(runes[lineStart:i])) {
		case markerStart:
			starts = append(starts, line)
		case markerEnd:
			ends = append(ends, line)
		case markerNone:
		}
		lineStart = i + 1
	}
	return starts, ends
}

type markerKind int

const (
	markerNone markerKind = iota
	markerStart
	markerEnd
)

// markerKindOf reports whether a line is a Project Gutenberg START or END
// marker. The fixed words are matched exactly; the eBook title after them is
// arbitrary.
func markerKindOf(line string) markerKind {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 9 || !strings.HasPrefix(trimmed, "***") || !strings.HasSuffix(trimmed, "***") {
		return markerNone
	}
	inner := strings.TrimSpace(trimmed[3 : len(trimmed)-3])
	switch {
	case strings.HasPrefix(inner, "START OF ") && isProjectGutenbergEbook(inner[len("START OF "):]):
		return markerStart
	case strings.HasPrefix(inner, "END OF ") && isProjectGutenbergEbook(inner[len("END OF "):]):
		return markerEnd
	default:
		return markerNone
	}
}

func isProjectGutenbergEbook(rest string) bool {
	rest = strings.TrimPrefix(rest, "THE ")
	rest = strings.TrimPrefix(rest, "THIS ")
	return rest == "PROJECT GUTENBERG EBOOK" || strings.HasPrefix(rest, "PROJECT GUTENBERG EBOOK ")
}

// combineMainText applies both exclusion rules over the ordered units.
func combineMainText(units []ExtractedUnit, landmark landmarkMainText, distributor gutenbergMainText) MainTextSelection {
	selected := make([]bool, len(units))
	for i := range selected {
		selected[i] = !landmark.identified || (i >= landmark.bodyStart && i < landmark.backStart)
	}
	if distributor.recognised {
		for i := distributor.endUnit + 1; i < len(units); i++ {
			selected[i] = false
		}
	}

	selection := MainTextSelection{
		Identified:      landmark.identified,
		BodyMatterStart: landmark.bodyStart,
		BackMatterStart: landmark.backStart,
		SelectedUnitIDs: make([]string, 0, len(units)),
		ExcludedUnitIDs: make([]string, 0, len(units)),
		LandmarkApplies: landmark.identified && landmark.applies,
	}
	for i, unit := range units {
		if selected[i] {
			selection.SelectedUnitIDs = append(selection.SelectedUnitIDs, unit.ID)
		} else {
			selection.ExcludedUnitIDs = append(selection.ExcludedUnitIDs, unit.ID)
		}
	}
	if distributor.recognised {
		selection.Distributor = DistributorSelection{
			Recognised:      true,
			Convention:      DistributorConventionGutenberg,
			ExcludedUnitIDs: make([]string, 0, len(units)),
			TrimmedRanges: []UnitTextRange{
				{UnitID: units[distributor.headerUnit].ID, StartOffset: 0, EndOffset: distributor.headerTrimEnd},
				{UnitID: units[distributor.endUnit].ID, StartOffset: distributor.endLineStart, EndOffset: distributor.endUnitTextSize},
			},
		}
		for i := distributor.endUnit + 1; i < len(units); i++ {
			selection.Distributor.ExcludedUnitIDs = append(selection.Distributor.ExcludedUnitIDs, units[i].ID)
		}
	}
	selection.Applies = len(selection.ExcludedUnitIDs) > 0 || selection.Distributor.Excludes()
	return selection
}

func hasBackMatterLandmark(landmarks []string) bool {
	for _, landmark := range landmarks {
		if _, ok := mainTextBackMatterLandmarks[strings.ToLower(strings.TrimSpace(landmark))]; ok {
			return true
		}
	}
	return false
}

func hasLandmark(landmarks []string, wanted string) bool {
	for _, landmark := range landmarks {
		if strings.EqualFold(strings.TrimSpace(landmark), wanted) {
			return true
		}
	}
	return false
}
