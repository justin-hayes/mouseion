package idiom

import (
	"testing"

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

func testAnchor(ordinal int, lemma string) Anchor {
	return Anchor{
		TokenOrdinal: ordinal,
		Identity: AnchorIdentity{
			Language:       "it",
			CanonicalLemma: lemma,
			UPOS:           "NOUN",
		},
	}
}

type wantCandidate struct {
	headword    string
	matched     []int
	anchorLemma string
}

func TestMatchFindsAnchoredCandidatesAndResolvesOverlaps(t *testing.T) {
	inGamba := testEntry("in gamba", "in", "gamba")
	ragazzoInGamba := testEntry("ragazzo in gamba", "ragazzo", "in", "gamba")
	gambaIn := testEntry("gamba in", "gamba", "in")
	ab := testEntry("a b", "a", "b")
	bc := testEntry("b c", "b", "c")

	tests := []struct {
		name    string
		tokens  []Token
		anchors []Anchor
		entries []Entry
		want    []wantCandidate
	}{
		{
			name:    "contiguous",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")},
			anchors: []Anchor{testAnchor(1, "gamba")},
			entries: []Entry{inGamba},
			want: []wantCandidate{
				{headword: "in gamba", matched: []int{0, 1}, anchorLemma: "gamba"},
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
			anchors: []Anchor{testAnchor(3, "gamba")},
			entries: []Entry{inGamba},
			want: []wantCandidate{
				{headword: "in gamba", matched: []int{0, 3}, anchorLemma: "gamba"},
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
			anchors: []Anchor{testAnchor(4, "gamba")},
			entries: []Entry{inGamba},
			want:    nil,
		},
		{
			name:    "wrong order",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")},
			anchors: []Anchor{testAnchor(1, "gamba")},
			entries: []Entry{gambaIn},
			want:    nil,
		},
		{
			name: "lemma matches an inflected surface",
			tokens: []Token{
				testToken(0, "In", "in"),
				testToken(1, "gambe", "gamba"),
			},
			anchors: []Anchor{testAnchor(1, "gamba")},
			entries: []Entry{inGamba},
			want: []wantCandidate{
				{headword: "in gamba", matched: []int{0, 1}, anchorLemma: "gamba"},
			},
		},
		{
			name: "case-folded surface matches",
			tokens: []Token{
				testToken(0, "IN", "in"),
				testToken(1, "GAMBA", "gamba"),
			},
			anchors: []Anchor{testAnchor(1, "gamba")},
			entries: []Entry{inGamba},
			want: []wantCandidate{
				{headword: "in gamba", matched: []int{0, 1}, anchorLemma: "gamba"},
			},
		},
		{
			name: "decomposed surface matches its composed entry word",
			tokens: []Token{
				testToken(0, "perche\u0301", ""),
				testToken(1, "no", "no"),
			},
			anchors: []Anchor{testAnchor(0, "perché")},
			entries: []Entry{testEntry("perché no", "perché", "no")},
			want: []wantCandidate{
				{headword: "perché no", matched: []int{0, 1}, anchorLemma: "perché"},
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
			anchors: []Anchor{testAnchor(1, "ragazzo"), testAnchor(3, "gamba")},
			entries: []Entry{inGamba, ragazzoInGamba},
			want: []wantCandidate{
				{headword: "ragazzo in gamba", matched: []int{1, 2, 3}, anchorLemma: "ragazzo"},
			},
		},
		{
			name: "overlap tie goes to the earliest position",
			tokens: []Token{
				testToken(0, "a", "a"),
				testToken(1, "b", "b"),
				testToken(2, "c", "c"),
			},
			anchors: []Anchor{testAnchor(0, "a"), testAnchor(2, "c")},
			entries: []Entry{bc, ab},
			want: []wantCandidate{
				{headword: "a b", matched: []int{0, 1}, anchorLemma: "a"},
			},
		},
		{
			name:    "candidate without an anchor is dropped",
			tokens:  []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")},
			anchors: nil,
			entries: []Entry{inGamba},
			want:    nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Match(DefaultParameters(), test.tokens, test.anchors, test.entries)

			require.Len(t, got, len(test.want))
			for index, want := range test.want {
				assert.Equal(t, want.headword, got[index].Entry.Headword)
				assert.Equal(t, want.matched, got[index].MatchedTokenOrdinals)
				assert.Equal(t, want.anchorLemma, got[index].Anchor.CanonicalLemma)
			}
		})
	}
}

func TestMatchRecordsProvenanceForInGambaWithAGambaAnchor(t *testing.T) {
	tokens := []Token{
		testToken(0, "È", "è"),
		testToken(1, "un", "un"),
		testToken(2, "ragazzo", "ragazzo"),
		testToken(3, "in", "in"),
		testToken(4, "gamba", "gamba"),
	}
	gamba := testAnchor(4, "gamba")

	got := Match(DefaultParameters(), tokens, []Anchor{gamba}, []Entry{testEntry("in gamba", "in", "gamba")})

	require.Len(t, got, 1, "È un ragazzo in gamba. anchored on gamba yields one candidate")
	assert.Equal(t, []int{3, 4}, got[0].MatchedTokenOrdinals)
	assert.Equal(t, gamba.Identity, got[0].Anchor)
	assert.Equal(t, 4, got[0].AnchorTokenOrdinal)
	assert.Equal(t, MatcherVersion, got[0].Provenance.MatcherVersion)
	assert.Equal(t, DefaultGapBound, got[0].Provenance.GapBound)
	assert.Equal(t, "wiktionary", got[0].Provenance.Source)
	assert.Equal(t, "fixture-v1", got[0].Provenance.Version)
}

func TestMatchPrefersAnAnchoredLaterMatchOverAnUnanchoredEarlierOne(t *testing.T) {
	tokens := []Token{
		testToken(0, "in", "in"),
		testToken(1, "gamba", "gamba"),
		testToken(2, "di", "di"),
		testToken(3, "nuovo", "nuovo"),
		testToken(4, "in", "in"),
		testToken(5, "gamba", "gamba"),
	}

	got := Match(DefaultParameters(), tokens, []Anchor{testAnchor(5, "gamba")}, []Entry{testEntry("in gamba", "in", "gamba")})

	require.Len(t, got, 1)
	assert.Equal(t, []int{4, 5}, got[0].MatchedTokenOrdinals)
	assert.Equal(t, "gamba", got[0].Anchor.CanonicalLemma)
}

func TestMatchIgnoresEntriesOutsideTheTwoToFourWordBound(t *testing.T) {
	tokens := []Token{testToken(0, "a", "a"), testToken(1, "b", "b")}
	anchors := []Anchor{testAnchor(0, "a")}

	t.Run("too few words", func(t *testing.T) {
		assert.Empty(t, Match(DefaultParameters(), tokens, anchors, []Entry{testEntry("a", "a")}))
	})

	t.Run("too many words", func(t *testing.T) {
		assert.Empty(t, Match(DefaultParameters(), tokens, anchors, []Entry{testEntry("a b c d e", "a", "b", "c", "d", "e")}))
	})
}

func TestMatchWithNoTokensOrEntriesReturnsNothing(t *testing.T) {
	entry := testEntry("in gamba", "in", "gamba")
	anchor := testAnchor(0, "in")

	t.Run("no tokens", func(t *testing.T) {
		assert.Empty(t, Match(DefaultParameters(), nil, []Anchor{anchor}, []Entry{entry}))
	})

	t.Run("no entries", func(t *testing.T) {
		assert.Empty(t, Match(DefaultParameters(), []Token{testToken(0, "in", "in")}, nil, nil))
	})
}

func TestMatchZeroParametersUseTheReviewedDefaults(t *testing.T) {
	tokens := []Token{
		testToken(0, "in", "in"),
		testToken(1, "un", "un"),
		testToken(2, "ragazzo", "ragazzo"),
		testToken(3, "gamba", "gamba"),
	}

	got := Match(Parameters{}, tokens, []Anchor{testAnchor(3, "gamba")}, []Entry{testEntry("in gamba", "in", "gamba")})

	require.Len(t, got, 1)
	assert.Equal(t, MatcherVersion, got[0].Provenance.MatcherVersion)
	assert.Equal(t, DefaultGapBound, got[0].Provenance.GapBound)
	assert.Equal(t, []int{0, 3}, got[0].MatchedTokenOrdinals)
}

func TestMatchNegativeGapBoundClampsToContiguous(t *testing.T) {
	parameters := Parameters{Version: "test", GapBound: -1}
	entry := testEntry("in gamba", "in", "gamba")

	t.Run("a separated pair is outside the clamped bound", func(t *testing.T) {
		tokens := []Token{
			testToken(0, "in", "in"),
			testToken(1, "un", "un"),
			testToken(2, "gamba", "gamba"),
		}

		got := Match(parameters, tokens, []Anchor{testAnchor(2, "gamba")}, []Entry{entry})

		assert.Empty(t, got)
		assert.Equal(t, -1, parameters.GapBound, "the caller's parameters are not mutated")
	})

	t.Run("a contiguous pair still matches", func(t *testing.T) {
		tokens := []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")}

		got := Match(parameters, tokens, []Anchor{testAnchor(1, "gamba")}, []Entry{entry})

		require.Len(t, got, 1)
	})
}

func TestMatchDeduplicatesEntries(t *testing.T) {
	tokens := []Token{testToken(0, "in", "in"), testToken(1, "gamba", "gamba")}
	entry := testEntry("in gamba", "in", "gamba")

	got := Match(DefaultParameters(), tokens, []Anchor{testAnchor(1, "gamba")}, []Entry{entry, entry})

	require.Len(t, got, 1)
}
