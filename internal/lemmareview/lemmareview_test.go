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
	proposal    domain.LemmaReviewProposal
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

func (s *fakeStore) ReadLemmaReviewProposal(_ context.Context, proposal domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error) {
	s.proposal = proposal
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

func germanBook() domain.MyBook {
	return domain.MyBook{Book: domain.Book{LanguageTag: "de"}, Acquired: &domain.SourceMaterialSummary{}}
}

func greekBook() domain.MyBook {
	return domain.MyBook{Book: domain.Book{LanguageTag: "grc"}, Acquired: &domain.SourceMaterialSummary{}}
}

func frenchBook() domain.MyBook {
	return domain.MyBook{Book: domain.Book{LanguageTag: "fr"}, Acquired: &domain.SourceMaterialSummary{}}
}

func occurrence() domain.LemmaReviewOccurrence {
	return domain.LemmaReviewOccurrence{
		AnalysisRunID: "run", SourceDocumentID: "doc", StartOffset: 0, EndOffset: 3,
		Surface: "Weg", CanonicalLemma: "weg", UPOS: "NOUN", SentenceText: "Der Weg.",
	}
}

func greekOccurrence() domain.LemmaReviewOccurrence {
	return domain.LemmaReviewOccurrence{
		AnalysisRunID: "run", SourceDocumentID: "doc", StartOffset: 0, EndOffset: 10,
		Surface: "λόγος", CanonicalLemma: "λόγοσ", UPOS: "NOUN", SentenceText: "λόγος δέω λέγει.",
	}
}

func TestAssessNonGermanBookIsNotAssessed(t *testing.T) {
	store := &fakeStore{book: domain.MyBook{Book: domain.Book{LanguageTag: "it"}, Acquired: &domain.SourceMaterialSummary{}}, occurrences: []domain.LemmaReviewOccurrence{occurrence()}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{exists: false}})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.False(t, assessment.Assessed, "risk is detected for German and French only")
	assert.Empty(t, store.saved)
}

func TestAssessFrenchBookIsAssessed(t *testing.T) {
	store := &fakeStore{book: frenchBook(), occurrences: []domain.LemmaReviewOccurrence{occurrence()}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{exists: true}})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.True(t, assessment.Assessed, "French has a dictionary-backed lemma-risk rule")
	assert.False(t, assessment.Unresolved)
	assert.Empty(t, store.saved)
}

func TestAssessWithoutIndexIsNotAssessed(t *testing.T) {
	store := &fakeStore{book: germanBook(), occurrences: []domain.LemmaReviewOccurrence{occurrence()}}
	service := New(Config{Store: store})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.False(t, assessment.Assessed, "no index means not assessed, never a clean verdict")
	assert.Empty(t, store.saved)
}

func TestAssessIndexFailureIsNotAssessedNotAnError(t *testing.T) {
	store := &fakeStore{book: germanBook(), occurrences: []domain.LemmaReviewOccurrence{occurrence()}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{err: errors.New("index offline")}})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err, "a failed optional index is not a Reading blocker")
	assert.False(t, assessment.Assessed)
}

func TestAssessIndexTimeoutIsNotAssessed(t *testing.T) {
	store := &fakeStore{book: germanBook(), occurrences: []domain.LemmaReviewOccurrence{occurrence()}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{block: true}})

	started := time.Now()
	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.False(t, assessment.Assessed, "a detection that runs past its timeout is not a clean verdict")
	assert.Less(t, time.Since(started), 2*time.Second, "detection is bounded by its timeout")
}

func TestAssessReportsUnresolvedFlags(t *testing.T) {
	flagged := occurrence()
	flagged.ReviewFlagReason = "alternative exists"
	store := &fakeStore{book: germanBook(), occurrences: []domain.LemmaReviewOccurrence{flagged}}
	service := New(Config{Store: store})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.True(t, assessment.Unresolved, "an unresolved flag blocks Reading")
	assert.False(t, assessment.Assessed, "the flag exists even though detection did not run")
}

func TestAssessResolvedFlagsAreNotUnresolved(t *testing.T) {
	flagged := occurrence()
	flagged.ReviewFlagReason = "alternative exists"
	flagged.ReviewFlagResolution = "kept"
	store := &fakeStore{book: germanBook(), occurrences: []domain.LemmaReviewOccurrence{flagged}}
	service := New(Config{Store: store})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.False(t, assessment.Unresolved)
}

