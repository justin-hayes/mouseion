package cardexport

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type idiomLookupStub struct {
	entries map[string][]idiom.Entry
	err     error
	calls   []string
}

func (s *idiomLookupStub) IdiomEntries(_ context.Context, _, word string) ([]idiom.Entry, error) {
	s.calls = append(s.calls, word)
	return s.entries[word], s.err
}

func testIdiomEntry() idiom.Entry {
	return idiom.Entry{Language: "it", Headword: "in gamba", UPOS: "ADJ", Words: []string{"in", "gamba"}, Source: "wiktionary", Version: "fixture-v1"}
}

func gambaCandidate() domain.SelectionCandidate {
	return domain.SelectionCandidate{Language: "it", CanonicalLemma: "gamba", UPOS: "NOUN", ObservedForms: []byte(`["gamba"]`)}
}

func gambaTokens() []analyzer.Token {
	return []analyzer.Token{
		{Surface: "un", CanonicalLemma: "un", Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 2}},
		{Surface: "ragazzo", CanonicalLemma: "ragazzo", Location: analyzer.SourceLocation{StartOffset: 3, EndOffset: 10}},
		{Surface: "in", CanonicalLemma: "in", Location: analyzer.SourceLocation{StartOffset: 11, EndOffset: 13}},
		{Surface: "gamba", CanonicalLemma: "gamba", Location: analyzer.SourceLocation{StartOffset: 14, EndOffset: 19}},
	}
}

func acceptedGambaEvidence(sentence string, tokens []analyzer.Token) sentencequality.SentenceEvidence {
	return sentencequality.SentenceEvidence{Sentence: sentence, Target: "gamba", Tokens: tokens, Quality: sentencequality.SentenceQuality{Accepted: true}}
}

// TestIdiomSentencePreferenceDetectsTheTargetInsideACandidate pins the leading
// rule at the seam, including the literal sentence it prefers.
func TestIdiomSentencePreferenceDetectsTheTargetInsideACandidate(t *testing.T) {
	lookup := &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}
	prefer, err := idiomSentencePreferenceFor(t.Context(), lookup, map[string]bool{"it": true}, gambaCandidate())
	require.NoError(t, err)

	t.Run("the idiom sentence is not preferred", func(t *testing.T) {
		require.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
	})
	t.Run("the literal sentence is preferred", func(t *testing.T) {
		require.True(t, prefer(acceptedGambaEvidence("la gamba", []analyzer.Token{
			{Surface: "la", CanonicalLemma: "la", Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 2}},
			{Surface: "gamba", CanonicalLemma: "gamba", Location: analyzer.SourceLocation{StartOffset: 3, EndOffset: 8}},
		})))
	})
}

// TestIdiomSentencePreferenceIsInertWithoutUsableInput pins every fallback that
// keeps the existing sentence choice: switch off, no provider, no entries,
// malformed forms, rejected evidence, and an unanchorable target.
func TestIdiomSentencePreferenceIsInertWithoutUsableInput(t *testing.T) {
	t.Run("switch off", func(t *testing.T) {
		lookup := &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}
		prefer, err := idiomSentencePreferenceFor(t.Context(), lookup, map[string]bool{"de": true}, gambaCandidate())
		require.NoError(t, err)
		assert.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
		assert.Empty(t, lookup.calls)
	})
	t.Run("no idiom provider", func(t *testing.T) {
		prefer, err := idiomSentencePreferenceFor(t.Context(), nil, map[string]bool{"it": true}, gambaCandidate())
		require.NoError(t, err)
		assert.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
	})
	t.Run("no entries", func(t *testing.T) {
		prefer, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{}, map[string]bool{"it": true}, gambaCandidate())
		require.NoError(t, err)
		assert.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
	})
	t.Run("malformed observed forms", func(t *testing.T) {
		candidate := gambaCandidate()
		candidate.ObservedForms = []byte(`{`)
		prefer, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}, map[string]bool{"it": true}, candidate)
		require.NoError(t, err)
		assert.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
	})
	t.Run("rejected evidence", func(t *testing.T) {
		prefer, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}, map[string]bool{"it": true}, gambaCandidate())
		require.NoError(t, err)
		evidence := acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())
		evidence.Quality.Accepted = false
		assert.False(t, prefer(evidence))
	})
	t.Run("unanchorable target", func(t *testing.T) {
		prefer, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}, map[string]bool{"it": true}, gambaCandidate())
		require.NoError(t, err)
		assert.False(t, prefer(sentencequality.SentenceEvidence{Sentence: "un ragazzo in gamba", Target: "gamba", Quality: sentencequality.SentenceQuality{Accepted: true}}))
	})
}

// TestIdiomSentencePreferencePropagatesLookupFailure pins the durable error path.
func TestIdiomSentencePreferencePropagatesLookupFailure(t *testing.T) {
	_, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{err: errors.New("corrupt table")}, map[string]bool{"it": true}, gambaCandidate())
	require.Error(t, err)
	assert.ErrorContains(t, err, "corrupt table")
}

func TestCachedIdiomIndexLooksUpEachWordOnce(t *testing.T) {
	stub := &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}
	cached := newCachedIdiomIndex(stub)
	for range 3 {
		entries, err := idiomEntriesForSentence(context.Background(), cached, "it", gambaTokens())
		require.NoError(t, err)
		require.Len(t, entries, 1)
	}
	require.Equal(t, []string{"un", "ragazzo", "in", "gamba"}, stub.calls, "each word reaches the index once per freeze")

	failing := newCachedIdiomIndex(&idiomLookupStub{err: errors.New("index closed")})
	_, err := failing.IdiomEntries(context.Background(), "it", "gamba")
	require.ErrorContains(t, err, "index closed")
}
