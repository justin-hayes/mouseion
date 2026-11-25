package cardexport

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var latinTranslationPin = "sha256:" + strings.Repeat("a", 64)

// useLatinTranslationRegister installs a synthetic perseus-engN register entry
// for the Latin pin, so a card test never depends on a real translation.
func useLatinTranslationRegister(t *testing.T) {
	t.Helper()
	previous := embeddedPassageAttribution
	embeddedPassageAttribution = func(pin string) (classical.PassageAttribution, bool, error) {
		if pin != latinTranslationPin {
			return classical.PassageAttribution{}, false, nil
		}
		return classical.PassageAttribution{
			Label:  "Synthetic (1900), Fixture Hist.",
			Notice: "Passage context: Synthetic translation, Perseus perseus-eng9 (PerseusDL/canonical-latinLit@0123456789abcdef0123456789abcdef01234567, SHA-256 aaaaaaaa). CC BY-SA 4.0.",
		}, true, nil
	}
	t.Cleanup(func() { embeddedPassageAttribution = previous })
}

func latinProjection(citations ...string) CandidateProjection {
	return CandidateProjection{
		ReferencePin: latinTranslationPin,
		ReferenceTranslations: map[string]string{
			"1.pr.1": "The first section.",
			"1.pr.2": "The second section.",
		},
		Sentences: map[int64]analyzer.Sentence{0: {Citations: citations}},
	}
}

func TestLatinPassageContextShowsEverySectionASentenceTouches(t *testing.T) {
	entry := Entry{Language: "la", SentenceOrdinal: 0}
	applyPassageContext(&entry, latinProjection("1.pr.1", "1.pr.2"))
	assert.Equal(t, "The first section. The second section.", entry.PassageContext)
	assert.Equal(t, "1.pr.1–2", entry.PassageCitation)
	assert.Equal(t, latinTranslationPin, entry.PassagePin)
}

func TestLatinPassageContextWithAMissingSegmentShowsNoEnglish(t *testing.T) {
	useLatinTranslationRegister(t)
	entry := Entry{Language: "la", SentenceOrdinal: 0}
	applyPassageContext(&entry, latinProjection("1.pr.1", "1.pr.9"))
	assert.Empty(t, entry.PassageContext+entry.PassageCitation+entry.PassagePin)

	entry.CanonicalLemma, entry.UPOS, entry.Headword = "domus", "NOUN", "domus"
	entry.Sentence, entry.TargetWord = "Domus stat.", "Domus"
	note, err := makeNote("owner-1", renderInputFromEntry(entry))
	require.NoError(t, err)
	assert.Empty(t, note.PassageLabel+note.PassageText, "no placeholder is rendered")
	assert.NotContains(t, note.BackExtra, "Passage context")
}

func TestLatinCardRendersLabelledUnhighlightedPassageContext(t *testing.T) {
	useLatinTranslationRegister(t)
	note, err := makeNote("owner-1", renderInputFromEntry(Entry{ //nolint:gosec // G101 false positive: a passage pin, not a credential.
		Language: "la", CanonicalLemma: "domus", UPOS: "NOUN", Headword: "domus",
		Sentence: "Domus stat.", TargetWord: "Domus",
		PassageContext: "The first section. The second section.", PassageCitation: "1.pr.1–2", PassagePin: latinTranslationPin,
	}))
	require.NoError(t, err)
	assert.Equal(t, "Passage context: Synthetic (1900), Fixture Hist. 1.pr.1–2. Not a translation of this sentence", note.PassageLabel)
	assert.Equal(t, "The first section. The second section.", note.PassageText)
}

func TestLatinDeckNoticesNameOLAAndThePerseusTextAndTranslation(t *testing.T) {
	useLatinTranslationRegister(t)
	entries := []RenderInput{{Language: "la", CanonicalLemma: "domus", UPOS: "NOUN", PassagePin: latinTranslationPin}}
	notices, err := attributionNotices(entries)
	require.NoError(t, err)
	assert.Contains(t, notices, cardpolicy.LatinSourceNotice)
	assert.Contains(t, notices, "Opera Latina Adnotata (OLA) v0.2.0")
	assert.Contains(t, notices, "canonical-latinLit")
	assert.Contains(t, notices, "0123456789abcdef0123456789abcdef01234567", "the translation's pin")
	assert.Contains(t, notices, "CC BY-SA 4.0")
	assert.NotContains(t, notices, AnalysisModelsNotice, "Latin analysis is imported, not NLP-analysed")
	assert.NotContains(t, notices, cardpolicy.GreekSourceNotice)
}

// A Latin card never looks a lemma up in the Greek LSJ.
type lsjProbe struct {
	enrichment.LexicalProvider
	called bool
}

func (p *lsjProbe) LookupAncientGreekLSJ(context.Context, string) (enrichment.AncientGreekLSJEntry, bool) {
	p.called = true
	return enrichment.AncientGreekLSJEntry{EntryID: "n1", Headword: "x", Pin: "p"}, true
}

func TestLatinCardFreezesNoGreekLSJEntry(t *testing.T) {
	probe := &lsjProbe{}
	_, found := resolveAncientGreekLSJ(t.Context(), probe, &Entry{Language: "la", CanonicalLemma: "domus"})
	assert.False(t, found)
	assert.False(t, probe.called)

	_, found = resolveAncientGreekLSJ(t.Context(), probe, &Entry{Language: "grc", CanonicalLemma: "λόγος"})
	assert.True(t, found, "the probe is reachable for a Greek card")
}

func TestPassageContextShowsSegmentsThatContainATouchedCitation(t *testing.T) {
	translations := map[string]string{"1.2": "Whole chapter.", "1.3.1": "Section."}
	text, citation, _ := resolvePassage([]string{"1.2.3"}, translations, latinTranslationPin)
	assert.Equal(t, "Whole chapter.", text)
	assert.Equal(t, "1.2", citation)
	text, _, _ = resolvePassage([]string{"1.2.3", "1.4.1"}, translations, latinTranslationPin)
	assert.Empty(t, text, "a touched section with no segment withholds the passage")
}
