package study

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIdiomIndex serves dictionary idiom entries by each of their words, the
// way the dictionary's idiom lookup does.
type fakeIdiomIndex struct {
	entries []idiom.Entry
	err     error
	lookups int
}

func (f *fakeIdiomIndex) IdiomEntries(_ context.Context, language, word string) ([]idiom.Entry, error) {
	f.lookups++
	if f.err != nil {
		return nil, f.err
	}
	var found []idiom.Entry
	for _, entry := range f.entries {
		for _, candidate := range entry.Words {
			if candidate == strings.ToLower(word) {
				found = append(found, entry)
				break
			}
		}
	}
	return found, nil
}

// mitDemHaus is the entry that makes the seeded sentence "… mit dem Haus." an
// Idiom candidate; a sentence with another preposition is outside it.
func mitDemHaus() idiom.Entry {
	return idiom.Entry{Language: "de", Headword: "mit dem haus", UPOS: "PHRASE", Words: []string{"mit", "dem", "haus"}, Source: "test", Version: "1"}
}

func switchOn(index idiom.Index) *IdiomAvoidance {
	return &IdiomAvoidance{Index: index, Enabled: func(language string) bool { return language == "de" }}
}

// addSentence adds an occurrence of "haus" at the ordinal whose preposition is
// the given three-letter word, so the text keeps its offsets.
func addSentence(store *fakeStore, ordinal int64, preposition string) string {
	sentence := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN"})
	sentence.Text = strings.Replace(sentence.Text, "mit", preposition, 1)
	sentence.Tokens[3].Surface, sentence.Tokens[3].RawLemma, sentence.Tokens[3].CanonicalLemma = preposition, preposition, preposition
	item := occurrence(otherBook, sentence.Text, ordinal)
	item.Surface, item.TokenOrdinal = "haus", int64(len(sentence.Tokens)-1)
	store.occurrences = append(store.occurrences, item)
	if store.tokens == nil {
		store.tokens = map[int64][]analyzer.Token{}
	}
	store.tokens[ordinal] = sentence.Tokens
	return sentence.Text
}

func TestWorkPrefersASentenceOutsideAnIdiomCandidate(t *testing.T) {
	store := &fakeStore{}
	inside := addSentence(store, 1, "mit")
	outside := addSentence(store, 2, "auf")
	worker := newWorker(store, &fakeEnricher{meaning: resolved()})
	worker.Idioms = switchOn(&fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}})

	require.NoError(t, work(worker))
	require.NotNil(t, store.ready)
	assert.Equal(t, outside, store.ready.Sentence, "the sentence outside the candidate leads, though the one inside comes first")
	assert.NotEqual(t, inside, store.ready.Sentence)
}

func TestWorkKeepsTheBestSentenceWhenEveryOneIsInsideACandidate(t *testing.T) {
	store := &fakeStore{}
	first := addSentence(store, 1, "mit")
	addSentence(store, 2, "mit")
	worker := newWorker(store, &fakeEnricher{meaning: resolved()})
	worker.Idioms = switchOn(&fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}})

	require.NoError(t, work(worker))
	require.NotNil(t, store.ready)
	assert.Equal(t, first, store.ready.Sentence, "with no sentence outside a candidate the existing order decides")
}

func TestWorkOutsideAnIdiomCandidateBeatsTheCurrentReading(t *testing.T) {
	store := &fakeStore{reading: domain.CurrentReading{OwnerID: "owner", Language: "de", BookID: readingBook}}
	addSentence(store, 1, "mit")
	store.occurrences[0].BookID = readingBook
	outside := addSentence(store, 2, "auf")
	worker := newWorker(store, &fakeEnricher{meaning: resolved()})
	worker.Idioms = switchOn(&fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}})

	require.NoError(t, work(worker))
	require.NotNil(t, store.ready)
	assert.Equal(t, outside, store.ready.Sentence)
	assert.Equal(t, 2, store.pagesRead, "the search reads on past a Current reading sentence inside a candidate")
}

func TestWorkIsUnchangedWhileTheSwitchIsOff(t *testing.T) {
	for name, idioms := range map[string]*IdiomAvoidance{
		"no avoidance":      nil,
		"language disabled": {Index: &fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}}, Enabled: func(string) bool { return false }},
		"no lookup":         {Enabled: func(string) bool { return true }},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			inside := addSentence(store, 1, "mit")
			addSentence(store, 2, "auf")
			worker := newWorker(store, &fakeEnricher{meaning: resolved()})
			worker.Idioms = idioms

			require.NoError(t, work(worker))
			require.NotNil(t, store.ready)
			assert.Equal(t, inside, store.ready.Sentence)
		})
	}
}

func TestWorkFailsWhenTheIdiomLookupFails(t *testing.T) {
	store := &fakeStore{}
	addSentence(store, 1, "mit")
	worker := newWorker(store, &fakeEnricher{meaning: resolved()})
	worker.Idioms = switchOn(&fakeIdiomIndex{err: errors.New("dictionary unavailable")})

	require.ErrorContains(t, work(worker), "dictionary unavailable")
	assert.Nil(t, store.ready)
}

func TestFluencyRanksOccurrencesOutsideAnIdiomCandidateFirst(t *testing.T) {
	worker, store, _ := newFluencyFixture()
	inside := store.addOccurrence(2, "haeuser", map[string]string{"Case": "Nom", "Number": "Plur"})
	outside := store.addOccurrence(3, "hauses", map[string]string{"Case": "Gen", "Number": "Sing"})
	worker.Idioms = switchOn(&fakeIdiomIndex{entries: []idiom.Entry{{Language: "de", Headword: "nicht mit dem haeuser", UPOS: "PHRASE", Words: []string{"nicht", "mit", "dem", "haeuser"}, Source: "test", Version: "1"}}})

	ranked, err := worker.rankedCandidates(context.Background(), "owner", store.progress, map[occurrenceKey]bool{})
	require.NoError(t, err)
	require.Len(t, ranked, 2)
	assert.Equal(t, outside, ranked[0].sentence)
	assert.Equal(t, inside, ranked[1].sentence)
	assert.True(t, ranked[1].inIdiom)

	worker.Idioms = nil
	ranked, err = worker.rankedCandidates(context.Background(), "owner", store.progress, map[occurrenceKey]bool{})
	require.NoError(t, err)
	assert.Equal(t, inside, ranked[0].sentence, "with the switch off the ranking is as before")
}
