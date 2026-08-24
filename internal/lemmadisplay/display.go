// Package lemmadisplay formats canonical lemmas for user-facing presentation.
package lemmadisplay

import (
	"strings"
	"unicode/utf8"
)

// Format formats a canonical lemma for presentation without changing the
// stored vocabulary identity. German common nouns begin with a capital letter;
// every other language and part of speech is returned unchanged.
func Format(language, lemma, upos string) string {
	if baseLanguage(language) != "de" || !strings.EqualFold(strings.TrimSpace(upos), "NOUN") || lemma == "" {
		return lemma
	}

	first, size := utf8.DecodeRuneInString(lemma)
	return strings.ToUpper(string(first)) + lemma[size:]
}

func baseLanguage(language string) string {
	language = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
	base, _, _ := strings.Cut(language, "-")
	return base
}
