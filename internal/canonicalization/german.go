package canonicalization

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// The normalization policy is shared with the dictionary index derivation.
// Keep the source next to this package so it can be embedded in the server.
//
//go:embed german_post1996.json
var germanPost1996Data []byte

type germanPost1996Policy struct {
	Equivalences   map[string]string       `json:"equivalences"`
	V6Equivalences map[string]string       `json:"v6_equivalences"`
	EdgeCleanup    germanEdgeCleanupPolicy `json:"edge_cleanup"`
}

type germanEdgeCleanupPolicy struct {
	StripUnicodeCategories []string `json:"strip_unicode_categories"`
	PreserveCharacters     []string `json:"preserve_characters"`
}

var germanPost1996PolicyData = func() germanPost1996Policy {
	var policy germanPost1996Policy
	if err := json.Unmarshal(germanPost1996Data, &policy); err != nil {
		panic(fmt.Errorf("canonicalization: load German post-1996 policy: %w", err))
	}
	return policy
}()

// GermanPost1996Profile is the conservative German standard-orthography
// profile. Version 6 adds explicit pre-1996 spelling equivalences; older
// versions remain registered for reproducible historical normalization.
type GermanPost1996Profile struct{ version string }

func GermanPost1996() Profile              { return GermanPost1996Profile{} }
func (GermanPost1996Profile) Name() string { return "german-standard-post-1996" }
func (p GermanPost1996Profile) Version() string {
	if p.version == "" {
		return "6"
	}
	return p.version
}
func (GermanPost1996Profile) Language() string { return "de" }
func (p GermanPost1996Profile) Canonical(s string) string {
	if p.Version() == "3" {
		s = primaryAnalyzerLemma(s)
	} else if p.Version() == "4" || p.Version() == "5" || p.Version() == "6" {
		s = cleanLemmaEdges(primaryAnalyzerLemma(s))
	}
	s = Lemma(s)
	if modern, ok := germanPost1996PolicyData.Equivalences[s]; ok {
		return modern
	}
	if p.Version() == "6" {
		if modern, ok := germanPost1996PolicyData.V6Equivalences[s]; ok {
			return modern
		}
	}
	return s
}

func primaryAnalyzerLemma(value string) string {
	for alternative := range strings.SplitSeq(value, "|") {
		if alternative = strings.TrimSpace(alternative); alternative != "" {
			return alternative
		}
	}
	return ""
}

func cleanLemmaEdges(value string) string {
	runes := []rune(value)
	start, end := 0, len(runes)
	for start < end && isEdgeDecoration(runes[start]) {
		start++
	}
	for end > start && isEdgeDecoration(runes[end-1]) {
		end--
	}
	if start == end {
		return value
	}
	return string(runes[start:end])
}

// isEdgeDecoration reports whether r is decoration stripped from a lemma's
// leading or trailing edge. Preserved characters are never decoration.
func isEdgeDecoration(r rune) bool {
	if slices.Contains(germanPost1996PolicyData.EdgeCleanup.PreserveCharacters, string(r)) {
		return false
	}
	return stripsUnicodeCategory(r)
}

// stripsUnicodeCategory reports whether r belongs to any Unicode category the
// policy strips from lemma edges.
func stripsUnicodeCategory(r rune) bool {
	for _, category := range germanPost1996PolicyData.EdgeCleanup.StripUnicodeCategories {
		if inUnicodeCategory(r, category) {
			return true
		}
	}
	return false
}

// inUnicodeCategory reports whether r belongs to one named category the policy
// supports.
func inUnicodeCategory(r rune, category string) bool {
	switch category {
	case "P":
		return unicode.IsPunct(r)
	case "S":
		return unicode.Is(unicode.S, r)
	}
	return false
}
