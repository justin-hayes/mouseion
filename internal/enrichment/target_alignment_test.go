package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSentenceTranslationTargetsIsAllOrNothing(t *testing.T) {
	const sentence = "And then the motorcycle knocks Piero over."
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{name: "discontinuous", input: []string{"knocks", "over"}, want: []string{"knocks", "over"}},
		{name: "contiguous", input: []string{"motorcycle"}, want: []string{"motorcycle"}},
		{name: "missing", input: nil},
		{name: "repeated", input: []string{"then", "Piero", "then"}},
		{name: "out of order", input: []string{"over", "knocks"}},
		{name: "overlap", input: []string{"knocks Piero", "Piero"}},
		{name: "non-word-bounded", input: []string{"ock"}},
		{name: "unsafe markup", input: []string{"knocks", "<b>over</b>"}},
		{name: "collectively whole sentence", input: []string{"And", "then", "the", "motorcycle", "knocks", "Piero", "over"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, ValidateSentenceTranslationTargets(sentence, test.input))
		})
	}
	assert.Equal(t, []string{"Go"}, ValidateSentenceTranslationTargets("Go!", []string{"Go"}), "a one-word sentence remains eligible")
	assert.Nil(t, ValidateSentenceTranslationTargets("He knocks and then knocks again.", []string{"knocks"}), "repeated excerpts are ambiguous")
	assert.Nil(t, ValidateSentenceTranslationTargets("And then the motorcycle knocks Piero over.", []string{"Knocks"}), "excerpts must exactly match their source text")
}
