package cardexport

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIdiomLexicon is a lexical provider that also exposes dictionary idiom
// entries, so detection runs without a SQLite index.
type fakeIdiomLexicon struct {
	entries map[string][]idiom.Entry
}

func (f fakeIdiomLexicon) Name() string    { return "fixture" }
func (f fakeIdiomLexicon) Version() string { return "fixture-v1" }

func (f fakeIdiomLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}

func (f fakeIdiomLexicon) IdiomEntries(_ context.Context, language, word string) ([]idiom.Entry, error) {
	return f.entries[language+"\x00"+word], nil
}

func inGambaEntry() idiom.Entry {
	return idiom.Entry{
		Language: "it", Headword: "in gamba", UPOS: "ADJ",
		Words:  []string{"in", "gamba"},
		Senses: []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "capable, on the ball"}},
		Source: "wiktionary", Version: "fixture-v1",
	}
}

func inGambaSentence(start uint64) (string, []analyzer.Token) {
	words := []string{"È", "davvero", "un", "ragazzo", "in", "gamba."}
	lemmas := []string{"è", "davvero", "un", "ragazzo", "in", "gamba"}
	upos := []string{"AUX", "ADV", "DET", "NOUN", "ADP", "NOUN"}
	tokens := make([]analyzer.Token, len(words))
	offset := start
	for i := range words {
		tokens[i] = analyzer.Token{
			Surface: words[i], CanonicalLemma: lemmas[i], UPOS: upos[i],
			Location: analyzer.SourceLocation{StartOffset: offset, EndOffset: offset + uint64(len(words[i]))},
		}
		offset += uint64(len(words[i])) + 1
	}
	return "È davvero un ragazzo in gamba.", tokens
}

func gambaProjection(t *testing.T) CandidateProjection {
	t.Helper()
	sentence, tokens := inGambaSentence(100)
	observed, err := json.Marshal([]string{"gamba"})
	require.NoError(t, err)
	return CandidateProjection{
		OwnerID: "owner-1", DeckName: "Libro",
		Candidate: domain.SelectionCandidate{
			OwnerID: "owner-1", CorpusID: "corpus-1", Language: "it",
			CanonicalLemma: "gamba", UPOS: "NOUN", OccurrenceCount: 3, FirstEncounter: 100,
			ObservedForms: observed,
		},
		Entry: Entry{
			OwnerID: "owner-1", Language: "it", CanonicalLemma: "gamba", UPOS: "NOUN",
			CorpusID: "corpus-1", SentenceOrdinal: 1, Sentence: sentence, TargetWord: "gamba",
			SentenceTokens: tokens, SentenceStartOffset: 100, FirstEncounter: 100,
			SourceDocument: "Libro",
		},
		Sentences:              map[int64]analyzer.Sentence{1: {Text: sentence, Tokens: tokens, Location: analyzer.SourceLocation{StartOffset: 100}}},
		Provider:               "fixture",
		ProviderVersion:        "lemma-v1",
		TargetLanguage:         "en",
		RequireContextualGloss: true,
	}
}

func idiomLexicon() fakeIdiomLexicon {
	entry := inGambaEntry()
	return fakeIdiomLexicon{entries: map[string][]idiom.Entry{
		"it\x00in":    {entry},
		"it\x00gamba": {entry},
	}}
}

func TestFreezeDetectsAnIdiomAndBindsItsOwnContract(t *testing.T) {
	deck, _, err := NewPresentation(idiomLexicon()).Freeze(t.Context(), "owner-1", "Libro", []CandidateProjection{gambaProjection(t)}, WithIdiomCards([]string{"it"}, "idiom-v1"))
	require.NoError(t, err)

	snapshot := deck.StorageProjection()
	require.Len(t, snapshot.Items, 2)
	assert.Equal(t, enrichment.ItemKindLemma, snapshot.Items[0].ItemKind.Normalized())
	assert.Equal(t, enrichment.ItemKindIdiom, snapshot.Items[1].ItemKind.Normalized())
	item := snapshot.Items[1]
	assert.Equal(t, "in gamba", item.Entry.Headword)
	assert.Equal(t, "IDIOM", item.Entry.UPOS)
	assert.Equal(t, []int{4, 5}, item.Entry.MatchedTokenOrdinals)
	assert.Equal(t, "gamba", item.Entry.AnchorCanonicalLemma)
	assert.Equal(t, "wiktionary", item.Entry.Source)
	assert.Equal(t, "fixture-v1", item.Entry.SourceVersion)
	assert.NotNil(t, item.CacheKey)
	assert.Equal(t, enrichment.ItemKindIdiom, item.CacheKey.ItemKind.Normalized())
	assert.Equal(t, "idiom-v1", item.CacheKey.ProviderVersion)

	work := deck.WorkProjection()
	require.Len(t, work, 2)
	idiomWork := work[1]
	assert.True(t, idiomWork.Request.RequireIdiomCard)
	assert.False(t, idiomWork.Request.RequireContextualGloss)
	assert.Equal(t, "in gamba", idiomWork.Request.IdiomHeadword)
	assert.Equal(t, []string{"in", "gamba"}, idiomWork.Request.IdiomMatchedForms)
	assert.Equal(t, "lemma-v1", work[0].CacheKey.ProviderVersion)
}

