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

const (
	readingBook = "book-reading"
	otherBook   = "book-library"
	goodText    = "Das alte Haus steht am Ende der langen Strasse."
	otherText   = "Gestern habe ich ein sehr schoenes Haus am See gesehen."
)

type fakeStore struct {
	material     domain.StudyMaterial
	reading      domain.CurrentReading
	occurrences  []domain.ConcordanceResultOccurrence
	ready        *domain.StudyMaterialReadyContent
	unpreparable string
	// omissionPin is the dictionary pin a no-dictionary-entry omission recorded.
	omissionPin string
	evidence    string
	pagesRead   int
	tokens      map[int64][]analyzer.Token
	// citations are the cited sections every sentence touches.
	citations []string
	retired   []domain.StudyOccurrenceRef
}

func (s *fakeStore) ListRetiredStudyOccurrencesOfWord(context.Context, string, domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error) {
	return s.retired, nil
}

func (s *fakeStore) ListRetiredStudyOccurrences(context.Context, string, string) ([]domain.StudyOccurrenceRef, error) {
	return s.retired, nil
}

func (s *fakeStore) GetStudyMaterial(context.Context, string, string) (domain.StudyMaterial, error) {
	return s.material, nil
}

func (s *fakeStore) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return s.reading, nil
}

// ListVocabularyConcordance serves the occurrences one per page, so a word
// with many occurrences is read over many pages.
func (s *fakeStore) ListVocabularyConcordance(_ context.Context, _, _ string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error) {
	if lookup.Page > len(s.occurrences) {
		return domain.ConcordanceResult{}, nil
	}
	s.pagesRead = lookup.Page
	return domain.ConcordanceResult{Occurrences: s.occurrences[lookup.Page-1 : lookup.Page], HasNext: lookup.Page < len(s.occurrences)}, nil
}

func (s *fakeStore) ListCorpusSentences(_ context.Context, _, _ string, ordinals []int64) (map[int64]analyzer.Sentence, error) {
	result := map[int64]analyzer.Sentence{}
	for _, ordinal := range ordinals {
		result[ordinal] = analyzer.Sentence{Tokens: s.tokens[ordinal], Citations: s.citations}
	}
	return result, nil
}

func (s *fakeStore) RecordStudyMaterialReady(_ context.Context, _, _ string, content domain.StudyMaterialReadyContent) error {
	s.ready = &content
	return nil
}

func (s *fakeStore) RecordStudyMaterialUnpreparable(_ context.Context, _, _, reason string) error {
	s.unpreparable = reason
	return nil
}

func (s *fakeStore) RecordStudyMaterialNoDictionaryEntry(_ context.Context, _, _, pin string) error {
	s.unpreparable, s.omissionPin = domain.StudyOmissionNoDictionaryEntry, pin
	return nil
}

func (s *fakeStore) ListStudyOccurrences(context.Context, string, domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error) {
	return nil, nil
}

func (s *fakeStore) SetStudyMaterialEvidence(_ context.Context, _, _, evidence string) error {
	s.evidence = evidence
	return nil
}

type fakeEnricher struct {
	meaning  enrichment.ContextualMeaning
	err      error
	requests []enrichment.Candidate
	// pass is the evidence-assisted pass answer, passErr its failure, and
	// passRequests and passages what the pass was sent.
	pass         enrichment.EvidenceAssistedMeaning
	passErr      error
	passRequests []enrichment.Candidate
	passages     []string
}

func (e *fakeEnricher) EnrichEvidenceAssistedPass(_ context.Context, candidate enrichment.Candidate, passage string) (enrichment.EvidenceAssistedMeaning, error) {
	e.passRequests = append(e.passRequests, candidate)
	e.passages = append(e.passages, passage)
	return e.pass, e.passErr
}

func (e *fakeEnricher) EnrichExternalContextual(_ context.Context, candidate enrichment.Candidate) (enrichment.ContextualMeaning, error) {
	e.requests = append(e.requests, candidate)
	return e.meaning, e.err
}

