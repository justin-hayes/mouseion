package canonicalization

import "strings"

// GermanPost1996Profile is the conservative German standard-orthography
// profile. Version 3 adds analyzer pipe-alternative selection; version 2
// remains registered for reproducible historical normalization.
type GermanPost1996Profile struct{ version string }

func GermanPost1996() Profile              { return GermanPost1996Profile{} }
func (GermanPost1996Profile) Name() string { return "german-standard-post-1996" }
func (p GermanPost1996Profile) Version() string {
	if p.version == "" {
		return "3"
	}
	return p.version
}
func (GermanPost1996Profile) Language() string { return "de" }
func (p GermanPost1996Profile) Canonical(s string) string {
	if p.Version() == "3" {
		s = primaryAnalyzerLemma(s)
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

// Exact lexical rules avoid unsafe blanket replacement: modern Straße and Maße
// retain ß, and diacritics remain part of vocabulary identity.
var germanPost1996Equivalences = map[string]string{
	"daß": "dass", "muß": "muss", "mußt": "musst", "müßt": "müsst",
	"fluß": "fluss", "kuß": "kuss", "nuß": "nuss", "naß": "nass",
	"schluß": "schluss", "schloß": "schloss", "thür": "tür", "thüre": "türe",
}
