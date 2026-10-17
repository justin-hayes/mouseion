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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsVocabularyStopword(tc.language, tc.lemma, tc.upos))
		})
	}
}
