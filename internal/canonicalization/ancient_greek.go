package canonicalization

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	// AncientGreekProfileName and AncientGreekProfileVersion identify the
	// normalization profile registered for the grc language.
	AncientGreekProfileName    = "ancient-greek"
	AncientGreekProfileVersion = "1"

	// AncientGreekUnicodeVersion is the Unicode release ancient-greek/1 is
	// pinned to. The Go toolchain's case-mapping tables and golang.org/x/text's
	// normalization tables must both report it.
	AncientGreekUnicodeVersion = "17.0.0"

	// ancientGreekLanguage is the base language tag the profile serves and the
	// language the registry treats as a dedicated-profile language.
	ancientGreekLanguage = "grc"
)

// IsAncientGreek reports whether language is Ancient Greek, the language whose
// vocabulary keys fold word-final sigma.
func IsAncientGreek(language string) bool {
	return normalizeLanguage(language) == ancientGreekLanguage
}

// AncientGreekDisplayForm reverses the key's sigma folding for display: a σ at
// the end of a word is always ς in Greek. Accents and breathings are never
// folded, so nothing else changes. It is for learner-facing text only; keys stay
// folded.
func AncientGreekDisplayForm(key string) string {
	runes := []rune(norm.NFC.String(key))
	for i, r := range runes {
		if r != 'σ' {
			continue
		}
		if i+1 == len(runes) || !unicode.IsLetter(runes[i+1]) && !unicode.Is(unicode.Mn, runes[i+1]) {
			runes[i] = 'ς'
		}
	}
	return string(runes)
}

// AncientGreekProfile derives vocabulary identity keys for Ancient Greek.
// Accents, breathings, diaeresis and iota subscript stay significant, so only
// canonical equivalence unifies spellings. It applies simple Unicode lowercase,
// never full case folding, and so keeps a subscript form and its adscript
// spelling distinct.
type AncientGreekProfile struct{}

func AncientGreek() Profile                  { return AncientGreekProfile{} }
func (AncientGreekProfile) Name() string     { return AncientGreekProfileName }
func (AncientGreekProfile) Version() string  { return AncientGreekProfileVersion }
func (AncientGreekProfile) Language() string { return ancientGreekLanguage }
func (AncientGreekProfile) Canonical(value string) string {
	value = norm.NFC.String(value)
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "ς", "σ")
	return norm.NFC.String(value)
}
