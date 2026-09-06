// Package analysisinsights calculates learner-specific metrics for analyzed books.
package analysisinsights

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
)

var ErrStatisticsUnavailable = errors.New("analysis insights: corpus statistics unavailable")

var thresholdTargets = [...]int{95, 97, 99}
var projectionSizes = [...]int64{10, 25, 50}

const (
	topUnknownLimit   = 5
	concentrationSize = int64(10)
)

type Store interface {
	GetAnalysisCorpusVocabulary(context.Context, string, string) (domain.AnalysisCorpusVocabulary, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
	ListActiveLearningCampaignVocabulary(context.Context, string, string) ([]domain.CampaignVocabulary, error)
	ListLegacyGeneratedVocabulary(context.Context, string, string) ([]domain.GeneratedVocabulary, error)
}

// JourneyStore, PrimaryGoalStore, and BookEvidenceStore are intentionally
// separate from Store so existing single-book insight stores remain useful.
type JourneyStore interface {
	GetReadingJourney(context.Context, string) (domain.ReadingJourney, error)
}

type PrimaryGoalStore interface {
	GetPrimaryGoal(context.Context, string) (domain.PrimaryGoal, error)
}

type BookEvidenceStore interface {
	ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
}

// LanguageCorpusStore is separate from Store so single-book insight stores do
// not need to load a collection read model.
type LanguageCorpusStore interface {
	ListLanguageCorpusEvidence(context.Context, string, string) ([]domain.LanguageCorpusBookEvidence, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Coverage(ctx context.Context, owner, corpusID string) (domain.AnalysisCoverage, error) {
	input, err := s.store.GetAnalysisCorpusVocabulary(ctx, owner, corpusID)
	if err != nil {
		return domain.AnalysisCoverage{}, fmt.Errorf("load analysis corpus vocabulary: %w", err)
	}
	if input.Statistics == nil {
		return domain.AnalysisCoverage{}, ErrStatisticsUnavailable
	}

	return s.coverage(ctx, owner, input, false)
}

func (s *Service) coverage(ctx context.Context, owner string, input domain.AnalysisCorpusVocabulary, activeIsKnown bool) (domain.AnalysisCoverage, error) {
	vocabularies := make(map[string]vocabulary)
	for _, lemma := range input.Lemmas {
		if !lexical.IsLemma(lemma.CanonicalLemma) {
			continue
		}
		if _, ok := vocabularies[lemma.Language]; ok {
			continue
		}
		loaded, err := s.loadVocabulary(ctx, owner, lemma.Language)
		if err != nil {
			return domain.AnalysisCoverage{}, err
		}
		vocabularies[lemma.Language] = loaded
	}
	result, _, err := coverageWithVocabulary(input, vocabularies, activeIsKnown)
	return result, err
}

type vocabulary struct {
	known     map[string]bool
	active    map[string]bool
	generated []domain.GeneratedVocabulary
}

func (s *Service) loadVocabulary(ctx context.Context, owner, language string) (vocabulary, error) {
	words, err := s.store.ListKnownVocabulary(ctx, owner, language)
	if err != nil {
		return vocabulary{}, fmt.Errorf("list known vocabulary for %s: %w", language, err)
	}
	known := make(map[string]bool, len(words))
	for _, word := range words {
		known[identity(word.CanonicalLemma, word.UPOS)] = true
	}

	generated, err := s.store.ListLegacyGeneratedVocabulary(ctx, owner, language)
	if err != nil {
		return vocabulary{}, fmt.Errorf("list generated vocabulary for %s: %w", language, err)
	}
	activeWords, err := s.store.ListActiveLearningCampaignVocabulary(ctx, owner, language)
	if err != nil {
		return vocabulary{}, fmt.Errorf("list active campaign vocabulary for %s: %w", language, err)
	}
	active := make(map[string]bool, len(activeWords))
	for _, word := range activeWords {
		active[identity(word.CanonicalLemma, word.UPOS)] = true
	}
	return vocabulary{known: known, active: active, generated: generated}, nil
}

func (v vocabulary) generatedFor(sourceMaterialID, key string) bool {
	for _, word := range v.generated {
		if identity(word.CanonicalLemma, word.UPOS) != key {
			continue
		}
		if word.FirstSourceMaterialID == nil || *word.FirstSourceMaterialID != sourceMaterialID {
			return true
		}
	}
	return false
}

// coverageWithVocabulary is the shared arithmetic path for single-book and
// language-level views. eligible contains the complete eligible identity list;
// the public per-book result retains its existing top-five presentation cap.
func coverageWithVocabulary(input domain.AnalysisCorpusVocabulary, vocabularies map[string]vocabulary, activeIsKnown bool) (domain.AnalysisCoverage, []domain.LemmaOccurrence, error) {
	result := domain.AnalysisCoverage{
		SourceMaterialID:     input.SourceMaterialID,
		AnalysisRunID:        input.AnalysisRunID,
		AnalyzableTokenCount: input.Statistics.AnalyzableTokenCount,
		DistinctLemmaCount:   input.Statistics.DistinctLemmaCount,
		TextProfile:          input.Statistics.TextProfile,
	}
	eligible := make([]domain.LemmaOccurrence, 0, len(input.Lemmas))
	for _, lemma := range input.Lemmas {
		if !lexical.IsLemma(lemma.CanonicalLemma) {
			result.AnalyzableTokenCount = max(result.AnalyzableTokenCount-lemma.OccurrenceCount, 0)
			result.DistinctLemmaCount = max(result.DistinctLemmaCount-1, 0)
			continue
		}
		vocab := vocabularies[lemma.Language]

		key := identity(lemma.CanonicalLemma, lemma.UPOS)
		activeMatch := vocab.active[key] || vocab.active[identity(lemma.CanonicalLemma, "")]
		if vocab.known[key] || vocab.known[identity(lemma.CanonicalLemma, "")] || (activeIsKnown && activeMatch) {
			result.KnownTokenCount += lemma.OccurrenceCount
			result.KnownLemmaCount++
			continue
		}
		result.UnknownLemmaCount++
		if activeMatch {
			result.ActiveCampaignTokenCount += lemma.OccurrenceCount
			result.ActiveCampaignLemmaCount++
		}
		if !vocab.generatedFor(input.SourceMaterialID, key) && !activeMatch {
			eligible = append(eligible, lemma)
		}
	}
	// Persisted statistics are authoritative. Clamp learner-derived values so
	// malformed legacy rows can never produce impossible coverage output.
	result.KnownTokenCount = min(result.KnownTokenCount, result.AnalyzableTokenCount)
	result.UnknownTokenCount = max(result.AnalyzableTokenCount-result.KnownTokenCount, 0)

	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].OccurrenceCount != eligible[j].OccurrenceCount {
			return eligible[i].OccurrenceCount > eligible[j].OccurrenceCount
		}
		return occurrenceKey(eligible[i]) < occurrenceKey(eligible[j])
	})
	var eligibleTokens int64
	for _, lemma := range eligible {
		eligibleTokens += lemma.OccurrenceCount
	}
	topUnknownCount := min(topUnknownLimit, len(eligible))
	result.TopUnknownLemmas = append([]domain.LemmaOccurrence(nil), eligible[:topUnknownCount]...)
	result.UnknownConcentration = projection(eligible, concentrationSize, eligibleTokens, result.KnownTokenCount, result.AnalyzableTokenCount)
	result.Projections = make([]domain.CoverageProjection, 0, len(projectionSizes))
	for _, size := range projectionSizes {
		result.Projections = append(result.Projections, projection(eligible, size, eligibleTokens, result.KnownTokenCount, result.AnalyzableTokenCount))
	}
	result.Thresholds = make([]domain.CoverageThreshold, 0, len(thresholdTargets))
	for _, target := range thresholdTargets {
		threshold := domain.CoverageThreshold{TargetPercent: target, EligibleTokenCount: eligibleTokens}
		for _, lemma := range eligible {
			if reachesThreshold(result.KnownTokenCount, threshold.OccurrenceCount, result.AnalyzableTokenCount, target) {
				break
			}
			threshold.LemmaCount++
			threshold.OccurrenceCount += lemma.OccurrenceCount
		}
		threshold.Reachable = reachesThreshold(result.KnownTokenCount, threshold.OccurrenceCount, result.AnalyzableTokenCount, target)
		result.Thresholds = append(result.Thresholds, threshold)
	}
	return result, eligible, nil
}

func projection(eligible []domain.LemmaOccurrence, size, eligibleTokens, knownTokens, analyzableTokens int64) domain.CoverageProjection {
	result := domain.CoverageProjection{TopLemmaCount: size, EligibleTokenCount: eligibleTokens, ProjectedTokenCount: knownTokens}
	for i := 0; i < len(eligible) && int64(i) < size; i++ {
		result.SelectedLemmaCount++
		result.OccurrenceCount += eligible[i].OccurrenceCount
	}
	result.ProjectedTokenCount = min(result.ProjectedTokenCount+result.OccurrenceCount, analyzableTokens)
	return result
}

func reachesThreshold(knownTokens, selectedTokens, analyzableTokens int64, target int) bool {
	return (knownTokens+selectedTokens)*100 >= analyzableTokens*int64(target)
}

func identity(lemma, upos string) string { return lemma + "\x00" + upos }

func occurrenceKey(value domain.LemmaOccurrence) string {
	return value.Language + "\x00" + value.CanonicalLemma + "\x00" + value.UPOS
}
