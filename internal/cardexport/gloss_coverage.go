package cardexport

import (
	"encoding/json"
	"log"
	"slices"
	"strings"
)

type glossCoverageGroup struct {
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

type glossCoverageEvent struct {
	Event  string               `json:"event"`
	Groups []glossCoverageGroup `json:"groups"`
}

func logGlossCoverage(entries []Entry) {
	grouped := make(map[glossCoverageKey]*glossCoverageGroup)
	for _, entry := range entries {
		key := glossCoverageKey{language: strings.ToLower(strings.TrimSpace(entry.Language)), pos: strings.ToUpper(strings.TrimSpace(entry.UPOS))}
		group := grouped[key]
		if group == nil {
			group = &glossCoverageGroup{Language: key.language, POS: key.pos}
			grouped[key] = group
		}
		group.Selected++
		if strings.TrimSpace(entry.Gloss) == "" {
			group.WithoutGloss++
		} else {
			group.WithGloss++
		}
	}

	groups := make([]glossCoverageGroup, 0, len(grouped))
	for _, group := range grouped {
		groups = append(groups, *group)
	}
	slices.SortFunc(groups, func(a, b glossCoverageGroup) int {
		if a.Language != b.Language {
			return strings.Compare(a.Language, b.Language)
		}
		return strings.Compare(a.POS, b.POS)
	})
	payload, err := json.Marshal(glossCoverageEvent{Event: "gloss_coverage", Groups: groups})
	if err == nil {
		log.Printf("gloss_coverage %s", payload)
	}
}
