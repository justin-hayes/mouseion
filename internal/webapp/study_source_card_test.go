package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	brownsonPin   = "sha256:f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7"
	cardLSJLemma  = "λόγοσ"
	cardLSJPinOne = "lsj-card-pin-1"
)

// lsjCardLexicon is a synthetic LSJ lexicon for one lemma.
type lsjCardLexicon struct{ pin string }

func (lsjCardLexicon) Name() string    { return "lsj-card-stub" }
func (lsjCardLexicon) Version() string { return "1" }
func (lsjCardLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}

func (l lsjCardLexicon) AncientGreekLSJPin() string { return l.pin }

func (l lsjCardLexicon) LookupAncientGreekLSJ(_ context.Context, lemma string) (enrichment.AncientGreekLSJEntry, bool) {
	if lemma != cardLSJLemma {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	return enrichment.AncientGreekLSJEntry{
		EntryID: "lsj-logos", Headword: lemma, Pin: l.pin,
		Senses: []enrichment.AncientGreekLSJSense{
			{ID: "lsj-logos.0", Number: "A", Level: 1, Definitions: []string{"word", "speech"}},
			{ID: "lsj-logos.1", Number: "B", Level: 1, Definitions: []string{"account"}},
		},
	}, true
}

// lsCardLexicon is a synthetic Lewis & Short lexicon with two stacked entries.
type lsCardLexicon struct{ lsjCardLexicon }

func (lsCardLexicon) LookupLatinLS(_ context.Context, lemma string) ([]enrichment.LatinLSEntry, bool) {
	if lemma != "ager" {
		return nil, false
	}
	return []enrichment.LatinLSEntry{
		{EntryID: "n1", Headword: "ager", Orth: "ager", Pin: "ls-pin", Senses: []enrichment.LatinLSSense{{ID: "n1.s1", Number: "I", Level: 1, Definitions: []string{"a field"}}}},
		{EntryID: "n2", Headword: "ager", Orth: "ager", Pin: "ls-pin", Senses: []enrichment.LatinLSSense{{ID: "n2.s1", Number: "I", Level: 1, Definitions: []string{"a territory"}}}},
	}, true
}

func greekCard(pin string, passage domain.StudyPassage) domain.StudyCard {
	cited := domain.StudyCitations{SenseIDs: []string{"lsj-logos.0"}, EntryIDs: []string{"lsj-logos"}, DictionaryPin: pin}
	return domain.StudyCard{
		Progress: domain.StudyProgress{StudyIdentity: domain.StudyIdentity{Language: "grc", CanonicalLemma: cardLSJLemma, UPOS: "NOUN"}, LearnedMeaning: "word", LearnedCitations: cited},
		Material: domain.StudyMaterial{Sentence: "ἄρχεται ὁ λόγος.", TargetWord: "λόγος", SentenceTranslation: "The word begins.", Cited: cited, Passage: passage},
	}
}

func renderCard(t *testing.T, language string, view *studyCardView, notices []string) string {
	t.Helper()
	page := studyPageView{Language: language, Available: true, Card: view, SourceNotices: notices}
	var body bytes.Buffer
	require.NoError(t, StudyPage(domain.User{}, "csrf", page).Render(t.Context(), &body))
	return body.String()
}

func TestSourceCardMarksCitedSensesWhenThePinMatches(t *testing.T) {
	t.Cleanup(cardpolicy.OverrideStudyForTest("grc", true))
	passage := domain.StudyPassage{Text: "He halted his chariot.", Citation: "1.2.17", TranslationPin: brownsonPin}
	view := newStudyCardView("grc", greekCard(cardLSJPinOne, passage), true)
	attachSourceEvidence(t.Context(), lsjCardLexicon{pin: cardLSJPinOne}, "grc", greekCard(cardLSJPinOne, passage), view)
	body := renderCard(t, "grc", view, view.Source.Notices())

	assert.Contains(t, body, `id="study-gloss"`)
	assert.Contains(t, body, `class="study-dictionary"`)
	assert.Contains(t, body, "word")
	assert.Contains(t, body, "account", "the whole compact outline is shown")
	assert.Equal(t, 1, bytes.Count([]byte(body), []byte(`study-sense--cited`)), "only the cited sense is marked")
	assert.Contains(t, body, `class="study-passage"`)
	assert.Contains(t, body, "He halted his chariot.")
	assert.Contains(t, body, "Passage context: Brownson (1922), Xen. Anab. 1.2.17. Not a translation of this sentence")
	assert.NotContains(t, body, `class="study-dictionary" open`, "the sections start collapsed")
	assert.NotContains(t, body, "provider-proposed")
	assert.Contains(t, body, cardpolicy.GreekSourceNotice)
	assert.Contains(t, body, "PerseusDL/lexica@"+cardLSJPinOne)
	assert.Contains(t, body, "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7")
}

func TestSourceCardShowsTheOutlineWithoutMarksWhenThePinDiffers(t *testing.T) {
	card := greekCard("lsj-card-old", domain.StudyPassage{})
	view := newStudyCardView("grc", card, true)
	attachSourceEvidence(t.Context(), lsjCardLexicon{pin: cardLSJPinOne}, "grc", card, view)
	body := renderCard(t, "grc", view, view.Source.Notices())

	assert.Contains(t, body, "account", "the outline is still shown")
	assert.NotContains(t, body, "study-sense--cited")
	assert.Contains(t, body, "PerseusDL/lexica@"+cardLSJPinOne, "the notice names the revision shown")
}

func TestSourceCardHasNoPassageSectionWithoutAPassage(t *testing.T) {
	card := greekCard(cardLSJPinOne, domain.StudyPassage{})
	view := newStudyCardView("grc", card, true)
	attachSourceEvidence(t.Context(), lsjCardLexicon{pin: cardLSJPinOne}, "grc", card, view)
	body := renderCard(t, "grc", view, view.Source.Notices())

	assert.Contains(t, body, "study-dictionary")
	assert.NotContains(t, body, "study-passage")
	assert.NotContains(t, body, "Not a translation of this sentence")
}

func TestSourceCardOmitsAnEntryNoLongerInTheIndex(t *testing.T) {
	card := greekCard(cardLSJPinOne, domain.StudyPassage{})
	card.Progress.CanonicalLemma = "other"
	view := newStudyCardView("grc", card, true)
	attachSourceEvidence(t.Context(), lsjCardLexicon{pin: cardLSJPinOne}, "grc", card, view)
	body := renderCard(t, "grc", view, view.Source.Notices())

	assert.NotContains(t, body, "study-dictionary")
	assert.NotContains(t, body, "study-sense")
}

func TestSourceCardOmitsACitedEntryTheLemmaNoLongerResolvesTo(t *testing.T) {
	card := greekCard(cardLSJPinOne, domain.StudyPassage{})
	card.Progress.LearnedCitations.EntryIDs = []string{"lsj-gone"}
	view := newStudyCardView("grc", card, true)
	attachSourceEvidence(t.Context(), lsjCardLexicon{pin: cardLSJPinOne}, "grc", card, view)
	assert.Empty(t, view.Source.Entries)
}

func TestSourceCardSkipsAWithheldPassageWhoseTranslationIsNotListed(t *testing.T) {
	passage := domain.StudyPassage{Text: "He halted.", Citation: "1.2.17", TranslationPin: "sha256:" + "00000000000000000000000000000000000000000000000000000000000000bb"}
	card := greekCard(cardLSJPinOne, passage)
	view := newStudyCardView("grc", card, true)
	attachSourceEvidence(t.Context(), lsjCardLexicon{pin: cardLSJPinOne}, "grc", card, view)
	body := renderCard(t, "grc", view, view.Source.Notices())
	assert.NotContains(t, body, "study-passage", "a passage is never shown without its credit")
	assert.NotContains(t, body, "He halted.")
}

func TestSourceCardLabelsStackedLatinEntriesWithTheirNumber(t *testing.T) {
	cited := domain.StudyCitations{SenseIDs: []string{"n2.s1"}, EntryIDs: []string{"n2"}, DictionaryPin: "ls-pin"}
	card := domain.StudyCard{
		Progress: domain.StudyProgress{StudyIdentity: domain.StudyIdentity{Language: "la", CanonicalLemma: "ager", UPOS: "NOUN"}, LearnedMeaning: "territory", LearnedCitations: cited},
		Material: domain.StudyMaterial{Sentence: "ager est.", TargetWord: "ager", Cited: cited},
	}
	view := newStudyCardView("la", card, true)
	attachSourceEvidence(t.Context(), lsCardLexicon{}, "la", card, view)
	require.Len(t, view.Source.Entries, 1, "only the cited entry is shown")
	body := renderCard(t, "la", view, view.Source.Notices())
	assert.Contains(t, body, "<sup>2</sup>")
	assert.Contains(t, body, "a territory")
	assert.NotContains(t, body, "a field")
	assert.Contains(t, body, "study-sense--cited")
	assert.Contains(t, body, cardpolicy.LatinSourceNotice)
}

func TestModernCardHasNoSourceSections(t *testing.T) {
	card := domain.StudyCard{
		Progress: domain.StudyProgress{StudyIdentity: domain.StudyIdentity{Language: "de", CanonicalLemma: "tisch", UPOS: "NOUN"}, LearnedMeaning: "table"},
		Material: domain.StudyMaterial{Sentence: "Der Tisch.", TargetWord: "Tisch", SentenceTranslation: "The table."},
	}
	view := newStudyCardView("de", card, true)
	attachSourceEvidence(t.Context(), lsjCardLexicon{pin: cardLSJPinOne}, "de", card, view)
	body := renderCard(t, "de", view, view.Source.Notices())
	assert.Contains(t, body, "table")
	assert.NotContains(t, body, "study-dictionary")
	assert.NotContains(t, body, "study-passage")
	assert.NotContains(t, body, "Source notices")
}

func TestSourceCardWithoutADictionaryShowsNoOutline(t *testing.T) {
	card := greekCard(cardLSJPinOne, domain.StudyPassage{})
	view := newStudyCardView("grc", card, true)
	attachSourceEvidence(t.Context(), nil, "grc", card, view)
	assert.Empty(t, view.Source.Entries)
}

// sharedSenseLexicon stacks two Lewis & Short homographs that both hold a sense
// "s1", as the source's sense IDs are only unique within an entry.
type sharedSenseLexicon struct{ lsjCardLexicon }

func (sharedSenseLexicon) LookupLatinLS(context.Context, string) ([]enrichment.LatinLSEntry, bool) {
	return []enrichment.LatinLSEntry{
		{EntryID: "n1", Headword: "sum", Orth: "sum", Pin: "ls-pin", Senses: []enrichment.LatinLSSense{{ID: "s1", Number: "I", Level: 1, Definitions: []string{"to take"}}}},
		{EntryID: "n2", Headword: "sum", Orth: "sum", Pin: "ls-pin", Senses: []enrichment.LatinLSSense{{ID: "s1", Number: "I", Level: 1, Definitions: []string{"to be"}}}},
	}, true
}

func TestSourceCardMarksTheSenseOnlyInTheCitedHomograph(t *testing.T) {
	cited := domain.StudyCitations{SenseIDs: []string{"s1"}, EntryIDs: []string{"n2"}, DictionaryPin: "ls-pin"}
	card := domain.StudyCard{
		Progress: domain.StudyProgress{StudyIdentity: domain.StudyIdentity{Language: "la", CanonicalLemma: "sum", UPOS: "VERB"}, LearnedMeaning: "be", LearnedCitations: cited},
		Material: domain.StudyMaterial{Sentence: "sum.", TargetWord: "sum", Cited: cited},
	}
	view := newStudyCardView("la", card, true)
	attachSourceEvidence(t.Context(), sharedSenseLexicon{}, "la", card, view)

	require.Len(t, view.Source.Entries, 1, "only the cited homograph is shown")
	require.Len(t, view.Source.Entries[0].Outline, 1)
	assert.Equal(t, "to be", view.Source.Entries[0].Outline[0].Definitions)
	assert.True(t, view.Source.Entries[0].Outline[0].Cited)

	both := cited
	both.SenseIDs, both.EntryIDs = []string{"s1", "s1"}, []string{"n1", "n2"}
	card.Progress.LearnedCitations, card.Material.Cited = both, both
	attachSourceEvidence(t.Context(), sharedSenseLexicon{}, "la", card, view)
	require.Len(t, view.Source.Entries, 2)
	for _, entry := range view.Source.Entries {
		assert.True(t, entry.Outline[0].Cited, entry.Outline[0].Definitions)
	}
}

func TestSourceCardKeepsTheDictionaryPinWhenTheCitedEntryIsGone(t *testing.T) {
	card := greekCard(cardLSJPinOne, domain.StudyPassage{})
	card.Progress.LearnedCitations.EntryIDs = []string{"lsj-gone"}
	view := newStudyCardView("grc", card, true)
	attachSourceEvidence(t.Context(), lsjCardLexicon{pin: cardLSJPinOne}, "grc", card, view)
	assert.Empty(t, view.Source.Entries)
	assert.Contains(t, strings.Join(view.Source.Notices(), "\n"), "PerseusDL/lexica@"+cardLSJPinOne, "the dictionary in use is still credited with its pin")
}
