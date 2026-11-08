package cardexport

import (
	"context"
	"encoding/json"
	"errors"
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

func TestFreezeBoundsIdiomsByAnchorOccurrencesAndReportsOverflow(t *testing.T) {
	projections, lexicon := idiomBoundFixture(t)
	deck, diagnostics, err := NewPresentation(lexicon).Freeze(t.Context(), "owner-1", "Libro", projections, WithIdiomCards([]string{"it"}, "idiom-v1"), withIdiomCardBound(2))
	require.NoError(t, err)
	snapshot := deck.StorageProjection()

	t.Run("reports overflow in the manifest", func(t *testing.T) {
		require.Equal(t, 3, snapshot.IdiomOverflow)
	})
	t.Run("reports overflow in diagnostics", func(t *testing.T) {
		require.Equal(t, 3, diagnostics.IdiomOverflow)
	})
	t.Run("keeps the highest-priority idioms in deterministic order", func(t *testing.T) {
		headwords := make([]string, 0, 2)
		for _, item := range snapshot.Items {
			if item.ItemKind.IsIdiom() {
				headwords = append(headwords, item.Entry.Headword)
			}
		}
		require.Equal(t, []string{"epsilon zeta", "beta gamma"}, headwords)
	})
	t.Run("restores the overflow", func(t *testing.T) {
		restored, restoreErr := NewPresentation(nil).Restore(snapshot)
		require.NoError(t, restoreErr)
		require.Equal(t, 3, restored.Diagnostics().IdiomOverflow)
	})
}

func TestFreezeReportsNoOverflowWhenEveryIdiomFits(t *testing.T) {
	projections, lexicon := idiomBoundFixture(t)
	deck, diagnostics, err := NewPresentation(lexicon).Freeze(t.Context(), "owner-1", "Libro", projections, WithIdiomCards([]string{"it"}, "idiom-v1"), withIdiomCardBound(5))
	require.NoError(t, err)

	t.Run("manifest overflow is zero", func(t *testing.T) {
		require.Zero(t, deck.StorageProjection().IdiomOverflow)
	})
	t.Run("diagnostics overflow is zero", func(t *testing.T) {
		require.Zero(t, diagnostics.IdiomOverflow)
	})
}

func TestIdiomWorkReturnsEveryUnboundedIdiomRequest(t *testing.T) {
	projections, lexicon := idiomBoundFixture(t)
	deck, diagnostics, err := NewPresentation(lexicon).Freeze(t.Context(), "owner-1", "Libro", projections, WithIdiomCards([]string{"it"}, "idiom-v1"), WithoutIdiomCardBound())
	require.NoError(t, err)
	items := deck.IdiomWork("IT")

	t.Run("no idiom overflows", func(t *testing.T) {
		require.Zero(t, diagnostics.IdiomOverflow)
	})
	t.Run("every idiom is returned", func(t *testing.T) {
		require.Len(t, items, 5)
	})
	t.Run("each item carries the idiom request", func(t *testing.T) {
		for _, item := range items {
			require.True(t, item.Work.Request.RequireIdiomCard)
			require.Equal(t, item.Headword, item.Work.Request.IdiomHeadword)
			require.NotEmpty(t, item.Work.Request.IdiomMatchedForms)
			require.NotEmpty(t, item.Evidence.AnchorCanonicalLemma)
			require.Equal(t, "idiom-v1", item.Work.CacheKey.ProviderVersion)
		}
	})
	t.Run("another language has no idiom work", func(t *testing.T) {
		require.Empty(t, deck.IdiomWork("de"))
	})
}

func TestBoundIdiomProjectionsTreatsANegativeBoundAsZero(t *testing.T) {
	bounded := boundIdiomProjections([]CandidateProjection{{Entry: Entry{Headword: "in gamba"}}}, -1)

	require.Empty(t, bounded)
}

func TestIdiomProjectionOrderBreaksTiesDeterministically(t *testing.T) {
	base := CandidateProjection{
		Candidate: domain.SelectionCandidate{OccurrenceCount: 2, FirstEncounter: 10},
		Entry:     Entry{Headword: "in gamba", Language: "it"},
	}
	higherOccurrences := base
	higherOccurrences.Candidate.OccurrenceCount = 3
	laterSentence := base
	laterSentence.Candidate.FirstEncounter = 20
	earlierHeadword := base
	earlierHeadword.Entry.Headword = "a gamba"
	laterLanguage := base
	laterLanguage.Entry.Language = "zz"

	t.Run("more occurrences rank first", func(t *testing.T) {
		require.True(t, idiomProjectionLess(higherOccurrences, base))
	})
	t.Run("earlier sentence breaks the occurrence tie", func(t *testing.T) {
		require.True(t, idiomProjectionLess(base, laterSentence))
	})
	t.Run("headword breaks the sentence tie", func(t *testing.T) {
		require.True(t, idiomProjectionLess(earlierHeadword, base))
	})
	t.Run("language breaks the headword tie", func(t *testing.T) {
		require.True(t, idiomProjectionLess(base, laterLanguage))
	})
}

func TestFreezeReportsAnIdiomLookupFailure(t *testing.T) {
	_, _, err := NewPresentation(errorIdiomLexicon{}).Freeze(t.Context(), "owner-1", "Libro", []CandidateProjection{gambaProjection(t)}, WithIdiomCards([]string{"it"}, "idiom-v1"))
	require.ErrorContains(t, err, "idiom lookup failed")
}

type errorIdiomLexicon struct{}

func (errorIdiomLexicon) Name() string    { return "fixture" }
func (errorIdiomLexicon) Version() string { return "fixture-v1" }
func (errorIdiomLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (errorIdiomLexicon) IdiomEntries(context.Context, string, string) ([]idiom.Entry, error) {
	return nil, errors.New("idiom lookup failed")
}

func idiomBoundFixture(t *testing.T) ([]CandidateProjection, fakeIdiomLexicon) {
	t.Helper()
	specs := []struct {
		anchor, other string
		occurrences   int
		start         uint64
	}{
		{anchor: "alfa", other: "beta", occurrences: 5, start: 100},
		{anchor: "beta", other: "gamma", occurrences: 9, start: 200},
		{anchor: "gamma", other: "delta", occurrences: 9, start: 200},
		{anchor: "delta", other: "epsilon", occurrences: 1, start: 400},
		{anchor: "epsilon", other: "zeta", occurrences: 9, start: 100},
	}
	entries := make(map[string][]idiom.Entry, len(specs)*2)
	projections := make([]CandidateProjection, 0, len(specs))
	for _, spec := range specs {
		sentence, tokens := idiomBoundSentence(spec.start, spec.anchor, spec.other)
		entry := idiom.Entry{
			Language: "it", Headword: spec.anchor + " " + spec.other, UPOS: "ADJ",
			Words: []string{spec.anchor, spec.other},
			Senses: []enrichment.LexicalSense{{
				EvidenceID: "e-" + spec.anchor, Gloss: "gloss for " + spec.anchor,
			}},
			Source: "wiktionary", Version: "fixture-v1",
		}
		entries["it\x00"+spec.anchor] = []idiom.Entry{entry}
		entries["it\x00"+spec.other] = []idiom.Entry{entry}
		observed, err := json.Marshal([]string{spec.anchor})
		require.NoError(t, err)
		projections = append(projections, CandidateProjection{
			OwnerID: "owner-1", DeckName: "Libro",
			Candidate: domain.SelectionCandidate{
				OwnerID: "owner-1", CorpusID: "corpus-" + spec.anchor, Language: "it",
				CanonicalLemma: spec.anchor, UPOS: "NOUN", OccurrenceCount: spec.occurrences,
				FirstEncounter: int64(spec.start), ObservedForms: observed, //nolint:gosec // fixture offsets are small
			},
			Entry: Entry{
				OwnerID: "owner-1", Language: "it", CanonicalLemma: spec.anchor, UPOS: "NOUN",
				CorpusID: "corpus-" + spec.anchor, SentenceOrdinal: 1, Sentence: sentence, TargetWord: spec.anchor,
				SentenceTokens: tokens, SentenceStartOffset: spec.start, FirstEncounter: int64(spec.start), //nolint:gosec // fixture offsets are small
				SourceDocument: "Libro",
			},
			Sentences: map[int64]analyzer.Sentence{1: {Text: sentence, Tokens: tokens, Location: analyzer.SourceLocation{StartOffset: spec.start}}},
			Provider:  "fixture", ProviderVersion: "lemma-v1", TargetLanguage: "en", RequireContextualGloss: true,
		})
	}
	return projections, fakeIdiomLexicon{entries: entries}
}

func idiomBoundSentence(start uint64, anchor, other string) (string, []analyzer.Token) {
	words := []string{"È", "davvero", "un", anchor, other, "bello."}
	lemmas := []string{"è", "davvero", "un", anchor, other, "bello"}
	upos := []string{"AUX", "ADV", "DET", "NOUN", "NOUN", "ADJ"}
	tokens := make([]analyzer.Token, len(words))
	offset := start
	for i := range words {
		tokens[i] = analyzer.Token{
			Surface: words[i], CanonicalLemma: lemmas[i], UPOS: upos[i],
			Location: analyzer.SourceLocation{StartOffset: offset, EndOffset: offset + uint64(len(words[i]))},
		}
		offset += uint64(len(words[i])) + 1
	}
	return "È davvero un " + anchor + " " + other + " bello.", tokens
}

type fakeLexical struct{}

func (fakeLexical) Name() string    { return "fixture" }
func (fakeLexical) Version() string { return "fixture-v1" }
func (fakeLexical) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}

func TestFreezeSendsThePooledSensesOfEveryPartOfSpeechWithinTheSenseBound(t *testing.T) {
	senses := func(prefix string, count int) []enrichment.LexicalSense {
		out := make([]enrichment.LexicalSense, count)
		for i := range out {
			out[i] = enrichment.LexicalSense{Gloss: prefix + string(rune('a'+i))}
		}
		return out
	}
	adjective := inGambaEntry()
	adjective.Senses = senses("adj-", 6)
	adverb := inGambaEntry()
	adverb.UPOS = "ADV"
	adverb.Senses = senses("adv-", 4)
	lexicon := fakeIdiomLexicon{entries: map[string][]idiom.Entry{
		"it\x00in":    {adverb, adjective},
		"it\x00gamba": {adjective, adverb},
	}}

	deck, _, err := NewPresentation(lexicon).Freeze(t.Context(), "owner-1", "Libro", []CandidateProjection{gambaProjection(t)}, WithIdiomCards([]string{"it"}, "idiom-v1"))
	require.NoError(t, err)

	work := deck.WorkProjection()
	require.Len(t, work, 2, "one lemma item and one idiom item")
	require.True(t, work[1].Request.RequireIdiomCard)
	glosses := make([]string, 0, len(work[1].Request.CandidateSenses))
	for _, sense := range work[1].Request.CandidateSenses {
		glosses = append(glosses, sense.Gloss)
	}
	assert.Equal(t, []string{"adj-a", "adj-b", "adj-c", "adj-d", "adj-e", "adj-f", "adv-a", "adv-b"}, glosses,
		"both parts of speech reach the provider, capped at the candidate-sense bound")
}
