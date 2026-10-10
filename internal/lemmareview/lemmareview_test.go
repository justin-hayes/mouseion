package lemmareview

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmarisk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore serves one Book's occurrences. Only the reads Assess uses are
// meaningful; the rest return zero values.
type fakeStore struct {
	book        domain.MyBook
	occurrences []domain.LemmaReviewOccurrence
	saved       []domain.LemmaReviewFlag
}

func (s *fakeStore) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return s.book, nil
}

func (s *fakeStore) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return domain.CurrentReading{}, nil
}

func (s *fakeStore) StartCurrentReading(context.Context, string, string, string) (domain.CurrentReading, error) {
	return domain.CurrentReading{}, nil
}

func (s *fakeStore) ListLemmaReviewOccurrences(context.Context, string, string, string) ([]domain.LemmaReviewOccurrence, error) {
	return s.occurrences, nil
}

func (s *fakeStore) SaveLemmaReviewFlags(_ context.Context, flags []domain.LemmaReviewFlag) error {
	s.saved = append(s.saved, flags...)
	return nil
}

func (s *fakeStore) ReadLemmaReviewProposal(context.Context, domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error) {
	return domain.LemmaReviewPreview{}, nil
}

func (s *fakeStore) PutLemmaDecisionProposal(context.Context, domain.LemmaReviewProposal, string) error {
	return nil
}

func (s *fakeStore) ListDeckPreparationsForSourceMaterial(context.Context, string, string) ([]domain.DeckPreparation, error) {
	return nil, nil
}

// fakeIndex answers every lemma lookup with exists, or with err when set. When
// block is set it waits for the caller's context instead of answering.
type fakeIndex struct {
	exists bool
	err    error
	block  bool
}

func (i fakeIndex) LemmaExists(ctx context.Context, _, _, _ string) (bool, error) {
	if i.block {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return i.exists, i.err
}

func (i fakeIndex) Alternatives(context.Context, string, string, string, string) ([]lemmarisk.Alternative, error) {
	return nil, nil
}

func germanBook(acquired bool) domain.MyBook {
	book := domain.MyBook{Book: domain.Book{LanguageTag: "de"}}
	if acquired {
		book.Acquired = &domain.SourceMaterialSummary{}
	}
	return book
}

func occurrence(offset int64) domain.LemmaReviewOccurrence {
	return domain.LemmaReviewOccurrence{
		AnalysisRunID: "run", SourceDocumentID: "doc", StartOffset: offset, EndOffset: offset + 3,
		Surface: "Weg", CanonicalLemma: "weg", UPOS: "NOUN", SentenceText: "Der Weg.",
	}
}

func TestAssessNonGermanBookIsNotAssessed(t *testing.T) {
	store := &fakeStore{book: domain.MyBook{Book: domain.Book{LanguageTag: "it"}, Acquired: &domain.SourceMaterialSummary{}}, occurrences: []domain.LemmaReviewOccurrence{occurrence(0)}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{exists: false}})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.False(t, assessment.Assessed, "risk is detected for German only")
	assert.Empty(t, store.saved)
}

func TestAssessWithoutIndexIsNotAssessed(t *testing.T) {
	store := &fakeStore{book: germanBook(true), occurrences: []domain.LemmaReviewOccurrence{occurrence(0)}}
	service := New(Config{Store: store})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.False(t, assessment.Assessed, "no index means not assessed, never a clean verdict")
	assert.Empty(t, store.saved)
}

func TestAssessIndexFailureIsNotAssessedNotAnError(t *testing.T) {
	store := &fakeStore{book: germanBook(true), occurrences: []domain.LemmaReviewOccurrence{occurrence(0)}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{err: errors.New("index offline")}})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err, "a failed optional index is not a Reading blocker")
	assert.False(t, assessment.Assessed)
}

func TestAssessIndexTimeoutIsNotAssessed(t *testing.T) {
	store := &fakeStore{book: germanBook(true), occurrences: []domain.LemmaReviewOccurrence{occurrence(0)}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{block: true}})

	started := time.Now()
	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.False(t, assessment.Assessed, "a detection that runs past its timeout is not a clean verdict")
	assert.Less(t, time.Since(started), 2*time.Second, "detection is bounded by its timeout")
}

func TestAssessReportsUnresolvedFlags(t *testing.T) {
	flagged := occurrence(0)
	flagged.ReviewFlagReason = "alternative exists"
	store := &fakeStore{book: germanBook(true), occurrences: []domain.LemmaReviewOccurrence{flagged}}
	service := New(Config{Store: store})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.True(t, assessment.Unresolved, "an unresolved flag blocks Reading")
	assert.False(t, assessment.Assessed, "the flag exists even though detection did not run")
}

func TestAssessResolvedFlagsAreNotUnresolved(t *testing.T) {
	flagged := occurrence(0)
	flagged.ReviewFlagReason = "alternative exists"
	flagged.ReviewFlagResolution = "kept"
	store := &fakeStore{book: germanBook(true), occurrences: []domain.LemmaReviewOccurrence{flagged}}
	service := New(Config{Store: store})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.False(t, assessment.Unresolved)
}

func TestAssessAssessedCleanBookSavesNoFlags(t *testing.T) {
	store := &fakeStore{book: germanBook(true), occurrences: []domain.LemmaReviewOccurrence{occurrence(0)}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{exists: true}})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.True(t, assessment.Assessed)
	assert.False(t, assessment.Unresolved)
	assert.Empty(t, store.saved)
}
