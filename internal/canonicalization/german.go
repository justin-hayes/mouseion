package canonicalization

// GermanPost1996Profile is the conservative German standard-orthography v1 profile.
type GermanPost1996Profile struct{}

func GermanPost1996() Profile                  { return GermanPost1996Profile{} }
func (GermanPost1996Profile) Name() string     { return "german-standard-post-1996" }
func (GermanPost1996Profile) Version() string  { return "1" }
func (GermanPost1996Profile) Language() string { return "de" }
func (GermanPost1996Profile) Canonical(s string) string {
	s = Lemma(s)
	if modern, ok := germanPost1996Equivalences[s]; ok {
		return modern
	}
	return s
}

// Exact lexical rules avoid unsafe blanket replacement: modern Straße and Maße
// retain ß, and diacritics remain part of vocabulary identity.
var germanPost1996Equivalences = map[string]string{
	"daß": "dass", "muß": "muss", "mußt": "musst", "müßt": "müsst",
	"fluß": "fluss", "kuß": "kuss", "nuß": "nuss", "naß": "nass",
	"schluß": "schluss", "schloß": "schloss", "thür": "tür", "thüre": "türe",
}
