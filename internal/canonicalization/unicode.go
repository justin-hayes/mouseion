package canonicalization

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	UnicodeCasefoldProfile    = "unicode-casefold"
	UnicodeCasefoldVersion    = "1.2.0"
	ModernGreekProfileName    = "modern-greek"
	ModernGreekProfileVersion = "1"
)

var unicodeCasefold = cases.Fold()

// LanguageNeutralProfile is the safe identity profile for languages without a
// dedicated orthographic policy. It intentionally uses full Unicode case
// folding, unlike strings.ToLower, so Greek final sigma and medial sigma share
// one identity.
type LanguageNeutralProfile struct{}

func LanguageNeutral() Profile                  { return LanguageNeutralProfile{} }
func (LanguageNeutralProfile) Name() string     { return UnicodeCasefoldProfile }
func (LanguageNeutralProfile) Version() string  { return UnicodeCasefoldVersion }
func (LanguageNeutralProfile) Language() string { return "" }
func (LanguageNeutralProfile) Canonical(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	return unicodeCasefold.String(norm.NFC.String(value))
}

// ModernGreekProfile defines the vocabulary identity contract for Modern
// Greek. Its normalization is deliberately accent-sensitive.
type ModernGreekProfile struct{}

func ModernGreek() Profile                  { return ModernGreekProfile{} }
func (ModernGreekProfile) Name() string     { return ModernGreekProfileName }
func (ModernGreekProfile) Version() string  { return ModernGreekProfileVersion }
func (ModernGreekProfile) Language() string { return "el" }
func (ModernGreekProfile) Canonical(value string) string {
	return LanguageNeutralProfile{}.Canonical(value)
}

// SearchKey normalizes display text for learner-facing search. It applies the
// Ancient Greek profile's NFC, simple lowercase and final-sigma folding, then
// removes combining marks so an unaccented query matches an accented title.
// Greek titles therefore use this Go key rather than SQL lower().
//
// It is a search key only, never an identity key: vocabulary identity keeps
// accents and breathings significant (see AncientGreekProfile).
func SearchKey(value string) string {
	value = AncientGreekProfile{}.Canonical(value)
	decomposed := norm.NFD.String(value)
	var builder strings.Builder
	builder.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		builder.WriteRune(r)
	}
	return norm.NFC.String(builder.String())
}
