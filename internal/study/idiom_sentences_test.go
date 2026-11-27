package study

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sentenceStoreStub is an idiomSentenceStore with fixed answers; failing names
// the call that returns errReviewStore.
type sentenceStoreStub struct {
	refs      []persistence.StudyIdiomSentenceRef
	sentences []persistence.StudyIdiomSentence
	failing   string
}

func (s sentenceStoreStub) ListStudyIdiomSentenceRefs(context.Context, string, string, string, []string) ([]persistence.StudyIdiomSentenceRef, error) {
	if s.failing == "refs" {
		return nil, errReviewStore
	}
	return s.refs, nil
}

func (s sentenceStoreStub) LoadStudyIdiomSentences(context.Context, string, []persistence.StudyIdiomSentenceRef) ([]persistence.StudyIdiomSentence, error) {
	if s.failing == "load" {
		return nil, errReviewStore
	}
	return s.sentences, nil
}

func TestIdiomChoicesReportAStoreOrIndexFailure(t *testing.T) {
	entry := mitDemHaus()
	reader := func(index idiom.Index) *idiomReader {
		return switchOn(index).reader("de")
	}
	refs := []persistence.StudyIdiomSentenceRef{{BookID: "b", SentenceOrdinal: 1}}
	sentence := fluencyIdiomSentence(1, "Emma", "haus", nominative())

	t.Run("sentence references that cannot be read", func(t *testing.T) {
		_, err := idiomChoicesOf(t.Context(), sentenceStoreStub{failing: "refs"}, "owner", reader(&fakeIdiomIndex{}), entry, nil, nil)
		require.ErrorIs(t, err, errReviewStore)
	})
	t.Run("sentences that cannot be loaded", func(t *testing.T) {
		_, err := idiomChoicesOf(t.Context(), sentenceStoreStub{refs: refs, failing: "load"}, "owner", reader(&fakeIdiomIndex{}), entry, nil, nil)
		require.ErrorIs(t, err, errReviewStore)
	})
	t.Run("an index that cannot be read", func(t *testing.T) {
		_, err := idiomChoicesOf(t.Context(), sentenceStoreStub{refs: refs, sentences: []persistence.StudyIdiomSentence{sentence}}, "owner", reader(&fakeIdiomIndex{err: errReviewStore}), entry, nil, nil)
		require.ErrorIs(t, err, errReviewStore)
	})
	t.Run("an entry the index cannot be asked for", func(t *testing.T) {
		_, _, err := reader(&fakeIdiomIndex{err: errReviewStore}).entry(t.Context(), "mit dem haus")
		require.ErrorIs(t, err, errReviewStore)
	})
}

func TestMatchedFormsSkipATokenTheSentenceDoesNotHave(t *testing.T) {
	tokens := []analyzer.Token{{Surface: "mit"}, {Surface: "dem"}, {Surface: "haus"}}

	forms, kept := matchedForms(tokens, []int{-1, 0, 2, 9})

	assert.Equal(t, []string{"mit", "haus"}, forms)
	assert.Equal(t, []int{0, 2}, kept)
}

func TestBetterIdiomChoiceOrdersByQualityThenPosition(t *testing.T) {
	choice := func(gdex float64, score int, offset, ordinal int64) idiomChoice {
		return idiomChoice{quality: sentencequality.SentenceQuality{GDEXScore: gdex, Score: score}, offset: offset, sentenceRef: domain.StudyOccurrenceRef{SentenceOrdinal: ordinal}}
	}
	cases := []struct {
		name string
		a, b idiomChoice
	}{
		{"a higher GDEX score first", choice(2, 1, 9, 9), choice(1, 5, 0, 0)},
		{"a higher quality score next", choice(1, 5, 9, 9), choice(1, 3, 0, 0)},
		{"an earlier offset next", choice(1, 3, 1, 9), choice(1, 3, 2, 0)},
		{"an earlier sentence last", choice(1, 3, 1, 1), choice(1, 3, 1, 2)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.True(t, betterIdiomChoice(c.a, c.b))
			require.False(t, betterIdiomChoice(c.b, c.a))
		})
	}
}
