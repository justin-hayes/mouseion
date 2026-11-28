package study

import (
	"context"
	"errors"
	"testing"
	"time"

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
	// removed names the Books that have left My Books.
	removed map[string]bool
	// exhausted holds when each scan that found nothing left to ask began.
	exhausted []time.Time
	// onePage serves every occurrence on the first concordance page.
	onePage bool
}

func (s *fluencyFakeStore) ListVocabularyConcordance(ctx context.Context, owner, language string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error) {
	if !s.onePage {
		return s.fakeStore.ListVocabularyConcordance(ctx, owner, language, lookup)
	}
	if lookup.Page > 1 {
		return domain.ConcordanceResult{}, nil
	}
	return domain.ConcordanceResult{Occurrences: s.occurrences}, nil
}

func (s *fluencyFakeStore) RecordStudyFluencyExhausted(_ context.Context, _, _ string, scannedAt time.Time) error {
	s.exhausted = append(s.exhausted, scannedAt)
	return nil
}

func (s *fluencyFakeStore) StudyBookInCorpus(_ context.Context, _, book string) (bool, error) {
	return !s.removed[book], nil
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
	// passFor answers the evidence-assisted pass by target word; a word with no
	// answer cites the first sense. passes are the pass requests, and variant
	// the agreement variant requests, sent.
	passFor map[string]enrichment.EvidenceAssistedMeaning
	passes  []enrichment.Candidate
	variant []variantRequest
	// idiomFor answers an Idiom's idiom-card request by its sentence; a sentence
	// with no answer is confirmed with the learned sense. idiomCards are the
	// requests sent.
	idiomFor   map[string]enrichment.ContextualMeaning
	idiomCards []enrichment.IdiomCard
}

func (e *fluencyFakeEnricher) EnrichIdiomCard(_ context.Context, card enrichment.IdiomCard) (enrichment.ContextualMeaning, error) {
	e.idiomCards = append(e.idiomCards, card)
	if e.err != nil {
		return enrichment.ContextualMeaning{}, e.err
	}
	return e.agreeingIdiom(card.Sentence), nil
}

// variantRequest is what the source-evidence agreement variant was sent.
type variantRequest struct {
	candidate         enrichment.Candidate
	learned, occurred []string
}

func (e *fluencyFakeEnricher) EnrichEvidenceAssistedPass(_ context.Context, candidate enrichment.Candidate, _ string) (enrichment.EvidenceAssistedMeaning, error) {
	e.passes = append(e.passes, candidate)
	if e.err != nil {
		return enrichment.EvidenceAssistedMeaning{}, e.err
	}
	if answer, ok := e.passFor[candidate.TargetWord]; ok {
		return answer, nil
	}
	return enrichment.EvidenceAssistedMeaning{Gloss: "a word", SentenceTranslation: "A translation.", SentenceTranslationTargets: []string{"word"}, CitedSenseIDs: []string{enrichment.EntryEvidenceID("n100", "n100.1")}}, nil
}