func TestFreezeLeavesTheDeckUnchangedWhenTheSwitchIsOffOrTheLanguageIsNotEnabled(t *testing.T) {
	for name, options := range map[string][]FreezeOption{
		"switch off":       nil,
		"another language": {WithIdiomCards([]string{"de"}, "idiom-v1")},
	} {
		t.Run(name, func(t *testing.T) {
			deck, _, err := NewPresentation(idiomLexicon()).Freeze(t.Context(), "owner-1", "Libro", []CandidateProjection{gambaProjection(t)}, options...)
			require.NoError(t, err)
			snapshot := deck.StorageProjection()
			require.Len(t, snapshot.Items, 1)
			assert.Equal(t, enrichment.ItemKindLemma, snapshot.Items[0].ItemKind.Normalized())
			assert.Len(t, deck.WorkProjection(), 1)
		})
	}
}

func TestFinalizeRendersTheIdiomCardAndRerenderReproducesIt(t *testing.T) {
	deck, _, err := NewPresentation(idiomLexicon()).Freeze(t.Context(), "owner-1", "Libro", []CandidateProjection{gambaProjection(t)}, WithIdiomCards([]string{"it"}, "idiom-v1"))
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 2)
	results := make([]StoredResult, len(work))
	for i, item := range work {
		key := item.CacheKey
		results[i] = StoredResult{CacheKey: key, Record: enrichment.CacheEntry{
			CacheKey: key, Translation: "translation", FallbackGloss: "capable",
			SentenceTranslation: "He is really a capable young man.", SentenceTranslationTargets: []string{"capable young man"},
		}}
	}
	artifact, _, err := NewPresentation(nil).Finalize(t.Context(), deck, results, RunFacts{})
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 2)
	note := artifact.Generated[1].Note
	assert.Equal(t, IdiomDedupKey("it", "in gamba", "owner-1"), note.Key)
	assert.Contains(t, note.Tags, "mouseion::idiom")
	assert.Equal(t, "È davvero un ragazzo <b>in</b> <b>gamba</b>.", note.Text)
	assert.Equal(t, "in gamba", note.Lemma)
	assert.Equal(t, "idiom", note.POS)
	assert.Equal(t, "capable", note.Gloss)
	assert.Equal(t, "He is really a <b>capable young man</b>.", note.EnglishSentence)
	assert.Empty(t, note.Article)
	assert.Empty(t, note.Gender)
	assert.Empty(t, note.Plural)
	assert.Empty(t, note.IPA)
	assert.Equal(t, "in gamba\nidiom\ncapable\nHe is really a <b>capable young man</b>.", note.BackExtra)

	restored, err := NewPresentation(nil).Restore(deck.StorageProjection())
	require.NoError(t, err)
	rerendered, _, err := NewPresentation(nil).Finalize(t.Context(), restored, results, RunFacts{})
	require.NoError(t, err)
	require.Len(t, rerendered.Generated, 2)
	assert.Equal(t, note, rerendered.Generated[1].Note)
}

func TestIdiomMatchingIsAbsentWithoutAnIdiomIndex(t *testing.T) {
	deck, _, err := NewPresentation(fakeLexical{}).Freeze(t.Context(), "owner-1", "Libro", []CandidateProjection{gambaProjection(t)}, WithIdiomCards([]string{"it"}, "idiom-v1"))
	require.NoError(t, err)
	assert.Len(t, deck.StorageProjection().Items, 1)
}

type fakeLexical struct{}

func (fakeLexical) Name() string    { return "fixture" }
func (fakeLexical) Version() string { return "fixture-v1" }
func (fakeLexical) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
