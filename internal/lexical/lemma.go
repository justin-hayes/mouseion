// Package lexical defines shared vocabulary eligibility rules.
package lexical

import "unicode"

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
