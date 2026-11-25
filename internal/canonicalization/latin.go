package canonicalization

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	// LatinProfileName and LatinProfileVersion identify the normalization
	// profile registered for the la language (ADR 0132).
	LatinProfileName    = "latin"
	LatinProfileVersion = "1"

	// latinLanguage is the base language tag the profile serves.
	latinLanguage = "la"
)

// LatinProfile derives vocabulary identity keys for Classical Latin. It
// strips combining marks (macrons, breves), applies simple Unicode lowercase and
// unifies the u/v and i/j spelling variants. It never folds the ae and oe
// digraphs: quae and que, caedo and cedo stay distinct words.
type LatinProfile struct{}

func Latin() Profile                  { return LatinProfile{} }
func (LatinProfile) Name() string     { return LatinProfileName }
func (LatinProfile) Version() string  { return LatinProfileVersion }
func (LatinProfile) Language() string { return latinLanguage }
func (LatinProfile) Canonical(value string) string {
	decomposed := norm.NFD.String(value)
	var out strings.Builder
	out.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		switch r = unicode.ToLower(r); r {
		case 'v':
			r = 'u'
		case 'j':
			r = 'i'
		}
		out.WriteRune(r)
	}
	return norm.NFC.String(out.String())
}
