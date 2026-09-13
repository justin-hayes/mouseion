package canonicalization

import (
	"strings"
	"unicode"
)

// GermanPost1996Profile is the conservative German standard-orthography
// profile. Version 5 includes producer-level separable-verb reattachment;
// older versions remain registered for reproducible historical normalization.
type GermanPost1996Profile struct{ version string }

func GermanPost1996() Profile              { return GermanPost1996Profile{} }
func (GermanPost1996Profile) Name() string { return "german-standard-post-1996" }
func (p GermanPost1996Profile) Version() string {
	if p.version == "" {
		return "5"
	}
	return p.version
}
func (GermanPost1996Profile) Language() string { return "de" }
func (p GermanPost1996Profile) Canonical(s string) string {
	if p.Version() == "3" {
		s = primaryAnalyzerLemma(s)
	} else if p.Version() == "4" || p.Version() == "5" {
		s = cleanLemmaEdges(primaryAnalyzerLemma(s))
	}
	s = Lemma(s)
	if modern, ok := germanPost1996Equivalences[s]; ok {
		return modern
	}
	return s
}

func primaryAnalyzerLemma(value string) string {
	for _, alternative := range strings.Split(value, "|") {
		if alternative = strings.TrimSpace(alternative); alternative != "" {
			return alternative
		}
	}
	return ""
}

func cleanLemmaEdges(value string) string {
	runes := []rune(value)
	isEdgeDecoration := func(r rune) bool {
		return r != '\'' && r != '’' && (unicode.IsPunct(r) || unicode.Is(unicode.S, r))
	}
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

// Exact lexical rules avoid unsafe blanket replacement: modern Straße and Maße
// retain ß, and diacritics remain part of vocabulary identity.
var germanPost1996Equivalences = map[string]string{
	"daß": "dass", "muß": "muss", "mußt": "musst", "müßt": "müsst",
	"fluß": "fluss", "kuß": "kuss", "nuß": "nuss", "naß": "nass",
	"schluß": "schluss", "schloß": "schloss", "thür": "tür", "thüre": "türe",
}
