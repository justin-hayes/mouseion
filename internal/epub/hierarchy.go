package epub

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
	"strings"
)

// UnitGroup is a deterministic projection over a persisted extracted-unit
// snapshot. It does not change unit identity or spine order.
type UnitGroup struct {
	ID       string
	Label    string
	ParentID string
	UnitIDs  []string
	First    uint64
	Depth    int
}

type groupCandidate struct {
	kind, key, label, parentKey string
	units                       []ExtractedUnit
}

// BuildUnitGroups derives only evidence-backed groups. Navigation labels and
// landmarks must repeat across multiple units; stable resource directories
// group nested resources. Contradictory overlapping evidence is discarded.
// The returned groups and their members always follow flat spine order.
func BuildUnitGroups(units []ExtractedUnit) []UnitGroup {
	if len(units) < 2 {
		return nil
	}
	var candidates []groupCandidate
	labels := map[string][]ExtractedUnit{}
	landmarks := map[string][]ExtractedUnit{}
	for _, unit := range units {
		for _, label := range uniqueNonBlank(unit.NavigationLabels) {
			labels[label] = append(labels[label], unit)
		}
		for _, landmark := range uniqueNonBlank(unit.LandmarkTypes) {
			landmarks[landmark] = append(landmarks[landmark], unit)
		}
	}
	for label, members := range labels {
		if contiguous(members) && len(members) > 1 {
			candidates = append(candidates, groupCandidate{kind: "nav", key: label, label: label, units: members})
		}
	}
	for landmark, members := range landmarks {
		if contiguous(members) && len(members) > 1 {
			candidates = append(candidates, groupCandidate{kind: "landmark", key: landmark, label: humanize(landmark), units: members})
		}
	}
	directories := map[string][]ExtractedUnit{}
	for _, unit := range units {
		base := path.Dir(unit.PackagePath)
		dir := path.Dir(unit.ResolvedHref)
		if relative, err := strings.CutPrefix(dir, base+"/"); err && relative != "." && relative != "" {
			parts := strings.Split(relative, "/")
			for i := range parts {
				key := strings.Join(parts[:i+1], "/")
				directories[key] = append(directories[key], unit)
			}
		}
	}
	for key, members := range directories {
		if len(members) > 1 && contiguous(members) {
			parent := path.Dir(key)
			if parent == "." {
				parent = ""
			}
			candidates = append(candidates, groupCandidate{kind: "path", key: key, label: humanize(path.Base(key)), parentKey: parent, units: members})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].units[0].Order != candidates[j].units[0].Order {
			return candidates[i].units[0].Order < candidates[j].units[0].Order
		}
		if len(candidates[i].units) != len(candidates[j].units) {
			return len(candidates[i].units) > len(candidates[j].units)
		}
		return candidates[i].kind+":"+candidates[i].key < candidates[j].kind+":"+candidates[j].key
	})
	ids := map[string]string{}
	for _, candidate := range candidates {
		ids[candidate.kind+":"+candidate.key] = groupID(candidate.kind, candidate.key, candidate.units)
	}
	groups := make([]UnitGroup, 0, len(candidates))
	for _, candidate := range candidates {
		group := UnitGroup{ID: ids[candidate.kind+":"+candidate.key], Label: candidate.label, First: candidate.units[0].Order}
		if candidate.parentKey != "" {
			group.ParentID = ids["path:"+candidate.parentKey]
			if group.ParentID != "" {
				group.Depth = strings.Count(candidate.key, "/")
			}
		}
		for _, unit := range candidate.units {
			group.UnitIDs = append(group.UnitIDs, unit.ID)
		}
		groups = append(groups, group)
	}
	return groups
}

func uniqueNonBlank(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func contiguous(units []ExtractedUnit) bool {
	for i := 1; i < len(units); i++ {
		if units[i].Order != units[i-1].Order+1 {
			return false
		}
	}
	return true
}

func groupID(kind, key string, units []ExtractedUnit) string {
	h := sha256.New()
	h.Write([]byte(kind + "\x00" + key))
	for _, unit := range units {
		h.Write([]byte("\x00" + unit.ID))
	}
	return "epub-group-v1:" + hex.EncodeToString(h.Sum(nil)[:12])
}

func humanize(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	if value == "" {
		return "Group"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
