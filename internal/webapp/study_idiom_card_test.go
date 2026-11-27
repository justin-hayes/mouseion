package webapp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSegmentMarkedMarksEveryMatchedWordInOrder(t *testing.T) {
	segments := segmentMarked("Anna ist nicht mit dem Haus, mit Hans.", []string{"mit", "dem", "Haus"})
	assert.Equal(t, []sentenceSegment{
		{Text: "Anna ist nicht "}, {Text: "mit", Marked: true}, {Text: " "}, {Text: "dem", Marked: true},
		{Text: " "}, {Text: "Haus", Marked: true}, {Text: ", mit Hans."},
	}, segments)
}

func TestSegmentMarkedSkipsAFormItCannotFindAndKeepsTheSentenceWhole(t *testing.T) {
	segments := segmentMarked("Er geht heim.", []string{"nach", "heim"})
	assert.Equal(t, []sentenceSegment{{Text: "Er geht "}, {Text: "heim", Marked: true}, {Text: "."}}, segments)
	assert.Equal(t, []sentenceSegment{{Text: "Er geht heim."}}, segmentMarked("Er geht heim.", nil))
}

func TestIdiomFormsSplitsTheTargetWord(t *testing.T) {
	assert.Equal(t, []string{"mit", "dem", "haus"}, idiomForms(" mit dem  haus "))
}

func TestSegmentMarkedPrefersTheTightestRunOfForms(t *testing.T) {
	segments := segmentMarked("Si dice in paese che sia in gamba", []string{"in", "gamba"})
	assert.Equal(t, []sentenceSegment{{Text: "Si dice in paese che sia "}, {Text: "in", Marked: true}, {Text: " "}, {Text: "gamba", Marked: true}}, segments)
}
