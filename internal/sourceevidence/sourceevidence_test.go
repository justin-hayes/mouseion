package sourceevidence

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

type lexicon struct {
	greek     enrichment.AncientGreekLSJEntry
	greekHit  bool
	latin     map[string][]enrichment.LatinLSEntry
	latinCall int
}

func (l *lexicon) Name() string    { return "stub" }
func (l *lexicon) Version() string { return "stub-v1" }
func (l *lexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (l *lexicon) LookupAncientGreekLSJ(context.Context, string) (enrichment.AncientGreekLSJEntry, bool) {
	return l.greek, l.greekHit
}
func (l *lexicon) LookupLatinLS(_ context.Context, lemma string) ([]enrichment.LatinLSEntry, bool) {
	l.latinCall++
	entries, ok := l.latin[lemma]
	return entries, ok
}

func TestGreekOutlineFlagsCitedSenses(t *testing.T) {
	lex := &lexicon{greekHit: true, greek: enrichment.AncientGreekLSJEntry{
		EntryID: "n1", Headword: "logos", Pin: "pin-1",
		Senses: []enrichment.AncientGreekLSJSense{
			{ID: "n1.0", Number: "A", Level: 1, Senses: []enrichment.AncientGreekLSJSense{
				{ID: "n1.1", Number: "I", Level: 2, Definitions: []string{"word"}},
				{ID: "n1.2", Number: "II", Level: 2, Glosses: []string{"only a gloss"}},
			}},
			{ID: "n1.3", Number: "B", Level: 1, Definitions: []string{"reason"}},
		},
	}}
	cited := map[string]struct{}{"n1.1": {}, "n1.3": {}, "missing": {}}

	resolution, ok := Resolve(t.Context(), lex, cardpolicy.For("grc"), "logos", cited)
	require.True(t, ok)
	assert.Equal(t, "n1", resolution.EntryID)
	assert.Equal(t, "pin-1", resolution.Pin)
	require.Len(t, resolution.Entries, 1)
	assert.Equal(t, []OutlineSense{
		{ID: "n1.0", Number: "A", Level: 1},
		{ID: "n1.1", Number: "I", Level: 2, Definitions: []string{"word"}, Cited: true},
		{ID: "n1.3", Number: "B", Level: 1, Definitions: []string{"reason"}, Cited: true},
	}, resolution.Entries[0].Outline, "a heading is kept for its span; only cited IDs are flagged")

	assert.Equal(t, []string{"n1.0", "n1.1", "n1.2", "n1.3"}, senseIDs(resolution.Entries[0].Senses))

	plain, ok := Resolve(t.Context(), lex, cardpolicy.For("grc"), "logos", nil)
	require.True(t, ok)
	for _, sense := range plain.Entries[0].Outline {
		assert.False(t, sense.Cited)
	}
}

func TestLatinStackedEntriesCarryTheirLSNumbersAndOutlines(t *testing.T) {
	lex := &lexicon{latin: map[string][]enrichment.LatinLSEntry{"dico": {
		{EntryID: "n9", Headword: "dico", Homograph: 1, Pin: "ls-pin", Senses: []enrichment.LatinLSSense{
			{ID: "n9.e", Level: 0},
			{ID: "n9.0", Number: "I", Level: 1, Definitions: []string{"to dedicate"}},
		}},
		{EntryID: "n10", Headword: "dico", Homograph: 2, Pin: "ls-pin", Senses: []enrichment.LatinLSSense{
			{ID: "n10.0", Number: "I", Level: 1, Definitions: []string{"to say"}},
		}},
		{},
	}}}
	cited := map[string]struct{}{"n10.0": {}}

	resolution, ok := Resolve(t.Context(), lex, cardpolicy.For("la"), "dico", cited)
	require.True(t, ok)
	assert.Equal(t, "n9,n10", resolution.EntryID)
	assert.Equal(t, "ls-pin", resolution.Pin)
	require.Len(t, resolution.Entries, 2, "an empty entry is dropped")
	assert.Equal(t, 1, resolution.Entries[0].Number)
	assert.Equal(t, 2, resolution.Entries[1].Number)
	assert.Equal(t, []OutlineSense{{ID: "n9.0", Number: "I", Level: 1, Definitions: []string{"to dedicate"}}}, resolution.Entries[0].Outline)
	assert.Equal(t, []OutlineSense{{ID: "n10.0", Number: "I", Level: 1, Definitions: []string{"to say"}, Cited: true}}, resolution.Entries[1].Outline)
}

func TestLatinSingleEntryHasNoNumberAndSenselessEntryOutlinesItsTarget(t *testing.T) {
	lex := &lexicon{latin: map[string][]enrichment.LatinLSEntry{
		"dicto": {{EntryID: "n11", Headword: "dicto", Senses: []enrichment.LatinLSSense{
			{ID: "n11.0", Level: 0, CrossRefs: []enrichment.LatinLSCrossRef{{Text: "v. dico", EntryID: "n9", Headword: "dico"}}},
		}}},
		"dico": {{EntryID: "n9", Headword: "dico", Senses: []enrichment.LatinLSSense{
			{ID: "n9.0", Number: "I", Level: 1, Definitions: []string{"to say"}},
		}}},
	}}
	resolution, ok := Resolve(t.Context(), lex, cardpolicy.For("la"), "dicto", map[string]struct{}{"n9.0": {}})
	require.True(t, ok)
	require.Len(t, resolution.Entries, 1)
	entry := resolution.Entries[0]
	assert.Zero(t, entry.Number)
	assert.Empty(t, entry.Outline)
	assert.Equal(t, []Target{{Reference: "v. dico", Outline: []OutlineSense{
		{ID: "n9.0", Number: "I", Level: 1, Definitions: []string{"to say"}, Cited: true},
	}}}, entry.Targets)
}

func TestResolveFindsNothingWithoutADictionaryOrCapability(t *testing.T) {
	_, ok := Resolve(t.Context(), nil, cardpolicy.For("la"), "dico", nil)
	assert.False(t, ok)
	_, ok = Resolve(t.Context(), &lexicon{}, cardpolicy.For("de"), "Haus", nil)
	assert.False(t, ok, "a policy with no dictionary resolves nothing")
	_, ok = Resolve(t.Context(), &lexicon{greekHit: false}, cardpolicy.For("grc"), "x", nil)
	assert.False(t, ok)
	_, ok = Resolve(t.Context(), &lexicon{latin: map[string][]enrichment.LatinLSEntry{"x": {{}}}}, cardpolicy.For("la"), "x", nil)
	assert.False(t, ok, "only empty entries resolve nothing")
}

func TestResolvePassageWithholdsAPartialPassageAndAnUntranslatedPolicy(t *testing.T) {
	translations := map[string]string{"1.2.17": "First.", "1.2.18": "Second."}
	got := ResolvePassage(cardpolicy.For("grc"), []string{"1.2.17", "1.2.18"}, translations, "pin")
	assert.Equal(t, "First. Second.", got.Text)
	assert.Equal(t, "pin", got.Pin)
	assert.NotEmpty(t, got.Citation)

	assert.True(t, ResolvePassage(cardpolicy.For("grc"), []string{"1.2.17", "1.2.99"}, translations, "pin").Empty())
	assert.True(t, ResolvePassage(cardpolicy.For("de"), []string{"1.2.17"}, translations, "pin").Empty())
}

func senseIDs(senses []Sense) []string {
	ids := make([]string, 0, len(senses))
	for _, sense := range senses {
		ids = append(ids, sense.ID)
	}
	return ids
}
