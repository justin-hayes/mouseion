package lexical

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLemma(t *testing.T) {
	for _, value := range []string{"Haus", "Straße", "l'acqua", "l’acqua", "città", "B2"} {
		assert.True(t, IsLemma(value), value)
	}
	for _, value := range []string{"", " ", "5", "123.4", "—", "'", "💡"} {
		assert.False(t, IsLemma(value), value)
	}
}

func TestIsVocabularyStopword(t *testing.T) {
	tests := []struct {
		name                  string
		language, lemma, upos string
		want                  bool
	}{
		{"French ADV ne", "fr", "ne", "ADV", true},
		{"French locale ADV ne", "fr-FR", "ne", "ADV", true},
		{"French ADV n' lemmatizes to ne", "fr", "ne", "adv", true},
		{"French ADV pas", "fr", "pas", "ADV", true},
		{"French NOUN pas stays eligible", "fr", "pas", "NOUN", false},
		{"French ADV près stays eligible", "fr", "près", "ADV", false},
		{"French ADV là stays eligible", "fr", "là", "ADV", false},
		{"French ADV où stays eligible", "fr", "où", "ADV", false},
		{"German ADV pas is unaffected", "de", "pas", "ADV", false},
		{"Italian ADV ne is unaffected", "it", "ne", "ADV", false},
		{"Greek ADV conjunction δέ", "grc", "δέ", "ADV", true},
		{"Greek ADV καί", "grc", "καί", "ADV", true},
		{"Greek ADV negation οὐ", "grc", "οὐ", "ADV", true},
		{"Greek ADV negation μή", "grc", "μή", "ADV", true},
		{"Greek ADV particle ἄν lower-case upos", "grc", "ἄν", "adv", true},
		{"Greek locale tag", "grc-GR", "γάρ", "ADV", true},
		{"Greek real adverb ὡς stays eligible", "grc", "ὡς", "ADV", false},
		{"Greek real adverb νῦν stays eligible", "grc", "νῦν", "ADV", false},
		{"Greek real adverb οὕτως stays eligible", "grc", "οὕτως", "ADV", false},
		{"Greek pronoun αὐτός stays eligible", "grc", "αὐτός", "PRON", false},
		{"Greek content homograph of a listed lemma stays eligible", "grc", "γε", "NOUN", false},
		{"Greek ADV ἤδη stays eligible", "grc", "ἤδη", "ADV", false},
		{"Modern Greek και is unaffected", "el", "και", "ADV", false},
		{"Modern Greek ADV δέ is unaffected", "el", "δέ", "ADV", false},
		{"Latin ADV non", "la", "non", "ADV", true},
		{"Latin ADV ne", "la", "ne", "ADV", true},
		{"Latin ADV et", "la", "et", "ADV", true},
		{"Latin ADV tamen", "la", "tamen", "ADV", true},
		{"Latin canonical uero (v folds to u)", "la", "uero", "ADV", true},
		{"Latin VERB homograph uero stays eligible", "la", "uero", "VERB", false},
		{"Latin NOUN homograph ne stays eligible", "la", "ne", "NOUN", false},
		{"Latin real adverb nunc stays eligible", "la", "nunc", "ADV", false},
		{"Latin real adverb ita stays eligible", "la", "ita", "ADV", false},
		{"Latin real adverb semper stays eligible", "la", "semper", "ADV", false},
		{"Latin lemma on a Greek-only entry", "la", "δέ", "ADV", false},
		{"Greek lemma on a Latin-only entry", "grc", "non", "ADV", false},
		{"German non is unaffected", "de", "non", "ADV", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsVocabularyStopword(tc.language, tc.lemma, tc.upos))
		})
	}
}

func TestIsAdmissibleLemma(t *testing.T) {
	for _, tc := range []struct {
		name, language, lemma string
		want                  bool
	}{
		{"latin word", "la", "video", true},
		{"latin with hyphen", "la", "ne-que", true},
		{"praenomen abbreviation", "la", "m.", false},
		{"digit lemma", "la", "et1", false},
		{"greek script lemma", "la", "λόγος", false},
		{"mixed script lemma", "la", "aλ", false},
		{"punctuation only", "la", "-", false},
		{"greek keeps its rule", "grc", "λόγος", true},
		{"dot is fine outside latin", "de", "z.b.", true},
		{"digit is fine outside latin", "de", "3d", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsAdmissibleLemma(tc.language, tc.lemma))
		})
	}
}
