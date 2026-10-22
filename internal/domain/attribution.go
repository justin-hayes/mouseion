package domain

import (
	"sort"
	"strings"
)

// AttributionKind is the structural attribution a source span carries. It is
// derived from recorded role spans; it is never an inferred speaker identity.
type AttributionKind string

const (
	AttributionNone           AttributionKind = ""
	AttributionSpeaker        AttributionKind = "speaker"
	AttributionStageDirection AttributionKind = "stage_direction"
)

// SourceAttribution is the attribution of one half-open source span in a unit.
// Speaker is set only for a speaker attribution and omits nested directions.
type SourceAttribution struct {
	Kind    AttributionKind
	Speaker string
}

// AttributionAt derives the structural attribution of a half-open span in one
// unit. A span overlapping a stage direction is a direction. Otherwise the
// nearest preceding speaker label in the same unit supplies the speaker, and a
// heading reset discards labels from before the heading. A direction does not
// clear the preceding speaker for later spans, and no preceding label means no
// attribution. A snapshot that did not record structure has no attribution.
func (s SnapshotStructure) AttributionAt(unit ExtractedUnit, start, end uint64) SourceAttribution {
	runes := []rune(unit.Text)
	if !s.Recorded || start >= end || end > uint64(len(runes)) {
		return SourceAttribution{}
	}
	if s.overlapsDirection(unit.ID, start, end) {
		return SourceAttribution{Kind: AttributionStageDirection}
	}
	label, ok := s.precedingSpeaker(unit.ID, start)
	if !ok {
		return SourceAttribution{}
	}
	return SourceAttribution{Kind: AttributionSpeaker, Speaker: speakerDisplayName(runes, label, s.RoleSpans)}
}

// overlapsDirection reports whether a queried span overlaps a recorded stage
// direction in the named unit.
func (s SnapshotStructure) overlapsDirection(unitID string, start, end uint64) bool {
	for _, span := range s.RoleSpans {
		if span.UnitID == unitID && span.Role == SourceStructureRoleDirection && span.StartOffset < end && span.EndOffset > start {
			return true
		}
	}
	return false
}

// precedingSpeaker returns the nearest speaker label that ends before start and
// is not discarded by the latest heading reset before start.
func (s SnapshotStructure) precedingSpeaker(unitID string, start uint64) (UnitRoleSpan, bool) {
	reset := s.latestHeadingReset(unitID, start)
	var label UnitRoleSpan
	found := false
	for _, span := range s.RoleSpans {
		if span.UnitID != unitID || span.Role != SourceStructureRoleSpeaker {
			continue
		}
		if span.EndOffset > start || span.EndOffset <= reset {
			continue
		}
		if !found || span.EndOffset > label.EndOffset {
			label, found = span, true
		}
	}
	return label, found
}

// latestHeadingReset is the greatest heading-reset offset that starts at or
// before start, or zero when the unit has no such reset.
func (s SnapshotStructure) latestHeadingReset(unitID string, start uint64) uint64 {
	var reset uint64
	for _, candidate := range s.HeadingResets {
		if candidate.UnitID == unitID && candidate.Offset <= start && candidate.Offset > reset {
			reset = candidate.Offset
		}
	}
	return reset
}

// speakerDisplayName renders only the name from a speaker label, omitting any
// directions nested inside it. Whitespace collapses to single spaces and one
// terminal period is removed.
func speakerDisplayName(text []rune, label UnitRoleSpan, spans []UnitRoleSpan) string {
	if label.EndOffset > uint64(len(text)) || label.EndOffset <= label.StartOffset {
		return ""
	}
	directions := nestedDirections(label, spans)
	var name strings.Builder
	previous := label.StartOffset
	for _, direction := range directions {
		if direction.StartOffset > previous {
			name.WriteString(string(text[previous:direction.StartOffset]))
		}
		if direction.EndOffset > previous {
			previous = direction.EndOffset
		}
	}
	if previous < label.EndOffset {
		name.WriteString(string(text[previous:label.EndOffset]))
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.Join(strings.Fields(name.String()), " "), "."))
}

// nestedDirections returns the direction spans contained by label, ordered by
// their start offset.
func nestedDirections(label UnitRoleSpan, spans []UnitRoleSpan) []UnitRoleSpan {
	var directions []UnitRoleSpan
	for _, span := range spans {
		if span.UnitID != label.UnitID || span.Role != SourceStructureRoleDirection {
			continue
		}
		if span.StartOffset >= label.StartOffset && span.EndOffset <= label.EndOffset {
			directions = append(directions, span)
		}
	}
	sort.Slice(directions, func(i, j int) bool { return directions[i].StartOffset < directions[j].StartOffset })
	return directions
}
