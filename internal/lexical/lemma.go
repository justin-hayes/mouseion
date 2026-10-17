// Package lexical defines shared vocabulary eligibility rules.
package lexical

import (
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

// IsLemma reports whether a canonical lemma contains at least one Unicode
// letter. Digits and punctuation may accompany a letter in a lexical form.
func IsLemma(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// frenchNegationAdverbs are French adverbs that mark grammatical negation, not
// vocabulary. ADV is otherwise an eligible part of speech, so they need an
// explicit exclusion (ADR 0102). NOUN pas, près, là and où stay eligible.
var frenchNegationAdverbs = map[string]struct{}{"ne": {}, "pas": {}}

// IsVocabularyStopword reports whether a canonical lemma with the given part of
// speech is a stopword that never becomes vocabulary in its language, even
// though its part of speech is otherwise eligible: French ADV ne/n' (lemma ne)
// and ADV pas. Every other language is unaffected, so a homograph such as NOUN
// pas stays eligible.
func IsVocabularyStopword(language, lemma, upos string) bool {
	if canonicalization.NormalizeLanguage(language) != "fr" {
		return false
	}
	if strings.ToUpper(strings.TrimSpace(upos)) != "ADV" {
		return false
	}
	_, stop := frenchNegationAdverbs[strings.TrimSpace(lemma)]
	return stop
}
