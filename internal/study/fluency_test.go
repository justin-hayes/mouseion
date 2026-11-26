package study

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fluencyFakeStore struct {
	fakeStore
	progress domain.StudyProgress
	existing []domain.FluencyContext
	recorded []domain.FluencyContextRecord
}

func (s *fluencyFakeStore) GetStudyProgress(context.Context, string, string) (domain.StudyProgress, error) {
	return s.progress, nil
}

func (s *fluencyFakeStore) ListStudyFluencyContexts(context.Context, string, string) ([]domain.FluencyContext, error) {
	return s.existing, nil
}

func (s *fluencyFakeStore) RecordStudyFluencyContext(_ context.Context, _ string, record domain.FluencyContextRecord) error {
	agreeing := 0
	for _, item := range s.recorded {
		if item.Agreement == domain.FluencyAgrees {
			agreeing++
		}
	}
	if record.Agreement == domain.FluencyAgrees && agreeing >= domain.FluencyContextsPerWord {
		return domain.ErrFluencyContextNotStored
	}
	s.recorded = append(s.recorded, record)
	return nil
}

// addOccurrence adds another occurrence of the word at sentence ordinal n: a
// sentence that passes the sentence-quality gate, whose target token is the
// surface with the given morphology. Each surface makes a distinct sentence. It
// returns the sentence text.
func (s *fluencyFakeStore) addOccurrence(ordinal int64, surface string, morphology map[string]string) string {
	sentence := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: surface, UPOS: "NOUN"})
	target := len(sentence.Tokens) - 1
	sentence.Tokens[target].Morphology = morphology
	item := occurrence(otherBook, sentence.Text, ordinal)
	item.Surface, item.TokenOrdinal = surface, int64(target)
	s.occurrences = append(s.occurrences, item)
	s.tokens[ordinal] = sentence.Tokens
	return sentence.Text
}

type fluencyFakeEnricher struct {
	disagree map[string]bool
	err      error
	requests []enrichment.Candidate
	meanings []string
}

func (e *fluencyFakeEnricher) EnrichFluencyAgreement(_ context.Context, candidate enrichment.Candidate, learnedMeaning string) (enrichment.FluencyAgreement, error) {
	e.requests = append(e.requests, candidate)
	e.meanings = append(e.meanings, learnedMeaning)
	if e.err != nil {
		return enrichment.FluencyAgreement{}, e.err
	}
	if e.disagree[candidate.TargetWord] {
		return enrichment.FluencyAgreement{Reason: enrichment.FluencyReasonIdiomatic, Gloss: "to be fine", SentenceTranslation: "It is fine.", SentenceTranslationTargets: []string{"fine"}}, nil
	}
	return enrichment.FluencyAgreement{Reason: enrichment.FluencyReasonSameMeaning, Gloss: "a building", SentenceTranslation: "A translation.", SentenceTranslationTargets: []string{"house"}}, nil
}

func nominative() map[string]string { return map[string]string{"Number": "Sing", "Case": "Nom"} }

// newFluencyFixture is a mature word introduced in a singular nominative
// sentence. The concordance lists that introducing occurrence too.
func newFluencyFixture() (*FluencyWorker, *fluencyFakeStore, *fluencyFakeEnricher) {
	store := &fluencyFakeStore{
		reading: domain.CurrentReading{OwnerID: "owner", Language: "de", BookID: readingBook},
		tokens:  map[int64][]analyzer.Token{},
		progress: domain.StudyProgress{
			ID: "p1", StudyIdentity: domain.StudyIdentity{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}, MaterialID: "m1", LearnedMeaning: "a building",
			Memory: domain.StudyMemory{State: domain.StudyStateReview, ScheduledDays: 30},
		},
	}
	introduced := store.addOccurrence(1, "haus", nominative())
	store.material = domain.StudyMaterial{
		ID: "m1", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", State: domain.StudyMaterialReady,
		Sentence: introduced, TargetWord: "haus", CorpusID: "corpus-" + otherBook, SentenceOrdinal: 1, TokenOrdinal: store.occurrences[0].TokenOrdinal,
	}
	enricher := &fluencyFakeEnricher{disagree: map[string]bool{}}
	return &FluencyWorker{Store: store, Enricher: enricher, MaySend: func(string) error { return nil }}, store, enricher
}

func fluencyWork(w *FluencyWorker) error {
	return w.Work(context.Background(), &river.Job[PrepareFluencyArgs]{Args: PrepareFluencyArgs{OwnerID: "owner", ProgressID: "p1"}})
}

func isSnooze(err error) bool {
	var snooze *rivertype.JobSnoozeError
	return errors.As(err, &snooze)
}

func TestFluencyWorkerSendsNothingWhileTheAssessmentIsNotRecorded(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur", "Case": "Nom"})
	worker.MaySend = func(string) error { return enrichment.ErrProviderAssessmentNotRecorded }

	require.NoError(t, fluencyWork(worker), "the job ends; the page enqueues it again later")
	assert.Empty(t, enricher.requests, "nothing is transmitted without a recorded assessment")
	assert.Empty(t, store.recorded)
}

