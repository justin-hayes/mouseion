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
	result := domain.AnalysisCoverage{
		SourceMaterialID:     input.SourceMaterialID,
		ReviewedScopeID:      input.ReviewedScopeID,
		AnalysisRunID:        input.AnalysisRunID,
		SelectedUnits:        append([]domain.CorpusSelectedUnit(nil), input.SelectedUnits...),
		AnalyzableTokenCount: input.Statistics.AnalyzableTokenCount,
		DistinctLemmaCount:   input.Statistics.DistinctLemmaCount,
		TextProfile:          input.Statistics.TextProfile,
	}
	eligible := make([]domain.LemmaOccurrence, 0, len(input.Lemmas))
	knownByLanguage := map[string]map[string]bool{}
	generatedByLanguage := map[string]map[string]bool{}
	activeByLanguage := map[string]map[string]bool{}
	for _, lemma := range input.Lemmas {
		if !lexical.IsLemma(lemma.CanonicalLemma) {
			result.AnalyzableTokenCount = max(result.AnalyzableTokenCount-lemma.OccurrenceCount, 0)
			result.DistinctLemmaCount = max(result.DistinctLemmaCount-1, 0)
			continue
		}
		known, ok := knownByLanguage[lemma.Language]
		if !ok {
			words, listErr := s.store.ListKnownVocabulary(ctx, owner, lemma.Language)
			if listErr != nil {
				return domain.AnalysisCoverage{}, fmt.Errorf("list known vocabulary for %s: %w", lemma.Language, listErr)
			}
			known = make(map[string]bool, len(words))
			for _, word := range words {
				known[identity(word.CanonicalLemma, word.UPOS)] = true
			}
			knownByLanguage[lemma.Language] = known
		}
		generated, ok := generatedByLanguage[lemma.Language]
		if !ok {
			words, listErr := s.store.ListLegacyGeneratedVocabulary(ctx, owner, lemma.Language)
			if listErr != nil {
				return domain.AnalysisCoverage{}, fmt.Errorf("list generated vocabulary for %s: %w", lemma.Language, listErr)
			}
			generated = make(map[string]bool, len(words))
			for _, word := range words {
				if word.FirstSourceMaterialID == nil || *word.FirstSourceMaterialID != input.SourceMaterialID {
					generated[identity(word.CanonicalLemma, word.UPOS)] = true
				}
			}
			generatedByLanguage[lemma.Language] = generated
		}
		active, ok := activeByLanguage[lemma.Language]
		if !ok {
			words, listErr := s.store.ListActiveLearningCampaignVocabulary(ctx, owner, lemma.Language)
			if listErr != nil {
				return domain.AnalysisCoverage{}, fmt.Errorf("list active campaign vocabulary for %s: %w", lemma.Language, listErr)
			}
			active = make(map[string]bool, len(words))
			for _, word := range words {
				active[identity(word.CanonicalLemma, word.UPOS)] = true
			}
			activeByLanguage[lemma.Language] = active
		}

		key := identity(lemma.CanonicalLemma, lemma.UPOS)
		activeMatch := active[key] || active[identity(lemma.CanonicalLemma, "")]
		if known[key] || known[identity(lemma.CanonicalLemma, "")] || (activeIsKnown && activeMatch) {
			result.KnownTokenCount += lemma.OccurrenceCount
			result.KnownLemmaCount++
			continue
		}
		result.UnknownLemmaCount++
		if activeMatch {
			result.ActiveCampaignTokenCount += lemma.OccurrenceCount
			result.ActiveCampaignLemmaCount++
		}
		if !generated[key] && !activeMatch {
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
	return result, nil
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
