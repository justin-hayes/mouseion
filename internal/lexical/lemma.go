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

// functionWordAdverbs are closed per-language lists of function words that the
// language's treebank tags ADV, an otherwise eligible part of speech: French
// negation (ADR 0102), and the Ancient Greek and Latin conjunctions, particles
// and negations (ADR 0140). Lemmas are canonical (Latin folds v to u). A word
// matches by canonical lemma plus part of speech, so a content-word homograph
// tagged NOUN, VERB or ADJ stays eligible, and real adverbs are not listed.
var functionWordAdverbs = map[string][]string{
	"fr": {"ne", "pas"},
	"grc": {
		"δέ", "καί", "τε", "μέν", "γάρ", "οὖν", "δή", "ἄν", "ἀλλά", "γε", "ἤ", "ἄρα",
		"τοίνυν", "μέντοι", "οὐ", "οὐκ", "οὐχ", "μή", "μη", "οὐδέ", "μηδέ",
	},
	"la": {
		"et", "ue", "uel", "nec", "atque", "enim", "nam", "uero", "igitur", "ergo", "itaque",
		"tamen", "quoque", "quidem", "etiam", "non", "ne", "nonne", "num", "haud",
	},
}

var functionWordSets = func() map[string]map[string]struct{} {
	sets := make(map[string]map[string]struct{}, len(functionWordAdverbs))
	for language, lemmas := range functionWordAdverbs {
		set := make(map[string]struct{}, len(lemmas))
		for _, lemma := range lemmas {
			set[lemma] = struct{}{}
		}
		sets[language] = set
	}
	return sets
}()

// FunctionWordLemmas returns a copy of the language's closed function-word
// list (canonical lemmas), or nil when the language has none. Context
// stopwords reuse it so the two lists cannot drift.
func FunctionWordLemmas(language string) []string {
	return append([]string(nil), functionWordAdverbs[canonicalization.NormalizeLanguage(language)]...)
}

// IsVocabularyStopword reports whether a canonical lemma with the given part of
// speech is a function word that never becomes vocabulary in its language, even
// though its part of speech is otherwise eligible: French ADV ne/n' (lemma ne)
// and ADV pas, and the Ancient Greek and Latin ADV function words. Every other
// language is unaffected, and so is a homograph with another part of speech,
// such as French NOUN pas.
func IsVocabularyStopword(language, lemma, upos string) bool {
	set := functionWordSets[canonicalization.NormalizeLanguage(language)]
	if set == nil || strings.ToUpper(strings.TrimSpace(upos)) != "ADV" {
		return false
	}
	_, stop := set[strings.TrimSpace(lemma)]
	return stop
}

// IsAdmissibleLemma reports whether a canonical lemma may become a vocabulary
// identity in its language. Every language needs a lemma with a letter
// (IsLemma). Latin (ADR 0132) additionally rejects a lemma that contains a
// full stop (the praenomen abbreviations M. and C.), a digit, or a letter
// outside the Latin script (Greek-script lemmas): such a token stays in the text
// but never becomes an identity, a card or Recurring vocabulary.
func IsAdmissibleLemma(language, lemma string) bool {
	lemma = strings.TrimSpace(lemma)
	if !IsLemma(lemma) {
		return false
	}
	if canonicalization.NormalizeLanguage(language) != "la" {
		return true
	}
	for _, r := range lemma {
		switch {
		case r == '.', unicode.IsDigit(r):
			return false
		case unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r):
			return false
		}
	}
	return true
}
