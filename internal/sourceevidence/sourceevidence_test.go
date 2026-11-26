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
	// greekEntries are the LSJ homographs of the lemma, for the every-entry lookup.
	greekEntries []enrichment.AncientGreekLSJEntry
}

func (l *lexicon) Name() string    { return "stub" }
func (l *lexicon) Version() string { return "stub-v1" }
func (l *lexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (l *lexicon) LookupAncientGreekLSJ(context.Context, string) (enrichment.AncientGreekLSJEntry, bool) {
	return l.greek, l.greekHit
}
func (l *lexicon) LookupAncientGreekLSJEntries(context.Context, string) ([]enrichment.AncientGreekLSJEntry, bool) {
	return l.greekEntries, len(l.greekEntries) > 0
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
	cited := map[SenseRef]struct{}{{"n1", "n1.1"}: {}, {"n1", "n1.3"}: {}, {"n1", "missing"}: {}, {"other", "n1.2"}: {}}

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
	cited := map[SenseRef]struct{}{{"n10", "n10.0"}: {}}

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
	resolution, ok := Resolve(t.Context(), lex, cardpolicy.For("la"), "dicto", map[SenseRef]struct{}{{"n9", "n9.0"}: {}})
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

type translations struct {
	byBook map[string]map[string]string
	pin    string
	err    error
	calls  int
}

func (t *translations) BookReferenceTranslations(_ context.Context, _, bookID string) (map[string]string, string, error) {
	t.calls++
	return t.byBook[bookID], t.pin, t.err
}

func TestResolvePassageOfBookReadsTheBooksTranslationsFromTheStore(t *testing.T) {
	store := &translations{pin: "sha256:abc", byBook: map[string]map[string]string{"book-1": {"1.2.17": "First.", "1.2.18": "Second."}}}
	got, err := ResolvePassageOfBook(t.Context(), store, cardpolicy.For("grc"), "owner", "book-1", []string{"1.2.17", "1.2.18"})
	require.NoError(t, err)
	want := ResolvePassage(cardpolicy.For("grc"), []string{"1.2.17", "1.2.18"}, store.byBook["book-1"], "sha256:abc")
	assert.Equal(t, want, got, "the store seam resolves the same passage the argument form does")
	assert.Equal(t, "First. Second.", got.Text)
	assert.Equal(t, "sha256:abc", got.Pin)

	none, err := ResolvePassageOfBook(t.Context(), store, cardpolicy.For("grc"), "owner", "book-2", []string{"1.2.17"})
	require.NoError(t, err)
	assert.True(t, none.Empty(), "a Book with no translations has no passage")
}

func TestResolvePassageOfBookAsksNothingWhenNoPassageApplies(t *testing.T) {
	store := &translations{pin: "p", byBook: map[string]map[string]string{"b": {"1.1": "x"}}}
	got, err := ResolvePassageOfBook(t.Context(), store, cardpolicy.For("de"), "owner", "b", []string{"1.1"})
	require.NoError(t, err)
	assert.True(t, got.Empty())
	got, err = ResolvePassageOfBook(t.Context(), store, cardpolicy.For("grc"), "owner", "b", nil)
	require.NoError(t, err)
	assert.True(t, got.Empty())
	got, err = ResolvePassageOfBook(t.Context(), nil, cardpolicy.For("grc"), "owner", "b", []string{"1.1"})
	require.NoError(t, err)
	assert.True(t, got.Empty())
	assert.Zero(t, store.calls)
}

func TestResolvePassageOfBookReturnsAStoreFailure(t *testing.T) {
	_, err := ResolvePassageOfBook(t.Context(), &translations{err: assert.AnError}, cardpolicy.For("grc"), "owner", "b", []string{"1.1"})
	require.ErrorIs(t, err, assert.AnError)
}

func TestLatinPassSensesRepresentEveryEntryWithinTheCap(t *testing.T) {
	first := enrichment.LatinLSEntry{EntryID: "n1", Senses: nil}
	for i := range 9 {
		first.Senses = append(first.Senses, enrichment.LatinLSSense{ID: "n1." + string(rune('a'+i)), Level: 1, Definitions: []string{"d"}})
	}
	second := enrichment.LatinLSEntry{EntryID: "n2", Senses: []enrichment.LatinLSSense{
		{ID: "n2.0", Level: 1, Glosses: []string{"only a gloss"}},
		{ID: "n2.1", Level: 1},
	}}
	senses := LatinPassSenses(BuildLewisShort([]enrichment.LatinLSEntry{first, second}, nil, nil))
	require.Len(t, senses, enrichment.DefaultMaxCandidateSenses)
	assert.Equal(t, enrichment.EntryEvidenceID("n1", "n1.a"), senses[0].EvidenceID, "source order is kept")
	last := senses[len(senses)-1]
	assert.Equal(t, enrichment.EntryEvidenceID("n2", "n2.0"), last.EvidenceID, "a later entry is not starved, and a glosses-only sense is sent")
	assert.Equal(t, "only a gloss", last.Gloss)
}

func TestSharedSenseIDIsFlaggedOnlyOnTheCitedHomograph(t *testing.T) {
	shared := func(id string) enrichment.LatinLSEntry {
		return enrichment.LatinLSEntry{EntryID: id, Headword: "sum", Pin: "pin", Senses: []enrichment.LatinLSSense{
			{ID: "s1", Number: "I", Level: 1, Definitions: []string{"to be " + id}},
		}}
	}
	lex := &lexicon{latin: map[string][]enrichment.LatinLSEntry{"sum": {shared("n1"), shared("n2")}}}
	resolution, ok := Resolve(t.Context(), lex, cardpolicy.For("la"), "sum", map[SenseRef]struct{}{{"n2", "s1"}: {}})
	require.True(t, ok)
	require.Len(t, resolution.Entries, 2)
	assert.False(t, resolution.Entries[0].Outline[0].Cited)
	assert.True(t, resolution.Entries[1].Outline[0].Cited)
}

func greekHomographs() *lexicon {
	return &lexicon{
		greekHit: false, // the deck's exactly-one lookup finds nothing for a homograph
		greekEntries: []enrichment.AncientGreekLSJEntry{
			{EntryID: "n1", Headword: "λέγω", Pin: "pin-1", Senses: []enrichment.AncientGreekLSJSense{{ID: "n1.0", Number: "A", Level: 1, Definitions: []string{"lay"}}}},
			{EntryID: "n2", Headword: "λέγω", Pin: "pin-1", Senses: []enrichment.AncientGreekLSJSense{{ID: "n2.0", Number: "A", Level: 1, Definitions: []string{"say"}}, {ID: "n2.1", Number: "B", Level: 1, Glosses: []string{"speak"}}}},
		},
	}
}

func TestResolveStudyStacksEveryGreekHomographWhereTheDeckTakesNone(t *testing.T) {
	lex := greekHomographs()
	policy := cardpolicy.For("grc")

	_, deckFound := Resolve(t.Context(), lex, policy, "λέγω", nil)
	assert.False(t, deckFound, "the Prepared deck keeps its exactly-one lookup")

	cited := map[SenseRef]struct{}{{"n2", "n2.0"}: {}}
	resolution, found := ResolveStudy(t.Context(), lex, policy, "λέγω", cited)
	require.True(t, found)
	require.Len(t, resolution.Entries, 2)
	assert.Equal(t, "n1,n2", resolution.EntryID)
	assert.Equal(t, "pin-1", resolution.Pin)
	assert.Equal(t, []int{1, 2}, []int{resolution.Entries[0].Number, resolution.Entries[1].Number}, "labelled by position")
	assert.False(t, resolution.Entries[0].Outline[0].Cited)
	assert.True(t, resolution.Entries[1].Outline[0].Cited, "the cited sense is flagged in its own entry")
}

func TestResolveStudyLeavesALoneGreekEntryUnnumbered(t *testing.T) {
	lex := greekHomographs()
	lex.greekEntries = lex.greekEntries[:1]
	resolution, found := ResolveStudy(t.Context(), lex, cardpolicy.For("grc"), "λέγω", nil)
	require.True(t, found)
	require.Len(t, resolution.Entries, 1)
	assert.Zero(t, resolution.Entries[0].Number)
}

func TestResolveStudyFindsNothingWithoutTheEntriesCapability(t *testing.T) {
	_, found := ResolveStudy(t.Context(), onlyDeckGreek{}, cardpolicy.For("grc"), "λέγω", nil)
	assert.False(t, found)
}

// onlyDeckGreek has the deck's one-entry LSJ capability and not Study's.
type onlyDeckGreek struct{}

func (onlyDeckGreek) Name() string    { return "stub" }
func (onlyDeckGreek) Version() string { return "stub-v1" }
func (onlyDeckGreek) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (onlyDeckGreek) LookupAncientGreekLSJ(context.Context, string) (enrichment.AncientGreekLSJEntry, bool) {
	return enrichment.AncientGreekLSJEntry{EntryID: "n1", Headword: "x", Senses: []enrichment.AncientGreekLSJSense{{ID: "n1.0"}}}, true
}

func TestPassSensesCarryTheirEntryForGreekAndLatin(t *testing.T) {
	resolution, found := ResolveStudy(t.Context(), greekHomographs(), cardpolicy.For("grc"), "λέγω", nil)
	require.True(t, found)
	senses := PassSenses(cardpolicy.For("grc"), resolution.Entries)
	assert.Equal(t, []enrichment.LexicalSense{
		{EvidenceID: enrichment.EntryEvidenceID("n1", "n1.0"), Source: "lsj", Gloss: "lay"},
		{EvidenceID: enrichment.EntryEvidenceID("n2", "n2.0"), Source: "lsj", Gloss: "say"},
	}, senses, "an entry sends its definitions, and its glosses only when none has any")
}

func TestPassSensesGreekSendsGlossesForAnEntryWithNoDefinitions(t *testing.T) {
	entry := BuildLSJ(enrichment.AncientGreekLSJEntry{EntryID: "n5", Senses: []enrichment.AncientGreekLSJSense{{ID: "n5.0", Glosses: []string{"old"}}}}, nil)
	senses := PassSenses(cardpolicy.For("grc"), []Entry{entry})
	require.Len(t, senses, 1)
	assert.Equal(t, "old", senses[0].Gloss)
}

func TestPassSensesGreekSendsEverySenseOfEveryEntry(t *testing.T) {
	long := enrichment.AncientGreekLSJEntry{EntryID: "n1"}
	for i := range 10 {
		long.Senses = append(long.Senses, enrichment.AncientGreekLSJSense{ID: "n1." + string(rune('a'+i)), Definitions: []string{"long"}})
	}
	short := enrichment.AncientGreekLSJEntry{EntryID: "n2", Senses: []enrichment.AncientGreekLSJSense{{ID: "n2.0", Definitions: []string{"short"}}}}
	senses := PassSenses(cardpolicy.For("grc"), []Entry{BuildLSJ(long, nil), BuildLSJ(short, nil)})
	require.Len(t, senses, 11, "Greek Study keeps its uncapped senses")
	assert.Equal(t, enrichment.EntryEvidenceID("n2", "n2.0"), senses[10].EvidenceID)
}

func TestResolveStudyDropsEmptyEntriesAndFindsNothingWhenNoneIsLeft(t *testing.T) {
	lex := greekHomographs()
	lex.greekEntries = []enrichment.AncientGreekLSJEntry{{}, lex.greekEntries[0], {}}
	resolution, found := ResolveStudy(t.Context(), lex, cardpolicy.For("grc"), "λέγω", nil)
	require.True(t, found)
	assert.Len(t, resolution.Entries, 1, "an empty entry is never stacked")
	assert.Zero(t, resolution.Entries[0].Number)

	lex.greekEntries = []enrichment.AncientGreekLSJEntry{{}}
	_, found = ResolveStudy(t.Context(), lex, cardpolicy.For("grc"), "λέγω", nil)
	assert.False(t, found)
}
