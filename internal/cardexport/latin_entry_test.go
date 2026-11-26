package cardexport

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testLSPin = "56061ca127f4a2844980baffc5f2b6d1332897b3"

// lsStub is a lexical provider that also answers the Lewis & Short capability.
type lsStub struct {
	entries []enrichment.LatinLSEntry
	found   bool
}

func (s *lsStub) Name() string    { return "ls-stub" }
func (s *lsStub) Version() string { return "ls-stub-v1" }
func (s *lsStub) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (s *lsStub) LookupLatinLS(context.Context, string) ([]enrichment.LatinLSEntry, bool) {
	return s.entries, s.found
}

func dicoEntries() []enrichment.LatinLSEntry {
	return []enrichment.LatinLSEntry{
		{
			EntryID: "n9", Headword: "dico", Orth: "dīco", Pin: testLSPin,
			Senses: []enrichment.LatinLSSense{
				{ID: "n9.e", Level: 0, Citations: []enrichment.LatinLSCitation{{Author: "Liv.", Text: "Liv. 1, 1"}}},
				{ID: "n9.0", Number: "I", Level: 1, Definitions: []string{"to dedicate"}, Glosses: []string{"to dedicate", "to devote"},
					Citations: []enrichment.LatinLSCitation{{Author: "Liv.", Text: "Liv. 2, 3"}, {Author: "Cic.", Text: "Cic. Att. 1, 2"}, {Text: "4, 5"}}},
				{ID: "n9.1", Number: "A", Level: 2, Glosses: []string{"in particular"}},
			},
		},
		{
			EntryID: "n10", Headword: "dico", Orth: "dīco", Pin: testLSPin,
			Senses: []enrichment.LatinLSSense{{ID: "n10.0", Number: "I", Level: 1, Definitions: []string{"to say"}, Glosses: []string{"to say"}}},
		},
	}
}

func latinFreezeProjection(author string) []CandidateProjection {
	return []CandidateProjection{{
		OwnerID: "owner-1", DeckName: "Livy",
		Candidate:  domain.SelectionCandidate{Language: "la", CanonicalLemma: "dico", UPOS: "VERB"},
		Entry:      Entry{Language: "la", CanonicalLemma: "dico", UPOS: "VERB", Headword: "dico", Sentence: "Dico.", TargetWord: "dico"},
		WorkAuthor: author, SkipMeaningEvidence: true,
	}}
}

