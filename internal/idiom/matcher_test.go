package idiom

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testToken(ordinal int, surface, lemma string) Token {
	return Token{Ordinal: ordinal, Surface: surface, CanonicalLemma: lemma}
}

func testEntry(headword string, words ...string) Entry {
	return Entry{
		Language: "it",
		Headword: headword,
		UPOS:     "ADJ",
		Words:    words,
		Source:   "wiktionary",
		Version:  "fixture-v1",
	}
}

type wantCandidate struct {
	headword string
	matched  []int
}

func TestMatchUnanchoredMatchesHeadwordsAndResolvesOverlaps(t *testing.T) {
	inGamba := testEntry("in gamba", "in", "gamba")
	ragazzoInGamba := testEntry("ragazzo in gamba", "ragazzo", "in", "gamba")
	gambaIn := testEntry("gamba in", "gamba", "in")
	ab := testEntry("a b", "a", "b")
	bc := testEntry("b c", "b", "c")

	tests := []struct {
		name    string
		tokens  []Token
		entries []Entry
		want    []wantCandidate
	}{
		{
			name:    "contiguous",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")},
			entries: []Entry{inGamba},
			want: []wantCandidate{
				{headword: "in gamba", matched: []int{0, 1}},
			},
		},
		{
			name: "gap within the bound",
			tokens: []Token{
				testToken(0, "in", "in"),
				testToken(1, "un", "un"),
				testToken(2, "ragazzo", "ragazzo"),
				testToken(3, "gamba", "gamba"),
			},
			entries: []Entry{inGamba},
			want: []wantCandidate{
				{headword: "in gamba", matched: []int{0, 3}},
			},
		},
		{
			name: "gap over the bound",
			tokens: []Token{
				testToken(0, "in", "in"),
				testToken(1, "un", "un"),
				testToken(2, "bel", "bel"),
				testToken(3, "ragazzo", "ragazzo"),
				testToken(4, "gamba", "gamba"),
			},
			entries: []Entry{inGamba},
			want:    nil,
		},
		{
			name:    "wrong order",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")},
			entries: []Entry{gambaIn},
			want:    nil,
		},
		{
			name: "lemma matches an inflected surface",
			tokens: []Token{
				testToken(0, "In", "in"),
				testToken(1, "gambe", "gamba"),
			},
			entries: []Entry{inGamba},
			want: []wantCandidate{
				{headword: "in gamba", matched: []int{0, 1}},
			},
		},
		{
			name: "case-folded surface matches",
			tokens: []Token{
				testToken(0, "IN", "in"),
				testToken(1, "GAMBA", "gamba"),
			},
			entries: []Entry{inGamba},
			want: []wantCandidate{
				{headword: "in gamba", matched: []int{0, 1}},
			},
		},
		{
			name: "decomposed surface matches its composed entry word",
			tokens: []Token{
				testToken(0, "perche\u0301", ""),
				testToken(1, "no", "no"),
			},
			entries: []Entry{testEntry("perché no", "perché", "no")},
			want: []wantCandidate{
				{headword: "perché no", matched: []int{0, 1}},
			},
		},
		{
			name: "overlap goes to the longest headword",
			tokens: []Token{
				testToken(0, "un", "un"),
				testToken(1, "ragazzo", "ragazzo"),
				testToken(2, "in", "in"),
				testToken(3, "gamba", "gamba"),
			},
			entries: []Entry{inGamba, ragazzoInGamba},
			want: []wantCandidate{
				{headword: "ragazzo in gamba", matched: []int{1, 2, 3}},
			},
		},
		{
			name: "overlap tie goes to the earliest position",
			tokens: []Token{
				testToken(0, "a", "a"),
				testToken(1, "b", "b"),
				testToken(2, "c", "c"),
			},
			entries: []Entry{bc, ab},
			want: []wantCandidate{
				{headword: "a b", matched: []int{0, 1}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MatchUnanchored(DefaultParameters(), test.tokens, test.entries)

			require.Len(t, got, len(test.want))
			for index, want := range test.want {
				assert.Equal(t, want.headword, got[index].Entry.Headword)
				assert.Equal(t, want.matched, got[index].MatchedTokenOrdinals)
			}
		})
	}
}

func TestMatchUnanchoredRecordsProvenanceForInGamba(t *testing.T) {
	tokens := []Token{
		testToken(0, "È", "è"),
		testToken(1, "un", "un"),
		testToken(2, "ragazzo", "ragazzo"),
		testToken(3, "in", "in"),
		testToken(4, "gamba", "gamba"),
	}

	got := MatchUnanchored(DefaultParameters(), tokens, []Entry{testEntry("in gamba", "in", "gamba")})

	require.Len(t, got, 1, "È un ragazzo in gamba. yields one candidate")
	assert.Equal(t, []int{3, 4}, got[0].MatchedTokenOrdinals)
	assert.Equal(t, MatcherVersion, got[0].Provenance.MatcherVersion)
	assert.Equal(t, DefaultGapBound, got[0].Provenance.GapBound)
	assert.Equal(t, "wiktionary", got[0].Provenance.Source)
	assert.Equal(t, "fixture-v1", got[0].Provenance.Version)
}

func TestMatchUnanchoredIgnoresEntriesOutsideTheTwoToFourWordBound(t *testing.T) {
	tokens := []Token{testToken(0, "a", "a"), testToken(1, "b", "b")}

	t.Run("too few words", func(t *testing.T) {
		assert.Empty(t, MatchUnanchored(DefaultParameters(), tokens, []Entry{testEntry("a", "a")}))
	})

	t.Run("too many words", func(t *testing.T) {
		assert.Empty(t, MatchUnanchored(DefaultParameters(), tokens, []Entry{testEntry("a b c d e", "a", "b", "c", "d", "e")}))
	})
}

func TestMatchUnanchoredZeroParametersUseTheReviewedDefaults(t *testing.T) {
	tokens := []Token{
		testToken(0, "in", "in"),
		testToken(1, "un", "un"),
		testToken(2, "ragazzo", "ragazzo"),
		testToken(3, "gamba", "gamba"),
	}

	got := MatchUnanchored(Parameters{}, tokens, []Entry{testEntry("in gamba", "in", "gamba")})

	require.Len(t, got, 1)
	assert.Equal(t, MatcherVersion, got[0].Provenance.MatcherVersion)
	assert.Equal(t, DefaultGapBound, got[0].Provenance.GapBound)
	assert.Equal(t, []int{0, 3}, got[0].MatchedTokenOrdinals)
}

func TestMatchUnanchoredNegativeGapBoundClampsToContiguous(t *testing.T) {
	parameters := Parameters{Version: "test", GapBound: -1}
	entry := testEntry("in gamba", "in", "gamba")

	t.Run("a separated pair is outside the clamped bound", func(t *testing.T) {
		tokens := []Token{
			testToken(0, "in", "in"),
			testToken(1, "un", "un"),
			testToken(2, "gamba", "gamba"),
		}

		got := MatchUnanchored(parameters, tokens, []Entry{entry})

		assert.Empty(t, got)
		assert.Equal(t, -1, parameters.GapBound, "the caller's parameters are not mutated")
	})

	t.Run("a contiguous pair still matches", func(t *testing.T) {
		tokens := []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")}

		got := MatchUnanchored(parameters, tokens, []Entry{entry})

		require.Len(t, got, 1)
	})
}

func TestMatchUnanchoredDeduplicatesEntries(t *testing.T) {
	tokens := []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")}
	entry := testEntry("in gamba", "in", "gamba")

	got := MatchUnanchored(DefaultParameters(), tokens, []Entry{entry, entry})

	require.Len(t, got, 1)
}

func TestMatchUnanchoredPoolsTheSensesOfEveryPartOfSpeechForOneHeadword(t *testing.T) {
	// Wiktionary lists di fronte as an adjective ("front, fore") and an adverb
	// ("opposite", "in front of"). Card 4 of the 2026-11-08 Italian review was
	// offered only the adjective's senses and glossed "front".
	tokens := []Token{
		testToken(0, "trovate", "trovare"), testToken(1, "di", "di"), testToken(2, "fronte", "fronte"),
	}
	adjective := testEntry("di fronte", "di", "fronte")
	adjective.Senses = []enrichment.LexicalSense{{Gloss: "front, fore"}}
	adverb := testEntry("di fronte", "di", "fronte")
	adverb.UPOS = "ADV"
	adverb.Senses = []enrichment.LexicalSense{{Gloss: "opposite (directly across)"}, {Gloss: "in front of"}}
	wantGlosses := []string{"front, fore", "opposite (directly across)", "in front of"}

	for name, entries := range map[string][]Entry{
		"adjective first": {adjective, adverb},
		"adverb first":    {adverb, adjective},
	} {
		t.Run(name, func(t *testing.T) {
			got := MatchUnanchored(DefaultParameters(), tokens, entries)

			require.Len(t, got, 1, "one headword in one sentence is one candidate")
			glosses := make([]string, 0, len(got[0].Entry.Senses))
			for _, sense := range got[0].Entry.Senses {
				glosses = append(glosses, sense.Gloss)
			}
			assert.Equal(t, wantGlosses, glosses, "senses are pooled in part-of-speech order")
			assert.Equal(t, []int{1, 2}, got[0].MatchedTokenOrdinals)
		})
	}
}

func TestMatchUnanchoredKeepsEntriesFromDifferentSourcesApart(t *testing.T) {
	tokens := []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")}
	wiktionary := testEntry("in gamba", "in", "gamba")
	wiktionary.Senses = []enrichment.LexicalSense{{Gloss: "capable"}}
	reviewed := testEntry("in gamba", "in", "gamba")
	reviewed.Source = "reviewed"
	reviewed.Senses = []enrichment.LexicalSense{{Gloss: "on the ball"}}

	got := MatchUnanchored(DefaultParameters(), tokens, []Entry{reviewed, wiktionary})

	require.Len(t, got, 1, "overlap resolution still keeps one candidate")
	assert.Equal(t, "reviewed", got[0].Entry.Source)
	require.Len(t, got[0].Entry.Senses, 1, "senses are not pooled across sources")
	assert.Equal(t, "on the ball", got[0].Entry.Senses[0].Gloss)
}

func TestMatchUnanchoredFindsEveryNonOverlappingCandidate(t *testing.T) {
	inGamba := testEntry("in gamba", "in", "gamba")
	ragazzoInGamba := testEntry("ragazzo in gamba", "ragazzo", "in", "gamba")
	gambaIn := testEntry("gamba in", "gamba", "in")
	ab := testEntry("a b", "a", "b")
	bc := testEntry("b c", "b", "c")
	cd := testEntry("c d", "c", "d")

	tests := []struct {
		name    string
		tokens  []Token
		entries []Entry
		want    []wantCandidate
	}{
		{
			name:    "contiguous",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")},
			entries: []Entry{inGamba},
			want:    []wantCandidate{{headword: "in gamba", matched: []int{0, 1}}},
		},
		{
			name:    "longest headword wins an overlap",
			tokens:  []Token{testToken(0, "ragazzo", "ragazzo"), testToken(1, "in", "in"), testToken(2, "gamba", "gamba")},
			entries: []Entry{inGamba, ragazzoInGamba},
			want:    []wantCandidate{{headword: "ragazzo in gamba", matched: []int{0, 1, 2}}},
		},
		{
			name:    "equal length overlap goes to the earlier position",
			tokens:  []Token{testToken(0, "a", "a"), testToken(1, "b", "b"), testToken(2, "c", "c")},
			entries: []Entry{bc, ab},
			want:    []wantCandidate{{headword: "a b", matched: []int{0, 1}}},
		},
		{
			name:    "disjoint candidates are both returned in sentence order",
			tokens:  []Token{testToken(0, "a", "a"), testToken(1, "b", "b"), testToken(2, "c", "c"), testToken(3, "d", "d")},
			entries: []Entry{cd, ab},
			want: []wantCandidate{
				{headword: "a b", matched: []int{0, 1}},
				{headword: "c d", matched: []int{2, 3}},
			},
		},
		{
			name:    "gap bound and word order apply",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "x", "x"), testToken(2, "y", "y"), testToken(3, "gamba", "gamba")},
			entries: []Entry{inGamba, gambaIn},
			want:    []wantCandidate{{headword: "in gamba", matched: []int{0, 3}}},
		},
		{
			name:    "too many gap tokens",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "x", "x"), testToken(2, "y", "y"), testToken(3, "z", "z"), testToken(4, "gamba", "gamba")},
			entries: []Entry{inGamba},
		},
		{
			name:    "an effective lemma matches the headword",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "gambe", "gamba")},
			entries: []Entry{inGamba},
			want:    []wantCandidate{{headword: "in gamba", matched: []int{0, 1}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MatchUnanchored(DefaultParameters(), test.tokens, test.entries)
			require.Len(t, got, len(test.want))
			for index, want := range test.want {
				assert.Equal(t, want.headword, got[index].Entry.Headword)
				assert.Equal(t, want.matched, got[index].MatchedTokenOrdinals)
				assert.Equal(t, MatcherVersion, got[index].Provenance.MatcherVersion)
			}
		})
	}
}

func TestMatchUnanchoredEdgeCases(t *testing.T) {
	entry := testEntry("in gamba", "in", "gamba")
	assert.Nil(t, MatchUnanchored(DefaultParameters(), nil, []Entry{entry}))
	assert.Nil(t, MatchUnanchored(DefaultParameters(), []Token{testToken(0, "in", "in")}, nil))
	tokens := []Token{testToken(0, "in", "in"), testToken(1, "x", "x"), testToken(2, "gamba", "gamba")}
	zero := MatchUnanchored(Parameters{}, tokens, []Entry{entry})
	require.Len(t, zero, 1)
	assert.Equal(t, DefaultGapBound, zero[0].Provenance.GapBound)
	assert.Empty(t, MatchUnanchored(Parameters{Version: "v", GapBound: -1}, tokens, []Entry{entry}))
}