func (e *fluencyFakeEnricher) EnrichSourceEvidenceFluencyAgreement(ctx context.Context, candidate enrichment.Candidate, learnedMeaning string, learned, occurred []string) (enrichment.FluencyAgreement, error) {
	e.variant = append(e.variant, variantRequest{candidate, learned, occurred})
	return e.EnrichFluencyAgreement(ctx, candidate, learnedMeaning)
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
		tokens: map[int64][]analyzer.Token{},
		progress: domain.StudyProgress{
			ID: "p1", StudyItem: domain.StudyItem{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}, MaterialID: "m1", LearnedMeaning: "a building",
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
	for _, priority := range store.priorities {
		assert.Equal(t, domain.ConcordancePriorityNone, priority, "no Book is read first")
	}
	assert.NotEmpty(t, store.priorities)

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

// A Book removed from My Books while its sentence is being assessed supplies no
// new context: Fluency never draws from outside the Corpus (ADR 0136 §2).
func TestFluencyWorkerStoresNoContextFromABookRemovedMeanwhile(t *testing.T) {
	worker, store, _ := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	store.removed = map[string]bool{store.occurrences[1].BookID: true}

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, store.recorded)
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

func (reasonEnricher) EnrichEvidenceAssistedPass(context.Context, enrichment.Candidate, string) (enrichment.EvidenceAssistedMeaning, error) {
	return enrichment.EvidenceAssistedMeaning{}, errors.New("not used")
}

func (reasonEnricher) EnrichIdiomCard(context.Context, enrichment.IdiomCard) (enrichment.ContextualMeaning, error) {
	return enrichment.ContextualMeaning{}, errors.New("not used")
}

func (reasonEnricher) EnrichSourceEvidenceFluencyAgreement(context.Context, enrichment.Candidate, string, []string, []string) (enrichment.FluencyAgreement, error) {
	return enrichment.FluencyAgreement{}, errors.New("not used")
}

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

func TestFluencyWorkerRecordsAWordWhoseCandidatesAreAllAssessedAsExhausted(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	item := store.occurrences[1]
	store.existing = []domain.FluencyContext{{ProgressID: "p1", Agreement: domain.FluencyDisagrees, Reason: domain.FluencyReasonUndefendable, AnalysisRunID: item.AnalysisRunID, SentenceOrdinal: item.SentenceOrdinal, TokenOrdinal: item.TokenOrdinal}}
	before := time.Now()

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, enricher.requests)
	require.Len(t, store.exhausted, 1)
	assert.False(t, store.exhausted[0].Before(before), "the scan is dated by its start")
}

func TestFluencyWorkerRecordsAWordWithNoOtherOccurrenceAsExhausted(t *testing.T) {
	worker, store, _ := newFluencyFixture()

	require.NoError(t, fluencyWork(worker))
	assert.Len(t, store.exhausted, 1)
}

func TestFluencyWorkerRecordsExhaustionWithoutARecordedAssessment(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	worker.MaySend = func(string) error { return enrichment.ErrProviderAssessmentNotRecorded }

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, enricher.requests)
	assert.Len(t, store.exhausted, 1, "that nothing is left to ask does not depend on the provider")
}

func TestFluencyWorkerDoesNotRecordExhaustionWhileCandidatesRemain(t *testing.T) {
	worker, store, _ := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	worker.MaySend = func(string) error { return enrichment.ErrProviderAssessmentNotRecorded }

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, store.exhausted, "an unassessed candidate is left")
}

func TestFluencyWorkerDoesNotRecordExhaustionWhenItStopsAtTheAskLimit(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.onePage = true
	for i := range maxFluencyAsks + 1 {
		surface := "haus" + string(rune('a'+i))
		store.addOccurrence(int64(i+2), surface, map[string]string{"Case": surface})
		enricher.disagree[surface] = true
	}

	require.NoError(t, fluencyWork(worker))
	require.Len(t, enricher.requests, maxFluencyAsks)
	assert.Empty(t, store.exhausted, "a candidate is left beyond the ask limit")
}

func TestFluencyWorkerDoesNotRecordExhaustionWhenAProviderFailureStopsIt(t *testing.T) {
	worker, store, enricher := newFluencyFixture()
	store.addOccurrence(2, "haeuser", map[string]string{"Number": "Plur"})
	enricher.err = errors.New("provider down")

	require.Error(t, fluencyWork(worker))
	assert.Empty(t, store.exhausted)
}

func TestFluencyWorkerDoesNotRecordExhaustionWhenThePageCapStopsTheRead(t *testing.T) {
	worker, store, _ := newFluencyFixture()
	for i := range maxFluencyPages + 1 {
		store.addOccurrence(int64(i+2), "haus"+string(rune('a'+i)), map[string]string{"Case": "x"})
	}
	// Every occurrence the capped read can reach is already assessed.
	for _, item := range store.occurrences[:maxFluencyPages] {
		store.existing = append(store.existing, domain.FluencyContext{ProgressID: "p1", Agreement: domain.FluencyDisagrees, Reason: domain.FluencyReasonUndefendable, AnalysisRunID: item.AnalysisRunID, SentenceOrdinal: item.SentenceOrdinal, TokenOrdinal: item.TokenOrdinal})
	}

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, store.exhausted, "occurrences remain beyond the page cap")
}
