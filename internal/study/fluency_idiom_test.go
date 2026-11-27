package study

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const idiomLearnedSense = "test/mit dem haus/1"

// fluencyIdiomSentence is a candidate sentence of "mit dem haus" whose subject
// is name (four letters) and whose "haus" has the surface and morphology given.
func fluencyIdiomSentence(ordinal int64, name, surface string, morphology map[string]string) persistence.StudyIdiomSentence {
	seeded := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN"})
	seeded.Text = strings.Replace(seeded.Text, "Anna", name, 1)
	seeded.Tokens[0].Surface, seeded.Tokens[0].RawLemma, seeded.Tokens[0].CanonicalLemma = name, name, name
	last := len(seeded.Tokens) - 1
	start := seeded.Tokens[last].Location.StartOffset
	seeded.Text = seeded.Text[:start] + surface + "."
	seeded.Tokens[last].Surface = surface
	seeded.Tokens[last].Location = analyzer.SourceLocation{StartOffset: start, EndOffset: start + uint64(len(surface))}
	seeded.Tokens[last].Morphology = morphology
	return persistence.StudyIdiomSentence{
		BookID: otherBook, AnalysisRunID: "run-" + otherBook, CorpusID: "corpus-" + otherBook, SentenceOrdinal: ordinal, BookStartOffset: ordinal * 100,
		Sentence: analyzer.Sentence{Text: seeded.Text, Tokens: seeded.Tokens}, Effective: seeded.Tokens,
	}
}

// newIdiomFluencyFixture is a mature Idiom introduced in a singular nominative
// sentence, with a switch that is on.
func newIdiomFluencyFixture() (*FluencyWorker, *fluencyFakeStore, *fluencyFakeEnricher) {
	introducing := fluencyIdiomSentence(1, "Emma", "haus", nominative())
	store := &fluencyFakeStore{
		reading: domain.CurrentReading{OwnerID: "owner", Language: "de"},
		tokens:  map[int64][]analyzer.Token{1: introducing.Sentence.Tokens},
		progress: domain.StudyProgress{
			ID: "p1", StudyItem: domain.IdiomItem("de", "mit dem haus"), MaterialID: "m1", LearnedMeaning: "at home",
			LearnedCitations: domain.StudyCitations{SenseIDs: []string{idiomLearnedSense}, EntryIDs: []string{"mit dem haus"}, DictionaryPin: "test/1"},
			Memory:           domain.StudyMemory{State: domain.StudyStateReview, ScheduledDays: 30},
		},
	}
	store.material = domain.StudyMaterial{
		ID: "m1", StudyItem: domain.IdiomItem("de", "mit dem haus"), State: domain.StudyMaterialReady,
		Sentence: introducing.Sentence.Text, TargetWord: "mit dem haus", CorpusID: "corpus-" + otherBook, SentenceOrdinal: 1, TokenOrdinal: 3,
		MatchedTokenOrdinals: []int{3, 4, 5},
	}
	store.idiomSentences = []persistence.StudyIdiomSentence{introducing}
	enricher := &fluencyFakeEnricher{disagree: map[string]bool{}}
	worker := &FluencyWorker{Store: store, Enricher: enricher, MaySend: func(string) error { return enrichment.ErrProviderAssessmentNotRecorded }}
	worker.Idioms = switchOn(&fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}})
	return worker, store, enricher
}

func (e *fluencyFakeEnricher) agreeingIdiom(sentence string) enrichment.ContextualMeaning {
	if answer, ok := e.idiomFor[sentence]; ok {
		return answer
	}
	return enrichment.ContextualMeaning{Gloss: "at home", SentenceTranslation: "At home.", SentenceTranslationTargets: []string{"at home"}, CitedSenseIDs: []string{idiomLearnedSense}}
}

