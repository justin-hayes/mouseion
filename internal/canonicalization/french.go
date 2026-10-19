package canonicalization

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

const (
	// FrenchProfileName and FrenchProfileVersion identify the normalization
	// profile registered for the fr language. Version 2 records the dialogue
	// segmentation that keeps an en dash or horizontal bar with the line it
	// introduces; version 1 stays registered for reproducible historical
	// analysis.
	FrenchProfileName    = "french"
	FrenchProfileVersion = "2"

	// frenchLanguage is the base language tag the profile serves and the
	// language the registry treats as a dedicated-profile language.
	frenchLanguage = "fr"

	// rightSingleQuotationMark is the typographic apostrophe French text uses
	// for elision and inside forms such as aujourd'hui.
	rightSingleQuotationMark = "\u2019"
	// asciiApostrophe is the canonical spelling the French profile folds the
	// typographic apostrophe to, so aujourd'hui and aujourd’hui share one
	// identity.
	asciiApostrophe = "'"
)

// FrenchProfile derives vocabulary identity keys for French. It applies NFC
// normalization and full Unicode case folding while keeping accents
// significant, so où and ou, and sûr and sur, stay distinct. In the canonical
// lemma only, U+2019 RIGHT SINGLE QUOTATION MARK folds to U+0027 APOSTROPHE.
// Raw lemmas and source text are never rewritten. Version 2 records the
// dialogue segmentation that keeps an en dash or horizontal bar with the line
// it introduces; version 1 remains addressable for reproducible analysis.
type FrenchProfile struct{ version string }

func French() Profile              { return FrenchProfile{} }
func (FrenchProfile) Name() string { return FrenchProfileName }
func (p FrenchProfile) Version() string {
	if p.version == "" {
		return FrenchProfileVersion
	}
	return p.version
}
func (FrenchProfile) Language() string { return frenchLanguage }
func (FrenchProfile) Canonical(value string) string {
	value = unicodeCasefold.String(norm.NFC.String(value))
	value = strings.Join(strings.Fields(value), " ")
	return strings.ReplaceAll(value, rightSingleQuotationMark, asciiApostrophe)
}