func TestLatinCardStacksEveryEntryWithItsOwnOutlineAndFullEntry(t *testing.T) {
	stub := &lsStub{found: true, entries: dicoEntries()}
	entries, err := freezeEntries(t.Context(), stub, nil, latinFreezeProjection("Livy"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	entry := entries[0]
	assert.Equal(t, "n9,n10", entry.DictionaryEntryID, "the frozen item records every entry ID")
	assert.Equal(t, testLSPin, entry.DictionaryPin)
	assert.Empty(t, entry.CandidateSenses)

	html := entry.DictionaryEntry
	assert.Equal(t, 2, strings.Count(html, `<div class="dictionary-entry ls-entry">`))
	assert.Equal(t, 2, strings.Count(html, "Full Lewis &amp; Short entry"), "each entry has its own collapsed full entry")
	assert.Equal(t, 2, strings.Count(html, `<div class="dict-outline">`))
	first, second := strings.Index(html, "dīco<sup>1</sup>"), strings.Index(html, "dīco<sup>2</sup>")
	require.NotEqual(t, -1, first)
	require.NotEqual(t, -1, second)
	assert.Less(t, first, second, "entries keep id order")
	assert.NotContains(t, html, "lsj-")

	note, err := makeNote("owner-1", renderInputFromEntry(entry))
	require.NoError(t, err)
	assert.Equal(t, html, note.DictionaryEntry)
}

func TestLatinFullEntryKeepsOnlyTheWorkAuthorsCitations(t *testing.T) {
	stub := &lsStub{found: true, entries: dicoEntries()[:1]}
	entries, err := freezeEntries(t.Context(), stub, nil, latinFreezeProjection("Titus Livius (Livy)"))
	require.NoError(t, err)
	html := entries[0].DictionaryEntry
	assert.Contains(t, html, "Liv. 2, 3")
	assert.Contains(t, html, "Liv. 1, 1", "an entry-level citation of the author is kept")
	assert.NotContains(t, html, "Cic. Att.")
	assert.NotContains(t, html, "4, 5", "a bibl with no author reference is dropped")
	assert.NotContains(t, html, "dict-headword\">dīco<sup", "a single entry carries no number")
}

func TestLatinAuthorMatchesCTSURNAuthorAndAbbreviations(t *testing.T) {
	assert.True(t, latinAuthorMatches("phi0474", "phi0474", ""))
	assert.True(t, latinAuthorMatches("Cic.", "Marcus Tullius Cicero", ""))
	assert.True(t, latinAuthorMatches("Liv.", "Livy", ""))
	assert.False(t, latinAuthorMatches("Cic.", "Livy", ""))
	assert.False(t, latinAuthorMatches("", "Livy", ""))
	assert.False(t, latinAuthorMatches("Liv.", "", ""))
	assert.False(t, latinAuthorMatches("L.", "Livy", ""), "an abbreviation under three letters never matches by prefix")
}

func TestLatinEntryWithNoSensesShowsOnlyItsHeadLine(t *testing.T) {
	html := renderLatinLSEntries([]enrichment.LatinLSEntry{{EntryID: "n11", Headword: "dicto", Orth: "dictō", Pin: testLSPin}}, nil, 0, "", "")
	assert.Contains(t, html, "dictō")
	assert.NotContains(t, html, "dict-outline")
	assert.NotContains(t, html, "Full Lewis")
	assert.NotContains(t, html, "dict-sense")
}

func TestLatinCardWithNoMatchOmitsTheDictionaryBlock(t *testing.T) {
	stub := &lsStub{}
	entries, err := freezeEntries(t.Context(), stub, nil, latinFreezeProjection("Livy"))
	require.NoError(t, err)
	assert.Empty(t, entries[0].DictionaryEntry)
	assert.Empty(t, entries[0].DictionaryEntryID)
	note, err := makeNote("owner-1", renderInputFromEntry(entries[0]))
	require.NoError(t, err)
	assert.Empty(t, note.DictionaryEntry)
}

func TestLatinEntryEscapesSourceText(t *testing.T) {
	html := renderLatinLSEntries([]enrichment.LatinLSEntry{{EntryID: "n1", Orth: "a<b>", Senses: []enrichment.LatinLSSense{{ID: "n1.0", Level: 1, Glosses: []string{"x <i>y</i> {z}"}}}}}, nil, 0, "", "")
	assert.NotContains(t, html, "<i>y</i>")
	assert.Contains(t, html, "x &lt;i&gt;y&lt;/i&gt; &#123;z&#125;")
}

func TestLatinFrozenEntriesKeepTheirTextAfterTheIndexChanges(t *testing.T) {
	stub := &lsStub{found: true, entries: dicoEntries()}
	entries, err := freezeEntries(t.Context(), stub, nil, latinFreezeProjection("Livy"))
	require.NoError(t, err)
	frozen := renderInputFromEntry(entries[0])
	stub.entries = dicoEntries()[1:] // the index is re-derived with one entry
	again, err := makeNote("owner-1", frozen)
	require.NoError(t, err)
	assert.Contains(t, again.DictionaryEntry, "dīco<sup>1</sup>")
	assert.Contains(t, again.DictionaryEntry, "dīco<sup>2</sup>")
}

func TestLatinEntryNumbersAreTheSourcesHomographNumbers(t *testing.T) {
	entries := dicoEntries()
	entries[0].Homograph, entries[1].Homograph = 2, 5
	html := renderLatinLSEntries(entries, nil, cardpolicy.CitationsNone, "Livy", "")
	assert.Contains(t, html, "dīco<sup>2</sup>")
	assert.Contains(t, html, "dīco<sup>5</sup>")
	assert.NotContains(t, html, "dīco<sup>1</sup>")

	entries[0].Homograph, entries[1].Homograph = 0, 0
	html = renderLatinLSEntries(entries, nil, cardpolicy.CitationsNone, "Livy", "")
	assert.Contains(t, html, "dīco<sup>1</sup>", "an entry whose key has no number keeps its place in id order")
	assert.Contains(t, html, "dīco<sup>2</sup>")

	html = renderLatinLSEntries(entries[:1], nil, cardpolicy.CitationsNone, "Livy", "")
	assert.NotContains(t, html, "<sup>", "a single entry shows no number")
}

func TestLatinDeckNoticeNamesLewisShortAndItsPin(t *testing.T) {
	notices, err := attributionNotices([]RenderInput{{Language: "la", DictionaryEntryID: "n9,n10", DictionaryPin: testLSPin}})
	require.NoError(t, err)
	assert.Contains(t, notices, dictionary.LatinLSNotice(testLSPin))
	assert.NotContains(t, notices, dictionary.AncientGreekLSJNotice(testLSPin))

	greek, err := attributionNotices([]RenderInput{{Language: "grc", DictionaryEntryID: "n1", DictionaryPin: testLSJPin}})
	require.NoError(t, err)
	assert.Contains(t, greek, dictionary.AncientGreekLSJNotice(testLSJPin))
	assert.NotContains(t, greek, dictionary.LatinLSNotice(testLSJPin))
}

func TestGreekCardFreezesNoLewisShortEntry(t *testing.T) {
	stub := &lsStub{found: true, entries: dicoEntries()}
	entry := Entry{Language: "grc", CanonicalLemma: "λόγος"}
	assert.False(t, resolveLatinLS(t.Context(), stub, &entry, "Livy", ""))
	assert.Empty(t, entry.DictionaryEntry)
}

func TestLatinFullEntryKeepsURNOnlyCitationsOfTheWorksTextgroup(t *testing.T) {
	urnOnly := []enrichment.LatinLSEntry{{EntryID: "n1", Headword: "x", Orth: "x", Pin: testLSPin, Senses: []enrichment.LatinLSSense{{
		ID: "n1.0", Number: "I", Level: 1, Definitions: []string{"d"}, Glosses: []string{"d"},
		Citations: []enrichment.LatinLSCitation{{Author: "phi0474", Text: "own 1, 2"}, {Author: "phi0914", Text: "other 3, 4"}},
	}}}}

	own := renderLatinLSEntries(urnOnly, nil, cardpolicy.CitationsWorkAuthor, "Marcus Tullius Cicero", "phi0474")
	assert.Contains(t, own, "own 1, 2", "a URN-only citation is kept for its own author")
	assert.NotContains(t, own, "other 3, 4")

	other := renderLatinLSEntries(urnOnly, nil, cardpolicy.CitationsWorkAuthor, "Livy", "phi0914")
	assert.Contains(t, other, "other 3, 4")
	assert.NotContains(t, other, "own 1, 2", "a URN-only citation is dropped for another author")

	none := renderLatinLSEntries(urnOnly, nil, cardpolicy.CitationsWorkAuthor, "Livy", "")
	assert.NotContains(t, none, "own 1, 2")
	assert.NotContains(t, none, "other 3, 4", "no textgroup and no name match drops it, never guesses")
}

func TestLatinEntryWithSensesButNoDefinitionsStillCollapsesItsFullEntry(t *testing.T) {
	html := renderLatinLSEntries([]enrichment.LatinLSEntry{{EntryID: "n1", Orth: "x", Senses: []enrichment.LatinLSSense{{ID: "n1.0", Level: 1, Glosses: []string{"g"}}}}}, nil, 0, "", "")
	assert.NotContains(t, html, "dict-outline")
	assert.Contains(t, html, `<details class="dict-full"><summary>Full Lewis &amp; Short entry</summary>`)
}

// lsByLemmaStub answers the Lewis & Short capability by headword key.
type lsByLemmaStub struct {
	lsStub
	byLemma map[string][]enrichment.LatinLSEntry
}

func (s *lsByLemmaStub) LookupLatinLS(_ context.Context, lemma string) ([]enrichment.LatinLSEntry, bool) {
	entries, ok := s.byLemma[lemma]
	return entries, ok
}

func TestLatinCrossReferenceEntryShowsTheTargetMeaningOrOnlyItsHeadLine(t *testing.T) {
	target := enrichment.LatinLSEntry{EntryID: "n20", Headword: "dicto", Orth: "dictō", Pin: testLSPin,
		Senses: []enrichment.LatinLSSense{{ID: "n20.0", Number: "I", Level: 1, Definitions: []string{"to say often"}}}}
	redirect := func(refs ...enrichment.LatinLSCrossRef) enrichment.LatinLSEntry {
		return enrichment.LatinLSEntry{EntryID: "n21", Headword: "dictito", Orth: "dictĭto", Pin: testLSPin,
			Senses: []enrichment.LatinLSSense{{ID: "n21.e", Level: 0, CrossRefs: refs}}}
	}
	freeze := func(provider enrichment.LexicalProvider) Entry {
		entry := Entry{Language: "la", CanonicalLemma: "dictito"}
		require.True(t, resolveLatinLS(context.Background(), provider, &entry, "Cicero", ""))
		return entry
	}

	t.Run("a resolved reference shows the target's outline", func(t *testing.T) {
		provider := &lsByLemmaStub{byLemma: map[string][]enrichment.LatinLSEntry{
			"dictito": {redirect(enrichment.LatinLSCrossRef{Text: "dicto", EntryID: "n20", Headword: "dicto"})},
			"dicto":   {target},
		}}

		html := freeze(provider).DictionaryEntry

		assert.Contains(t, html, "dictĭto")
		assert.Contains(t, html, "to say often")
		assert.NotContains(t, html, "Full Lewis")
	})
	t.Run("an unresolved reference shows only the head line", func(t *testing.T) {
		provider := &lsByLemmaStub{byLemma: map[string][]enrichment.LatinLSEntry{
			"dictito": {redirect(enrichment.LatinLSCrossRef{Text: "the foll."})},
		}}

		html := freeze(provider).DictionaryEntry

		assert.Contains(t, html, "dictĭto")
		assert.NotContains(t, html, "dict-outline")
		assert.NotContains(t, html, "Full Lewis")
		assert.NotContains(t, html, "the foll.")
	})
}

func TestLatinEvidenceAssistedPassSendsEntryCarryingSensesAcrossStackedHomographs(t *testing.T) {
	projections := latinFreezeProjection("Livy")
	projections[0].Provider, projections[0].ProviderVersion, projections[0].TargetLanguage = "openai-compatible", "model/translation-v18-evidence-assisted-latin", "en"
	projections[0].EvidenceAssistedPass = true
	projections[0].Entry.Sentence = "vir magnam rem dico et cum amicis in forum venit."
	projections[0].Entry.TargetWord = "dico"
	deck, _, err := NewPresentation(&lsStub{found: true, entries: dicoEntries()}).Freeze(t.Context(), "owner-1", "Livy", projections)
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	request := work[0].Request
	require.True(t, request.RequireEvidenceAssistedPass)
	assert.Equal(t, []string{
		enrichment.LatinEvidenceID("n9", "n9.0"),
		enrichment.LatinEvidenceID("n9", "n9.1"),
		enrichment.LatinEvidenceID("n10", "n10.0"),
	}, evidenceIDs(request.CandidateSenses), "each cited sense names its entry; a glosses-only sense is evidence, the level-0 pseudo-sense is not")
	assert.Equal(t, "to dedicate", request.CandidateSenses[0].Gloss)
}

func TestLatinDeckWithoutThePassSendsNoCandidateSenses(t *testing.T) {
	deck, _, err := NewPresentation(&lsStub{found: true, entries: dicoEntries()}).Freeze(t.Context(), "owner-1", "Livy", latinFreezeProjection("Livy"))
	require.NoError(t, err)
	assert.Empty(t, deck.WorkProjection(), "the default Latin card has no provider work")
}

// TestLatinEvidenceAssistedPassEncodedRequestCarriesNoNativeLewisShortID encodes
// the request a frozen Latin deck would send and checks the bytes: only the
// per-request handles go out, never the native entry or sense IDs (#2766).
func TestLatinEvidenceAssistedPassEncodedRequestCarriesNoNativeLewisShortID(t *testing.T) {
	projections := latinFreezeProjection("Livy")
	projections[0].Provider, projections[0].ProviderVersion, projections[0].TargetLanguage = "openai-compatible", "model/translation-v18-evidence-assisted-latin", "en"
	projections[0].EvidenceAssistedPass = true
	projections[0].Entry.Sentence = "vir magnam rem dico et cum amicis in forum venit."
	projections[0].Entry.TargetWord = "dico"
	deck, _, err := NewPresentation(&lsStub{found: true, entries: dicoEntries()}).Freeze(t.Context(), "owner-1", "Livy", projections)
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	body, err := codec.EncodeRequest(work[0].Request)
	require.NoError(t, err)

	t.Run("no native entry or sense ID is in the request", func(t *testing.T) {
		var leaked []string
		for _, native := range []string{"n9", "n10"} {
			if strings.Contains(string(body), `"`+native) || strings.Contains(string(body), `\"`+native) {
				leaked = append(leaked, native)
			}
		}
		require.Empty(t, leaked, "native IDs must not go out")
		require.NotContains(t, string(body), enrichment.LatinEvidenceID("n9", "n9.0"))
	})
	t.Run("each sense carries its evidence and entry handles", func(t *testing.T) {
		require.Contains(t, string(body), `\"evidence_id\":\"e1\"`)
		require.Contains(t, string(body), `\"entry\":\"h1\"`)
		require.Contains(t, string(body), `\"entry\":\"h2\"`)
	})
}