func occurrence(book, text string, ordinal int64) domain.ConcordanceResultOccurrence {
	return domain.ConcordanceResultOccurrence{
		Surface: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", SentenceText: text,
		BookID: book, AnalysisRunID: "run-" + book, CorpusID: "corpus-" + book, SentenceOrdinal: ordinal, BookStartOffset: ordinal * 100}
}

func newWorker(store *fakeStore, enricher *fakeEnricher) *MaterialWorker {
	store.material = domain.StudyMaterial{ID: "m1", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", State: domain.StudyMaterialPending}
	return &MaterialWorker{Store: store, Enricher: enricher}
}

func work(w *MaterialWorker) error {
	return w.Work(context.Background(), &river.Job[PrepareMaterialArgs]{Args: PrepareMaterialArgs{OwnerID: "owner", MaterialID: "m1"}})
}

func resolved() enrichment.ContextualMeaning {
	return enrichment.ContextualMeaning{Gloss: "a building", SentenceTranslation: "The old house.", SentenceTranslationTargets: []string{"house"}}
}

func TestWorkPrefersTheCurrentReadingsSentence(t *testing.T) {
	store := &fakeStore{
		reading:     domain.CurrentReading{OwnerID: "owner", Language: "de", BookID: readingBook},
		occurrences: []domain.ConcordanceResultOccurrence{occurrence(otherBook, goodText, 1), occurrence(readingBook, otherText, 2)},
	}
	enricher := &fakeEnricher{meaning: resolved()}
	require.NoError(t, work(newWorker(store, enricher)))

	require.NotNil(t, store.ready)
	assert.Equal(t, readingBook, store.ready.BookID, "the Current reading's sentence wins even when the library has another")
	assert.Equal(t, otherText, store.ready.Sentence)
	assert.Equal(t, "a building", store.ready.Gloss)
	require.Len(t, enricher.requests, 1)
	assert.Equal(t, "Haus", enricher.requests[0].TargetWord)
}

func TestWorkFallsBackToTheLibraryWhenTheReadingHasNoUsableSentence(t *testing.T) {
	store := &fakeStore{
		reading:     domain.CurrentReading{OwnerID: "owner", Language: "de", BookID: readingBook},
		occurrences: []domain.ConcordanceResultOccurrence{occurrence(readingBook, "Haus.", 1), occurrence(otherBook, goodText, 2)},
	}
	require.NoError(t, work(newWorker(store, &fakeEnricher{meaning: resolved()})))
	require.NotNil(t, store.ready)
	assert.Equal(t, otherBook, store.ready.BookID)
}

func TestWorkRecordsASentenceQualityOmissionWithoutCallingTheProvider(t *testing.T) {
	store := &fakeStore{occurrences: []domain.ConcordanceResultOccurrence{occurrence(otherBook, "Haus.", 1)}}
	enricher := &fakeEnricher{meaning: resolved()}
	require.NoError(t, work(newWorker(store, enricher)))
	assert.Equal(t, domain.StudyOmissionSentenceQuality, store.unpreparable)
	assert.Nil(t, store.ready)
	assert.Empty(t, enricher.requests)
}

func TestWorkRecordsAMeaningOmission(t *testing.T) {
	store := &fakeStore{occurrences: []domain.ConcordanceResultOccurrence{occurrence(otherBook, goodText, 1)}}
	enricher := &fakeEnricher{meaning: enrichment.ContextualMeaning{SentenceTranslation: "The house.", UnresolvedKind: enrichment.ContextualGlossUnresolvedLemmaMismatch, UnresolvedReason: "another word"}}
	require.NoError(t, work(newWorker(store, enricher)))
	assert.Equal(t, domain.StudyOmissionMeaning, store.unpreparable)
	assert.Equal(t, domain.StudyEvidenceFingerprint(nil), store.evidence, "the occurrences the attempt saw are recorded with the omission")
}

func TestWorkNeverRecordsAProviderOmission(t *testing.T) {
	store := &fakeStore{occurrences: []domain.ConcordanceResultOccurrence{occurrence(otherBook, goodText, 1)}}

	err := work(newWorker(store, &fakeEnricher{err: errors.New("provider unavailable")}))
	require.Error(t, err)
	assert.Empty(t, store.evidence, "a provider omission records nothing")
	assert.Empty(t, store.unpreparable)
	assert.Nil(t, store.ready)

	err = work(newWorker(store, &fakeEnricher{err: enrichment.ErrExternalNotConfigured}))
	var snooze *rivertype.JobSnoozeError
	require.ErrorAs(t, err, &snooze, "a missing provider waits instead of failing or omitting")
	assert.Empty(t, store.unpreparable)
	assert.Nil(t, store.ready)
}

func TestWorkIgnoresSettledMaterial(t *testing.T) {
	store := &fakeStore{}
	worker := newWorker(store, &fakeEnricher{})
	store.material.State = domain.StudyMaterialReady
	require.NoError(t, work(worker))
	assert.Nil(t, store.ready)
	assert.Empty(t, store.unpreparable)
}

func TestWorkSearchesPastTheFirstPagesForAQualifyingSentence(t *testing.T) {
	var occurrences []domain.ConcordanceResultOccurrence
	for ordinal := int64(1); ordinal <= 40; ordinal++ {
		occurrences = append(occurrences, occurrence(otherBook, "Haus.", ordinal))
	}
	occurrences = append(occurrences, occurrence(otherBook, goodText, 41))
	store := &fakeStore{occurrences: occurrences}
	require.NoError(t, work(newWorker(store, &fakeEnricher{meaning: resolved()})))
	require.NotNil(t, store.ready, "a qualifying sentence past the first pages is found")
	assert.Equal(t, goodText, store.ready.Sentence)
	assert.Empty(t, store.unpreparable)
}

func TestWorkStopsReadingOnceTheCurrentReadingHasASentence(t *testing.T) {
	occurrences := []domain.ConcordanceResultOccurrence{occurrence(readingBook, goodText, 1)}
	for ordinal := int64(2); ordinal <= 30; ordinal++ {
		occurrences = append(occurrences, occurrence(otherBook, goodText, ordinal))
	}
	store := &fakeStore{reading: domain.CurrentReading{OwnerID: "owner", Language: "de", BookID: readingBook}, occurrences: occurrences}
	require.NoError(t, work(newWorker(store, &fakeEnricher{meaning: resolved()})))
	assert.Equal(t, readingBook, store.ready.BookID)
	assert.Less(t, store.pagesRead, 30, "the library is not read once the Current reading has an accepted sentence")
}

func TestWorkRecordsASentenceQualityOmissionOnlyWhenEvidenceIsExhausted(t *testing.T) {
	var occurrences []domain.ConcordanceResultOccurrence
	for ordinal := int64(1); ordinal <= 12; ordinal++ {
		occurrences = append(occurrences, occurrence(otherBook, "Haus.", ordinal))
	}
	store := &fakeStore{occurrences: occurrences}
	require.NoError(t, work(newWorker(store, &fakeEnricher{meaning: resolved()})))
	assert.Equal(t, 12, store.pagesRead, "every page is read before the word is given up on")
	assert.Equal(t, domain.StudyOmissionSentenceQuality, store.unpreparable)
}

func TestWorkUsesOnlyOccurrencesOfTheFullIdentity(t *testing.T) {
	// Each seeded sentence passes the gate; only the target token's lexical
	// discriminator differs.
	seeded := func(discriminator string) testutil.SeededSentence {
		return testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", LexicalDiscriminator: discriminator})
	}
	build := func(ordinal int64, discriminator string, corrected bool) (domain.ConcordanceResultOccurrence, []analyzer.Token) {
		sentence := seeded(discriminator)
		item := occurrence(otherBook, sentence.Text, ordinal)
		item.Surface, item.TokenOrdinal, item.Corrected = "haus", int64(len(sentence.Tokens)-1), corrected
		return item, sentence.Tokens
	}
	other, otherTokens := build(1, "morpheus:1", false)
	corrected, correctedTokens := build(2, "morpheus:2", true)
	wanted, wantedTokens := build(3, "morpheus:2", false)
	store := &fakeStore{
		occurrences: []domain.ConcordanceResultOccurrence{other, corrected, wanted},
		tokens:      map[int64][]analyzer.Token{1: otherTokens, 2: correctedTokens, 3: wantedTokens},
	}
	worker := newWorker(store, &fakeEnricher{meaning: resolved()})
	store.material.LexicalDiscriminator = "morpheus:2"
	require.NoError(t, work(worker))
	require.NotNil(t, store.ready)
	assert.EqualValues(t, 3, store.ready.SentenceOrdinal, "another discriminator's sentence, and a corrected occurrence (which carries none), are not this word's")
}

func TestWorkNeverChoosesARetiredOccurrence(t *testing.T) {
	retired := occurrence(readingBook, goodText, 1)
	store := &fakeStore{
		reading:     domain.CurrentReading{OwnerID: "owner", Language: "de", BookID: readingBook},
		occurrences: []domain.ConcordanceResultOccurrence{retired, occurrence(otherBook, otherText, 2)},
		retired:     []domain.StudyOccurrenceRef{retired.StudyRef()},
	}
	enricher := &fakeEnricher{meaning: resolved()}
	require.NoError(t, work(newWorker(store, enricher)))
	require.NotNil(t, store.ready)
	assert.Equal(t, otherBook, store.ready.BookID, "the reported sentence is passed over for another one, though it is the Current reading's")
	assert.Equal(t, otherText, store.ready.Sentence)
	require.Len(t, enricher.requests, 1)
	assert.Equal(t, otherText, enricher.requests[0].ExampleSentence, "the provider never sees the retired sentence")
}

func TestWorkRecordsAnOmissionWhenEveryRemainingOccurrenceIsRetired(t *testing.T) {
	only := occurrence(otherBook, goodText, 1)
	store := &fakeStore{occurrences: []domain.ConcordanceResultOccurrence{only}, retired: []domain.StudyOccurrenceRef{only.StudyRef()}}
	enricher := &fakeEnricher{meaning: resolved()}
	require.NoError(t, work(newWorker(store, enricher)))
	assert.Nil(t, store.ready)
	assert.Equal(t, domain.StudyOmissionSentenceQuality, store.unpreparable, "the word is unpreparable, and withdrawn as having no suitable context")
	assert.Empty(t, enricher.requests, "no provider call for a retired sentence")
}

func TestWorkSkipsEveryTokenOfARetiredSentence(t *testing.T) {
	// The reported sentence holds the word twice: the other token is retired too.
	reported := occurrence(otherBook, goodText, 3)
	reported.TokenOrdinal = 2
	sameSentence := occurrence(otherBook, goodText, 3)
	sameSentence.TokenOrdinal = 6
	other := occurrence(otherBook, otherText, 4)
	store := &fakeStore{occurrences: []domain.ConcordanceResultOccurrence{sameSentence, other}, retired: []domain.StudyOccurrenceRef{reported.StudyRef()}}
	require.NoError(t, work(newWorker(store, &fakeEnricher{meaning: resolved()})))
	require.NotNil(t, store.ready)
	assert.EqualValues(t, 4, store.ready.SentenceOrdinal, "the sentence is retired for the word, not only the reported token")
}

func TestWorkKeepsASentenceRetiredInTheRunAnAnalysisUpdateCarriedItTo(t *testing.T) {
	// The store resolves the report to the sentence in the current run as well.
	reportedOld := domain.StudyOccurrenceRef{BookID: otherBook, AnalysisRunID: "run-1", CorpusID: "corpus-1", SentenceOrdinal: 7, TokenOrdinal: 2}
	moved := occurrence(otherBook, goodText, 3)
	other := occurrence(otherBook, otherText, 4)
	store := &fakeStore{occurrences: []domain.ConcordanceResultOccurrence{moved, other}, retired: []domain.StudyOccurrenceRef{reportedOld, moved.StudyRef()}}
	require.NoError(t, work(newWorker(store, &fakeEnricher{meaning: resolved()})))
	require.NotNil(t, store.ready)
	assert.EqualValues(t, 4, store.ready.SentenceOrdinal)
}
