package idiom

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lookupStub struct {
	entries map[string][]Entry
	err     error
	calls   []string
}

func (s *lookupStub) IdiomEntries(_ context.Context, _, word string) ([]Entry, error) {
	s.calls = append(s.calls, word)
	return s.entries[word], s.err
}

func sentence(lemmas ...string) []analyzer.Token {
	tokens := make([]analyzer.Token, len(lemmas))
	for index, lemma := range lemmas {
		tokens[index] = analyzer.Token{Surface: lemma, CanonicalLemma: lemma}
	}
	return tokens
}

func TestCandidatesInSentenceAssemblesWithoutAnAnchor(t *testing.T) {
	inGamba := testEntry("in gamba", "in", "gamba")
	stub := &lookupStub{entries: map[string][]Entry{"in": {inGamba}, "gamba": {inGamba}}}
	candidates, err := CandidatesInSentence(t.Context(), stub, DefaultParameters(), "it", sentence("un", "ragazzo", "in", "gamba"))
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "in gamba", candidates[0].Entry.Headword)
	assert.Equal(t, []int{2, 3}, candidates[0].MatchedTokenOrdinals)
	assert.Equal(t, []string{"un", "ragazzo", "in", "gamba"}, stub.calls)
}

func TestCandidatesInSentenceEdgeCases(t *testing.T) {
	stub := &lookupStub{}
	candidates, err := CandidatesInSentence(t.Context(), stub, DefaultParameters(), "it", nil)
	require.NoError(t, err)
	assert.Empty(t, candidates)
	assert.Empty(t, stub.calls)

	blank := []analyzer.Token{{Surface: " ", CanonicalLemma: ""}}
	candidates, err = CandidatesInSentence(t.Context(), stub, DefaultParameters(), "it", blank)
	require.NoError(t, err)
	assert.Empty(t, candidates)

	failing := &lookupStub{err: errors.New("index closed")}
	_, err = CandidatesInSentence(t.Context(), failing, DefaultParameters(), "it", sentence("in"))
	require.ErrorContains(t, err, "index closed")
}

func TestCachedIndexReadsEachWordOnce(t *testing.T) {
	stub := &lookupStub{entries: map[string][]Entry{"gamba": {testEntry("in gamba", "in", "gamba")}}}
	cached := NewCachedIndex(stub)
	for range 3 {
		entries, err := cached.IdiomEntries(t.Context(), "it", "gamba")
		require.NoError(t, err)
		require.Len(t, entries, 1)
	}
	assert.Equal(t, []string{"gamba"}, stub.calls)
	_, err := NewCachedIndex(&lookupStub{err: errors.New("closed")}).IdiomEntries(t.Context(), "it", "x")
	require.ErrorContains(t, err, "closed")
}