func TestFluencyWorkerPrefersDifferentFormsAndStopsAtFiveAgreeingContexts(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	// Listed in the concordance's order. The same form as the introduction
	// comes first, but a different form is always asked about before it.
	store.addOccurrence(2, "hausa", nominative())
	store.addOccurrence(3, "haeuser", map[string]string{"Number": "Plur", "Case": "Nom"})
	store.addOccurrence(4, "hauses", map[string]string{"Number": "Sing", "Case": "Gen"})
	store.addOccurrence(5, "haeusern", map[string]string{"Number": "Plur", "Case": "Dat"})
	store.addOccurrence(6, "hause", map[string]string{"Number": "Sing", "Case": "Dat"})
	store.addOccurrence(7, "hausb", map[string]string{"Number": "Sing", "Case": "Acc"})
	store.addOccurrence(8, "hausc", nominative())

	require.NoError(t, fluencyWork(worker))

	require.Len(t, enricher.requests, domain.FluencyContextsPerWord, "asking stops once five agree")
	for _, meaning := range enricher.meanings {
		assert.Equal(t, "a building", meaning, "every request carries the learned meaning")
	}
	var asked []string
	for _, request := range enricher.requests {
		asked = append(asked, request.TargetWord)
		assert.NotEqual(t, store.material.Sentence, request.ExampleSentence, "the introducing sentence is never a Fluency context")
	}
	assert.NotContains(t, asked, "hausa", "a sentence of the introduced form is not among the first five")
	assert.NotContains(t, asked, "hausc")
	require.Len(t, store.recorded, domain.FluencyContextsPerWord)
	for _, record := range store.recorded {
		assert.Equal(t, domain.FluencyAgrees, record.Agreement)
		assert.Equal(t, domain.FluencyReasonSameMeaning, record.Reason)
		assert.NotEmpty(t, record.Form.Features, "the form the context was chosen for is kept")
	}
}

func TestFluencyWorkerKeepsContextsThatDisagreeWithTheLearnedMeaning(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	store.addOccurrence(3, "hauses", map[string]string{"Case": "Gen"})
	store.addOccurrence(4, "hause", map[string]string{"Case": "Dat"})
	enricher.disagree["hauses"] = true

	require.NoError(t, fluencyWork(worker))

	require.Len(t, store.recorded, 3)
	var disagreeing, agreeing int
	for _, record := range store.recorded {
		if record.Agreement == domain.FluencyDisagrees {
			disagreeing++
			assert.Equal(t, domain.FluencyReasonIdiomatic, record.Reason, "a disagreeing context keeps its reason")
			assert.Equal(t, "to be fine", record.Gloss, "a disagreeing context keeps its Gloss as evidence")
			assert.Equal(t, "It is fine.", record.SentenceTranslation)
			assert.Equal(t, []string{"fine"}, record.SentenceTranslationTargets)
			assert.Equal(t, "hauses", record.TargetWord)
		} else {
			agreeing++
		}
	}
	assert.Equal(t, 1, disagreeing)
	assert.Equal(t, 2, agreeing)
}

func TestFluencyWorkerDoesNotAskAboutAnOccurrenceTwice(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	item := store.occurrences[1]
	store.existing = []domain.FluencyContext{{ProgressID: "p1", Agreement: domain.FluencyDisagrees, Reason: domain.FluencyReasonUndefendable, AnalysisRunID: item.AnalysisRunID, SentenceOrdinal: item.SentenceOrdinal, TokenOrdinal: item.TokenOrdinal}}

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, enricher.requests)
}

func TestFluencyWorkerLeavesAWordThatLapsedAlone(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	store.progress.Memory = domain.StudyMemory{State: domain.StudyStateRelearning}

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, enricher.requests)
	assert.Empty(t, store.recorded)
}

func TestFluencyWorkerRetriesAProviderFailureWithoutRecordingIt(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	enricher.err = errors.New("provider unavailable")

	err := fluencyWork(worker)
	require.Error(t, err)
	assert.False(t, isSnooze(err))
	assert.Empty(t, store.recorded, "a provider failure is never recorded as a disagreement")

	enricher.err = enrichment.ErrExternalNotConfigured
	assert.True(t, isSnooze(fluencyWork(worker)), "a missing provider waits")
}

func TestFluencyFormOfDescribesFeaturesAndASplitParticle(t *testing.T) {
	tokens := []analyzer.Token{
		{Surface: "stehe", Morphology: map[string]string{"Tense": "Pres", "Person": "1"}},
		{Surface: "auf", Dependency: "compound:prt", Head: 0},
	}
	form := fluencyFormOf(tokens, 0)
	assert.Equal(t, "Person=1|Tense=Pres", form.Features, "features come in a fixed order")
	assert.True(t, form.Split, "a verb whose particle is its own token is split")
	assert.False(t, fluencyFormOf(tokens, 1).Split)
	assert.Equal(t, domain.FluencyForm{}, fluencyFormOf(tokens, 5))
}

func TestFluencyWorkerRefusesAnUnknownReason(t *testing.T) {
	worker, store, _ := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	worker.Enricher = reasonEnricher("maybe")

	require.Error(t, fluencyWork(worker))
	assert.Empty(t, store.recorded)
}

type reasonEnricher string

func (r reasonEnricher) EnrichFluencyAgreement(context.Context, enrichment.Candidate, string) (enrichment.FluencyAgreement, error) {
	return enrichment.FluencyAgreement{Reason: string(r), Gloss: "x", SentenceTranslation: "y"}, nil
}

func TestFluencyWorkerNeverAsksAboutARetiredOccurrence(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur", "Case": "Nom"})
	store.addOccurrence(3, "hause", map[string]string{"Number": "Sing", "Case": "Dat"})
	store.retired = []domain.StudyOccurrenceRef{store.occurrences[1].StudyRef()}

	require.NoError(t, fluencyWork(worker))
	for _, request := range enricher.requests {
		assert.NotEqual(t, "haeuser", request.TargetWord, "the reported sentence is not asked about")
	}
	require.Len(t, store.recorded, 1)
	assert.EqualValues(t, 3, store.recorded[0].SentenceOrdinal)
}