func TestFluencyWorkerPreparesAnIdiomsContextsPreferringDifferentForms(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	forms := []struct {
		name, surface string
		morphology    map[string]string
	}{
		{"Emma", "haus", nominative()}, // the introduced form: ranked last
		{"Lena", "haeuser", map[string]string{"Number": "Plur", "Case": "Nom"}},
		{"Mara", "hauses", map[string]string{"Number": "Sing", "Case": "Gen"}},
		{"Nina", "haeusern", map[string]string{"Number": "Plur", "Case": "Dat"}},
		{"Olga", "hause", map[string]string{"Number": "Sing", "Case": "Dat"}},
		{"Rosa", "haeuserns", map[string]string{"Number": "Plur", "Case": "Gen"}},
		{"Tina", "haeuschen", map[string]string{"Number": "Plur", "Case": "Nom"}}, // Lena's features
	}
	for i, form := range forms {
		sentence := fluencyIdiomSentence(int64(10+i), form.name, form.surface, form.morphology)
		if form.name == "Emma" {
			sentence.Sentence.Text = strings.Replace(sentence.Sentence.Text, "Emma", "Elke", 1)
			sentence.Effective[0].Surface = "Elke"
			sentence.Sentence.Tokens[0].Surface = "Elke"
		}
		store.idiomSentences = append(store.idiomSentences, sentence)
	}

	require.NoError(t, fluencyWork(worker))

	require.Len(t, store.recorded, domain.FluencyContextsPerWord)
	features := map[string]bool{}
	for _, record := range store.recorded {
		assert.Equal(t, domain.FluencyAgrees, record.Agreement)
		assert.Equal(t, "at home", record.Gloss)
		assert.Equal(t, domain.StudyCitations{SenseIDs: []string{idiomLearnedSense}, EntryIDs: []string{"mit dem haus"}, DictionaryPin: "test/1"}, record.Cited, "the context cites the sense its Gloss rests on")
		assert.Equal(t, int64(3), record.TokenOrdinal, "the occurrence is the first matched word")
		assert.Contains(t, strings.ToLower(record.TargetWord), "mit dem ", "the matched forms, joined by a space")
		features[record.Form.Features] = true
		assert.NotContains(t, record.Sentence, "Tina", "a context of an already chosen form comes after every different form")
		assert.NotContains(t, record.Sentence, "Elke")
	}
	assert.Len(t, features, domain.FluencyContextsPerWord, "no two contexts share their matched words' inflection")
	for _, card := range enricher.idiomCards {
		assert.Equal(t, "mit dem haus", card.Headword)
		assert.NotContains(t, card.Sentence, "Emma ist", "the introducing sentence is never asked about")
	}
	assert.Empty(t, enricher.requests, "the word agreement payload is never used for an Idiom")
}

func TestFluencyWorkerKeepsAnIdiomContextThatDisagreesAsEvidence(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	cases := map[string]struct {
		answer enrichment.ContextualMeaning
		reason domain.FluencyReason
		gloss  string
	}{
		"Lena": {enrichment.ContextualMeaning{SentenceTranslation: "She holds the house.", UnresolvedKind: enrichment.ContextualGlossUnresolvedLiteralUse, UnresolvedReason: "literal"}, domain.FluencyReasonLiteralUse, ""},
		"Mara": {enrichment.ContextualMeaning{Gloss: "a building", SentenceTranslation: "The building.", SentenceTranslationTargets: []string{"building"}, CitedSenseIDs: []string{"test/mit dem haus/2"}}, domain.FluencyReasonDifferentMeaning, "a building"},
		"Nina": {enrichment.ContextualMeaning{SentenceTranslation: "Unclear.", UnresolvedKind: enrichment.ContextualGlossUnresolvedAmbiguous, UnresolvedReason: "no sense"}, domain.FluencyReasonAmbiguous, ""},
	}
	enricher.idiomFor = map[string]enrichment.ContextualMeaning{}
	forms := map[string]map[string]string{
		"Lena": {"Number": "Plur", "Case": "Nom"}, "Mara": {"Number": "Sing", "Case": "Gen"}, "Nina": {"Number": "Plur", "Case": "Dat"},
	}
	ordinal := int64(10)
	byText := map[string]string{}
	for _, name := range []string{"Lena", "Mara", "Nina"} {
		sentence := fluencyIdiomSentence(ordinal, name, "haus"+strings.ToLower(name[:1]), forms[name])
		ordinal++
		store.idiomSentences = append(store.idiomSentences, sentence)
		enricher.idiomFor[strings.TrimSpace(sentence.Sentence.Text)] = cases[name].answer
		byText[strings.TrimSpace(sentence.Sentence.Text)] = name
	}

	require.NoError(t, fluencyWork(worker))

	require.Len(t, store.recorded, 3)
	for _, record := range store.recorded {
		want := cases[byText[record.Sentence]]
		assert.Equal(t, domain.FluencyDisagrees, record.Agreement)
		assert.Equal(t, want.reason, record.Reason)
		assert.Equal(t, want.gloss, record.Gloss, "the Gloss is kept as evidence")
		assert.NotEmpty(t, record.SentenceTranslation, "the translation is kept as evidence")
	}
}

func TestFluencyWorkerDoesNotAskAboutAnIdiomOccurrenceTwice(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	sentence := fluencyIdiomSentence(10, "Lena", "haeuser", map[string]string{"Number": "Plur"})
	store.idiomSentences = append(store.idiomSentences, sentence)
	store.existing = []domain.FluencyContext{{
		ProgressID: "p1", Agreement: domain.FluencyDisagrees, AnalysisRunID: sentence.AnalysisRunID,
		SentenceOrdinal: sentence.SentenceOrdinal, TokenOrdinal: 3,
	}}

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, enricher.idiomCards, "an assessed occurrence is not asked about again")
	assert.Empty(t, store.recorded)
}

