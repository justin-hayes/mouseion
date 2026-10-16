package canonicalization

import (
	"strings"

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
