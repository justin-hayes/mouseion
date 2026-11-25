package study

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
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
}

func (s *fakeStore) GetStudyMaterial(context.Context, string, string) (domain.StudyMaterial, error) {
	return s.material, nil
}

func (s *fakeStore) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return s.reading, nil
}

func (s *fakeStore) ListVocabularyConcordance(_ context.Context, _, _ string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error) {
	if lookup.Page > 1 {
		return domain.ConcordanceResult{}, nil
	}
	return domain.ConcordanceResult{Occurrences: s.occurrences}, nil
}

func (s *fakeStore) ListCorpusSentences(_ context.Context, _, _ string, ordinals []int64) (map[int64]analyzer.Sentence, error) {
	result := map[int64]analyzer.Sentence{}
	for _, ordinal := range ordinals {
		result[ordinal] = analyzer.Sentence{}
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

type fakeEnricher struct {
	meaning  enrichment.ContextualMeaning
	err      error
	requests []enrichment.Candidate
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
}

func TestWorkNeverRecordsAProviderOmission(t *testing.T) {
	store := &fakeStore{occurrences: []domain.ConcordanceResultOccurrence{occurrence(otherBook, goodText, 1)}}

	err := work(newWorker(store, &fakeEnricher{err: errors.New("provider unavailable")}))
	require.Error(t, err)
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
