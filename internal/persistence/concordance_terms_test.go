package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConcordanceTermKeysNormalizeCaseAndUnicodeButKeepAccents(t *testing.T) {
	for _, test := range []struct {
		name, language, term, lemma, form string
	}{
		{"German capitalization and composition", "de", "Häuser", "häuser", "häuser"},
		{"German keeps eszett distinct", "de", "Straße", "straße", "straße"},
		{"Italian keeps accents", "it", "Città", "città", "città"},
		{"Greek folds final sigma but keeps accents", "el", "ΛΌΓΟΣ", "λόγοσ", "λόγοσ"},
		{"Greek unaccented stays distinct", "el", "λογος", "λογοσ", "λογοσ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			lemma, form := concordanceTermKeys(test.language, test.term)
			assert.Equal(t, test.form, form)
			assert.Equal(t, test.lemma, lemma)
		})
	}
}
