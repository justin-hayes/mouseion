package cardexport

import (
	"slices"
	"strings"
)

type GlossCoverage struct {
	Language     string `json:"language"`
	POS          string `json:"pos"`
	Selected     int    `json:"selected"`
	WithGloss    int    `json:"with_gloss"`
	WithoutGloss int    `json:"without_gloss"`
}

type glossCoverageKey struct {
	language string
	pos      string
}

func glossCoverage(entries []Entry) []GlossCoverage {
	grouped := make(map[glossCoverageKey]*GlossCoverage)
	for _, entry := range entries {
		key := glossCoverageKey{language: strings.ToLower(strings.TrimSpace(entry.Language)), pos: strings.ToUpper(strings.TrimSpace(entry.UPOS))}
		group := grouped[key]
		if group == nil {
			group = &GlossCoverage{Language: key.language, POS: key.pos}
			grouped[key] = group
		}
		group.Selected++
		if strings.TrimSpace(entry.Gloss) == "" {
			group.WithoutGloss++
		} else {
			group.WithGloss++
		}
	}

	groups := make([]GlossCoverage, 0, len(grouped))
	for _, group := range grouped {
		groups = append(groups, *group)
	}
	slices.SortFunc(groups, func(a, b GlossCoverage) int {
		if a.Language != b.Language {
			return strings.Compare(a.Language, b.Language)
		}
		return strings.Compare(a.POS, b.POS)
	})
	return groups
}

func cloneGlossCoverage(groups []GlossCoverage) []GlossCoverage {
	if groups == nil {
		return nil
	}
	return append([]GlossCoverage{}, groups...)
}
