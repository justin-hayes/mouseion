package cardexport

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testLSJPin = "56061ca127f4a2844980baffc5f2b6d1332897b3"

// lsjStub is a lexical provider that also answers the Ancient Greek LSJ
// capability, so freezeEntries resolves a matched entry without Wiktionary
// meaning evidence.
type lsjStub struct {
	entry enrichment.AncientGreekLSJEntry
	found bool
	calls int
}

func (s *lsjStub) Name() string    { return "lsj-stub" }
func (s *lsjStub) Version() string { return "lsj-stub-v1" }

func (s *lsjStub) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}

func (s *lsjStub) LookupAncientGreekLSJ(_ context.Context, lemma string) (enrichment.AncientGreekLSJEntry, bool) {
	s.calls++
	if !s.found {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	return s.entry, true
}

func lsjMatchedEntry() enrichment.AncientGreekLSJEntry {
	return enrichment.AncientGreekLSJEntry{
		EntryID:  "n61607",
		Headword: "λόγος",
		Pin:      testLSJPin,
		Senses: []enrichment.AncientGreekLSJSense{
			{
				ID: "n61607.0", Number: "A", Level: 1,
				Glosses:   []string{"word", "speech"},
				Citations: []string{"X. An. 1.1.1"},
				Senses: []enrichment.AncientGreekLSJSense{
					{ID: "n61607.1", Number: "2", Level: 3, Glosses: []string{"account"}},
				},
			},
			{ID: "n61607.2", Number: "B", Level: 1, Glosses: []string{"reason"}},
		},
	}
}

// TestAncientGreekLSJEntryRendersHierarchyInSourceOrder proves the full sense
// hierarchy is rendered in source order with LSJ numbering, the English glosses
// and the kept Xenophon citation, and never reranked or truncated.
func TestAncientGreekLSJEntryRendersHierarchyInSourceOrder(t *testing.T) {
	rendered := renderAncientGreekLSJEntry(lsjMatchedEntry())

	require.NotEmpty(t, rendered)
	assert.Less(t, strings.Index(rendered, "word"), strings.Index(rendered, "reason"), "senses keep source order")
	assert.Less(t, strings.Index(rendered, "account"), strings.Index(rendered, "reason"), "a nested sense stays inside its parent")
	assert.Contains(t, rendered, `<span class="lsj-number">A</span>`)
	assert.Contains(t, rendered, `<span class="lsj-number">B</span>`)
	assert.Contains(t, rendered, `<span class="lsj-gloss">word</span>`)
	assert.Contains(t, rendered, `<span class="lsj-gloss">speech</span>`)
	assert.Contains(t, rendered, `<span class="lsj-gloss">reason</span>`)
	assert.Contains(t, rendered, `lsj-level-3`)
	assert.Contains(t, rendered, `<span class="lsj-citation">X. An. 1.1.1</span>`)
}

// TestAncientGreekLSJEntryEscapesSourceText proves source text cannot inject
// markup into the rendered entry.
func TestAncientGreekLSJEntryEscapesSourceText(t *testing.T) {
	injected := renderAncientGreekLSJEntry(enrichment.AncientGreekLSJEntry{
		EntryID: "n1", Senses: []enrichment.AncientGreekLSJSense{{ID: "n1.0", Glosses: []string{"a <b>x</b> {y}"}}},
	})

	assert.NotContains(t, injected, "<b>x</b>")
	assert.Contains(t, injected, "a &lt;b&gt;x&lt;/b&gt; &#123;y&#125;")
}

// TestFreezeAncientGreekLSJFrozenIdentityAndPin proves a matched Ancient Greek
// card freezes the native LSJ entry ID, the LSJ pin and the rendered entry on
// the manifest entry, without any Wiktionary meaning evidence.
func TestFreezeAncientGreekLSJFrozenIdentityAndPin(t *testing.T) {
	stub := &lsjStub{found: true, entry: lsjMatchedEntry()}
	projections := []CandidateProjection{{
		OwnerID: "owner-1", DeckName: "Anabasis",
		Candidate: domain.SelectionCandidate{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", LexicalDiscriminator: "morpheus:1"},
		Entry: Entry{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", LexicalDiscriminator: "morpheus:1", Headword: "λόγος",
			Sentence: "ἐν ἀρχῇ ἦν ὁ λόγος.", TargetWord: "λόγος"},
		SkipMeaningEvidence: true,
	}}

	entries, err := freezeEntries(t.Context(), stub, projections)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	entry := entries[0]
	assert.Equal(t, "n61607", entry.DictionaryEntryID)
	assert.Equal(t, testLSJPin, entry.DictionaryPin)
	assert.Contains(t, entry.DictionaryEntry, "word")
	assert.Empty(t, entry.Gloss, "a Greek card carries no Wiktionary gloss")
	assert.Empty(t, entry.CandidateSenses, "a Greek card carries no Wiktionary meaning evidence")

	note, err := makeNote("owner-1", renderInputFromEntry(entry))
	require.NoError(t, err)
	assert.Contains(t, note.DictionaryEntry, `<span class="lsj-gloss">word</span>`)
	assert.NotContains(t, note.DictionaryEntry, "{{")
}

// TestFreezeAncientGreekLSJAmbiguousFreezesNoEntry proves an unresolved
// homograph or missing headword freezes no entry, never a guess.
func TestFreezeAncientGreekLSJAmbiguousFreezesNoEntry(t *testing.T) {
	projections := []CandidateProjection{{
		OwnerID: "owner-1", DeckName: "Anabasis",
		Candidate:           domain.SelectionCandidate{Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB", LexicalDiscriminator: "morpheus:1"},
		Entry:               Entry{Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB", LexicalDiscriminator: "morpheus:1", Sentence: "ἔδει γὰρ αὐτὸν δῆσαι τὸν ἵππον.", TargetWord: "ἔδει"},
		SkipMeaningEvidence: true,
	}}

	entries, err := freezeEntries(t.Context(), &lsjStub{found: false}, projections)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].DictionaryEntry)
	assert.Empty(t, entries[0].DictionaryEntryID)
	assert.Empty(t, entries[0].DictionaryPin)
}

// TestAncientGreekLSJNoticeNamesPinAndLicence proves a deck that froze a matched
// LSJ entry carries the LSJ/Perseus notice with its pin and CC BY-SA 4.0
// attribution, alongside the Ancient Greek source notices.
func TestAncientGreekLSJNoticeNamesPinAndLicence(t *testing.T) {
	entries := []RenderInput{{
		Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", Headword: "λόγος",
		Sentence: "ἐν ἀρχῇ ἦν ὁ λόγος.", TargetWord: "λόγος",
		DictionaryEntryID: "n61607", DictionaryPin: testLSJPin, DictionaryEntry: "<div></div>",
	}}

	attribution := attributionNotices(entries)
	assert.Contains(t, attribution, "LSJ")
	assert.Contains(t, attribution, "PerseusDL/lexica@"+testLSJPin)
	assert.Contains(t, attribution, "CC BY-SA 4.0")
	assert.Contains(t, attribution, "OGA")
	assert.Contains(t, deckDescription(entries), "PerseusDL/lexica@"+testLSJPin)
}

// TestAncientGreekDeckWithoutMatchedEntryCarriesNoLSJNotice proves a Greek deck
// with no matched LSJ entry does not claim one.
func TestAncientGreekDeckWithoutMatchedEntryCarriesNoLSJNotice(t *testing.T) {
	attribution := attributionNotices([]RenderInput{{Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB"}})

	assert.NotContains(t, attribution, "LSJ")
}

// TestFreezeAncientGreekLSJWithoutCapabilityFreezesNoEntry proves a lexical
// provider that does not implement the LSJ capability leaves the card without
// an entry rather than failing the freeze.
func TestFreezeAncientGreekLSJWithoutCapabilityFreezesNoEntry(t *testing.T) {
	projections := []CandidateProjection{{
		OwnerID: "owner-1", DeckName: "Anabasis",
		Candidate:           domain.SelectionCandidate{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN"},
		Entry:               Entry{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", Sentence: "ἐν ἀρχῇ ἦν ὁ λόγος.", TargetWord: "λόγος"},
		SkipMeaningEvidence: true,
	}}

	entries, err := freezeEntries(t.Context(), lexicalStub{}, projections)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].DictionaryEntryID)
	assert.Empty(t, entries[0].DictionaryEntry)
}

// TestResolveAncientGreekLSJIgnoresOtherLanguages proves the resolver only acts
// on Ancient Greek entries.
func TestResolveAncientGreekLSJIgnoresOtherLanguages(t *testing.T) {
	stub := &lsjStub{found: true, entry: lsjMatchedEntry()}
	entry := Entry{Language: "de", CanonicalLemma: "λόγος"}

	resolveAncientGreekLSJ(t.Context(), stub, &entry)

	assert.Empty(t, entry.DictionaryEntryID)
	assert.Zero(t, stub.calls)
}
