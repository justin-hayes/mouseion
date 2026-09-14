package epub

import "strings"

// MainTextSelection is the pure, structure-derived analysis scope decision.
// BodyMatterStart and BackMatterStart are zero-based positions in units.
// Identified distinguishes a valid no-op decision from the fail-safe fallback;
// Applies is true only when the decision excludes at least one unit.
type MainTextSelection struct {
	Identified      bool
	Applies         bool
	BodyMatterStart uint64
	BackMatterStart uint64
	SelectedUnitIDs []string
	ExcludedUnitIDs []string
}

var mainTextBackMatterLandmarks = map[string]struct{}{
	"backmatter":   {},
	"bibliography": {},
	"index":        {},
	"glossary":     {},
	"colophon":     {},
}

// IdentifyMainText identifies the declared main-text run in ordered readable
// units. Ambiguous declarations fall back to selecting the complete snapshot.
func IdentifyMainText(units []ExtractedUnit) MainTextSelection {
	bodyMatterStart := -1
	for i, unit := range units {
		if !hasLandmark(unit.LandmarkTypes, "bodymatter") {
			continue
		}
		if bodyMatterStart >= 0 {
			return wholeSnapshotSelection(units)
		}
		bodyMatterStart = i
	}
	if bodyMatterStart < 0 {
		return wholeSnapshotSelection(units)
	}

	backMatterStart := len(units)
	for i := bodyMatterStart; i < len(units); i++ {
		if hasBackMatterLandmark(units[i].LandmarkTypes) {
			backMatterStart = i
			break
		}
	}
	if backMatterStart == bodyMatterStart {
		return wholeSnapshotSelection(units)
	}

	selected := unitIDs(units[bodyMatterStart:backMatterStart])
	excluded := append(unitIDs(units[:bodyMatterStart]), unitIDs(units[backMatterStart:])...)
	return MainTextSelection{
		Identified:      true,
		Applies:         len(excluded) > 0,
		BodyMatterStart: uint64(bodyMatterStart),
		BackMatterStart: uint64(backMatterStart),
		SelectedUnitIDs: selected,
		ExcludedUnitIDs: excluded,
	}
}

func wholeSnapshotSelection(units []ExtractedUnit) MainTextSelection {
	return MainTextSelection{
		SelectedUnitIDs: unitIDs(units),
		ExcludedUnitIDs: []string{},
	}
}

func unitIDs(units []ExtractedUnit) []string {
	ids := make([]string, 0, len(units))
	for _, unit := range units {
		ids = append(ids, unit.ID)
	}
	return ids
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
