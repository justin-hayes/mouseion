package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChooseCustomDeckOccurrenceUsesQualityAndStableCrossBookTies(t *testing.T) {
	candidates := []customDeckOccurrence{
		{BookID: "book-b", SourceID: "source-a", Offset: 3, Sentence: "tie", Quality: cardexport.SentenceQuality{Accepted: true, Score: 90, GDEXScore: .8}},
		{BookID: "book-a", SourceID: "source-z", Offset: 9, Sentence: "tie", Quality: cardexport.SentenceQuality{Accepted: true, Score: 90, GDEXScore: .8}},
		{BookID: "book-a", SourceID: "source-a", Offset: 12, Sentence: "best", Quality: cardexport.SentenceQuality{Accepted: true, Score: 90, GDEXScore: .9}},
	}

	got, ok := chooseCustomDeckOccurrence(candidates)
	require.True(t, ok)
	assert.Equal(t, "best", got.Sentence)

	candidates[2].Quality.GDEXScore = .8
	got, ok = chooseCustomDeckOccurrence(candidates)
	require.True(t, ok)
	assert.Equal(t, "book-a", got.BookID)
	assert.Equal(t, "source-a", got.SourceID)
	assert.EqualValues(t, 12, got.Offset)
}

func TestChooseCustomDeckOccurrenceRejectsQualityOmissions(t *testing.T) {
	got, ok := chooseCustomDeckOccurrence([]customDeckOccurrence{{BookID: "book-a", Sentence: "poor context", Quality: cardexport.SentenceQuality{Accepted: false}}})
	assert.False(t, ok)
	assert.Empty(t, got)
}
