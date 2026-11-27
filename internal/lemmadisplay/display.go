// Package lemmadisplay formats canonical lemmas for user-facing presentation.
package lemmadisplay

import (
	"strings"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

// Format formats a canonical lemma for presentation without changing the
// stored vocabulary identity. German common nouns begin with a capital letter,
// and a dedicated-profile language's folded key gets its word-final sigma back
// (ς, never σ); every other language and part of speech is returned unchanged.
func Format(language, lemma, upos string) string {
	if canonicalization.DedicatedProfileLanguage(language) {
		return canonicalization.AncientGreekDisplayForm(lemma)
	}
	if canonicalization.NormalizeLanguage(language) != "de" || !strings.EqualFold(strings.TrimSpace(upos), "NOUN") || lemma == "" {
		return lemma
	}

	first, size := utf8.DecodeRuneInString(lemma)
	return strings.ToUpper(string(first)) + lemma[size:]
}