func TestFluencyWorkerPreparesNoIdiomContextsWhileTheSwitchIsOff(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	store.idiomSentences = append(store.idiomSentences, fluencyIdiomSentence(10, "Lena", "haeuser", map[string]string{"Number": "Plur"}))

	worker.Idioms = nil
	require.NoError(t, fluencyWork(worker))
	worker.Idioms = &IdiomAvoidance{Index: &fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}}, Enabled: func(string) bool { return false }}
	require.NoError(t, fluencyWork(worker))

	assert.Empty(t, enricher.idiomCards, "nothing is sent")
	assert.Empty(t, store.recorded, "nothing is stored")
}

func TestFluencyWorkerSendsNothingWhenTheIdiomsLearnedMeaningCitesNoSense(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	store.progress.LearnedCitations = domain.StudyCitations{}
	store.idiomSentences = append(store.idiomSentences, fluencyIdiomSentence(10, "Lena", "haeuser", map[string]string{"Number": "Plur"}))

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, enricher.idiomCards, "there is no cited sense to compare a context against")
	assert.Empty(t, store.recorded)
}

func TestFluencyWorkerRequiresTheSameDictionaryPinForAnIdiomsAgreement(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	store.progress.LearnedCitations.DictionaryPin = "test/0" // the entry has since moved to test/1
	store.idiomSentences = append(store.idiomSentences, fluencyIdiomSentence(10, "Lena", "haeuser", map[string]string{"Number": "Plur"}))

	require.NoError(t, fluencyWork(worker))
	require.Len(t, store.recorded, 1)
	assert.Equal(t, domain.FluencyDisagrees, store.recorded[0].Agreement, "a sense of another pin is not the Learned meaning's sense")
	assert.Len(t, enricher.idiomCards, 1)
}

func TestFluencyWorkerWaitsForTheProviderWhenAnIdiomContextIsAsked(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	store.idiomSentences = append(store.idiomSentences, fluencyIdiomSentence(10, "Lena", "haeuser", map[string]string{"Number": "Plur"}))
	enricher.err = enrichment.ErrExternalNotConfigured

	assert.True(t, isSnooze(fluencyWork(worker)), "a missing provider waits and is never an answer")
	assert.Empty(t, store.recorded)
}

func TestIdiomFormTellsInflectionAndArrangementApart(t *testing.T) {
	tokens := func(gap bool, number string) []analyzer.Token {
		result := []analyzer.Token{
			{Surface: "mit", Morphology: map[string]string{}}, {Surface: "dem", Morphology: map[string]string{"Case": "Dat"}},
			{Surface: "x"}, {Surface: "haus", Morphology: map[string]string{"Number": number}},
		}
		if !gap {
			result = append(result[:2], result[3])
		}
		return result
	}
	matched := func(gap bool) []int {
		if gap {
			return []int{0, 1, 3}
		}
		return []int{0, 1, 2}
	}
	base := idiomFormOf(tokens(false, "Sing"), matched(false))
	assert.Equal(t, base.Key(), idiomFormOf(tokens(false, "Sing"), matched(false)).Key())
	assert.NotEqual(t, base.Key(), idiomFormOf(tokens(false, "Plur"), matched(false)).Key(), "a different inflection")
	assert.NotEqual(t, base.Key(), idiomFormOf(tokens(true, "Sing"), matched(true)).Key(), "a different arrangement: a word between the matched ones")
	assert.Equal(t, "mit dem haus", strings.ToLower(base.Surface))
}

func TestFluencyWorkerNeverAsksAboutTheIntroducingSentenceAndRanksItsFormLast(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	// The introducing sentence's text is repeated elsewhere in the library, in
	// the reading, with the same matched form; two sentences share a form that
	// differs from it.
	repeat := fluencyIdiomSentence(5, "Emma", "haus", nominative())
	repeat.InCurrentReading = true
	store.idiomSentences = append(store.idiomSentences, repeat,
		fluencyIdiomSentence(10, "Lena", "haeuser", map[string]string{"Number": "Plur"}),
		fluencyIdiomSentence(11, "Mara", "haeuserr", map[string]string{"Number": "Plur"}))
	store.idiomSentences = append(store.idiomSentences, fluencyIdiomSentence(12, "Nina", "haus", nominative()))
	store.idiomSentences[len(store.idiomSentences)-1].InCurrentReading = true

	require.NoError(t, fluencyWork(worker))

	var asked []string
	for _, card := range enricher.idiomCards {
		asked = append(asked, card.Sentence)
		assert.NotContains(t, card.Sentence, "Emma ist", "the introducing sentence is never a context")
	}
	require.Len(t, asked, 3)
	assert.Contains(t, asked[2], "Nina", "the context of the introducing form comes after the two of another form")
}

func TestFluencyWorkerNeverAsksAboutTheIntroducingSentenceWhereverItsTextIsRepeatedOrChanged(t *testing.T) {
	worker, store, enricher := newIdiomFluencyFixture()
	// The same sentence of the same corpus, read again with other text.
	changed := fluencyIdiomSentence(1, "Lena", "haeuser", map[string]string{"Number": "Plur"})
	store.idiomSentences = []persistence.StudyIdiomSentence{changed}

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, enricher.idiomCards, "the introducing sentence's place is never a context")
}