func TestAssessAssessedCleanBookSavesNoFlags(t *testing.T) {
	store := &fakeStore{book: germanBook(), occurrences: []domain.LemmaReviewOccurrence{occurrence()}}
	service := New(Config{Store: store, RiskIndex: fakeIndex{exists: true}})

	assessment, err := service.Assess(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.True(t, assessment.Assessed)
	assert.False(t, assessment.Unresolved)
	assert.Empty(t, store.saved)
}

// A dedicated-profile language (Ancient Greek) is corrected with exactly the
// profile its analysis recorded, looked up by version, so a later active profile
// never rewrites a decision on an earlier analysis.
func TestCorrectedLemmaUsesTheRecordedProfileForADedicatedLanguage(t *testing.T) {
	recorded := greekOccurrence()
	recorded.NormalizationProfile = "ancient-greek"
	recorded.NormalizationVersion = "1"
	store := &fakeStore{book: greekBook(), occurrences: []domain.LemmaReviewOccurrence{recorded}}
	service := New(Config{Store: store})

	_, err := service.Preview(context.Background(), "owner", "book", Proposal{
		Language: "grc", Form: "λόγος", Action: "correct", Lemma: "λόγος", OccurrenceIDs: []string{recorded.ID()},
	})
	require.NoError(t, err)

	t.Run("the proposal records the analysis's profile", func(t *testing.T) {
		assert.Equal(t, "ancient-greek", store.proposal.NormalizationProfile)
		assert.Equal(t, "1", store.proposal.NormalizationVersion)
	})
	t.Run("the recorded profile normalized the correction", func(t *testing.T) {
		assert.Equal(t, "λόγοσ", store.proposal.Lemma, "ancient-greek/1 lowercases and maps final sigma to σ")
	})
}

// A dedicated-profile language never falls back to generic normalization: an
// analysis with no recorded profile, an unknown version, or a name the registry
// does not carry is reported unsupported rather than silently casefolded.
func TestCorrectedLemmaRefusesAnUnresolvableRecordedProfileForADedicatedLanguage(t *testing.T) {
	cases := map[string][2]string{
		"an unregistered version": {"ancient-greek", "2"},
		"a mismatched name":       {"not-the-profile", "1"},
		"no recorded profile":     {"", ""},
	}
	for name, recorded := range cases {
		t.Run(name, func(t *testing.T) {
			occ := greekOccurrence()
			occ.NormalizationProfile, occ.NormalizationVersion = recorded[0], recorded[1]
			store := &fakeStore{book: greekBook(), occurrences: []domain.LemmaReviewOccurrence{occ}}
			service := New(Config{Store: store})

			_, err := service.Preview(context.Background(), "owner", "book", Proposal{
				Language: "grc", Form: "λόγος", Action: "correct", Lemma: "λόγος", OccurrenceIDs: []string{occ.ID()},
			})

			require.ErrorIs(t, err, domain.ErrLemmaReviewUnsupportedLanguage)
		})
	}
}

// A modern language ignores any recorded profile and keeps using its active
// profile, so modern lemma review is unchanged. The recorded version 3 would
// keep the sharp s; the active version 6 maps it to "ss".
func TestCorrectedLemmaForAModernLanguageUsesTheActiveProfile(t *testing.T) {
	recorded := occurrence()
	recorded.NormalizationProfile = "german-standard-post-1996"
	recorded.NormalizationVersion = "3"
	store := &fakeStore{book: germanBook(), occurrences: []domain.LemmaReviewOccurrence{recorded}}
	service := New(Config{Store: store})

	_, err := service.Preview(context.Background(), "owner", "book", Proposal{
		Language: "de", Form: "Weg", Action: "correct", Lemma: "Haß", OccurrenceIDs: []string{recorded.ID()},
	})
	require.NoError(t, err)

	t.Run("the active profile's version is recorded", func(t *testing.T) {
		assert.Equal(t, "german-standard-post-1996", store.proposal.NormalizationProfile)
		assert.Equal(t, "6", store.proposal.NormalizationVersion)
	})
	t.Run("the active profile normalized the correction", func(t *testing.T) {
		assert.Equal(t, "hass", store.proposal.Lemma)
	})
}
