// Package analysisinsights calculates learner-specific metrics for analyzed books.
package analysisinsights

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/justin-hayes/mouseion/internal/domain"
)

var ErrStatisticsUnavailable = errors.New("analysis insights: corpus statistics unavailable")

// ErrCountsUpdating reports that the Book's effective vocabulary counts are not
// ready. Coverage is then withheld rather than derived from raw corpus counts.
var ErrCountsUpdating = errors.New("analysis insights: vocabulary counts updating")

var thresholdTargets = [...]int{95, 97, 99}
var projectionSizes = [...]int64{10, 25, 50}

const (
	topUnknownLimit   = 5
	concentrationSize = int64(10)
)

type Store interface {
	GetProjectedCorpusVocabulary(context.Context, string, string) (domain.ProjectedCorpusVocabulary, error)
	ListStudyProgress(context.Context, string, string) ([]domain.StudyProgress, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

// Coverage is the Book's coverage by Study progress (ADR 0136 §3): the share of
// analyzable word tokens whose effective vocabulary identity is Mature, and a
// lighter figure that also counts words in study. The counts are the Browse
// count projection, so confirmed Lemma corrections apply; the denominator is
// the persisted analyzable-token total, so excluded occurrences stay in it.
func (s *Service) Coverage(ctx context.Context, owner, corpusID string) (domain.AnalysisCoverage, error) {
	projected, err := s.store.GetProjectedCorpusVocabulary(ctx, owner, corpusID)
	if err != nil {
		return domain.AnalysisCoverage{}, fmt.Errorf("load projected corpus vocabulary: %w", err)
	}
	if projected.Statistics == nil {
		return domain.AnalysisCoverage{}, ErrStatisticsUnavailable
	}
	if !projected.Ready {
		return domain.AnalysisCoverage{}, ErrCountsUpdating
	}

	indexes := make(map[string]domain.StudyCoverageIndex)
	for _, lemma := range projected.Lemmas {
		if _, ok := indexes[lemma.Language]; ok {
			continue
		}
		progress, err := s.store.ListStudyProgress(ctx, owner, lemma.Language)
		if err != nil {
			return domain.AnalysisCoverage{}, fmt.Errorf("list study progress for %s: %w", lemma.Language, err)
		}
		indexes[lemma.Language] = domain.NewStudyCoverageIndex(progress)
	}
	return coverageWithStudy(projected.AnalysisCorpusVocabulary, indexes), nil
}

// coverageWithStudy is the shared arithmetic path for coverage results. The
// public per-book result keeps its top-five presentation cap.
func coverageWithStudy(input domain.AnalysisCorpusVocabulary, indexes map[string]domain.StudyCoverageIndex) domain.AnalysisCoverage {
	result := domain.AnalysisCoverage{
		SourceMaterialID:     input.SourceMaterialID,
		AnalysisRunID:        input.AnalysisRunID,
		AnalyzableTokenCount: input.Statistics.AnalyzableTokenCount,
		DistinctLemmaCount:   input.Statistics.DistinctLemmaCount,
		TextProfile:          input.Statistics.TextProfile,
	}
	eligible := classifyOccurrences(&result, input, indexes)
	// Persisted statistics are authoritative. Clamp learner-derived values so
	// malformed legacy rows can never produce impossible coverage output.
	result.MatureTokenCount = min(result.MatureTokenCount, result.AnalyzableTokenCount)
	result.InStudyTokenCount = min(max(result.InStudyTokenCount, result.MatureTokenCount), result.AnalyzableTokenCount)
	result.UnknownTokenCount = max(result.AnalyzableTokenCount-result.MatureTokenCount, 0)

	sortEligible(eligible)
	eligibleTokens := eligibleTokenCount(eligible)
	topUnknownCount := min(topUnknownLimit, len(eligible))
	result.TopUnknownLemmas = append([]domain.LemmaOccurrence(nil), eligible[:topUnknownCount]...)
	result.UnknownConcentration = projection(eligible, concentrationSize, eligibleTokens, result.MatureTokenCount, result.AnalyzableTokenCount)
	result.Projections = coverageProjections(eligible, eligibleTokens, result.MatureTokenCount, result.AnalyzableTokenCount)
	result.Thresholds = coverageThresholds(eligible, eligibleTokens, result.MatureTokenCount, result.AnalyzableTokenCount)
	result.ToTarget = coverageToTarget(eligible, eligibleTokens, result.InStudyTokenCount, result.AnalyzableTokenCount)
	return result
}

// classifyOccurrences accumulates the Mature, in-study and unknown counts onto
// result and returns the eligible identities, those not yet in study, in input
// order.
func classifyOccurrences(result *domain.AnalysisCoverage, input domain.AnalysisCorpusVocabulary, indexes map[string]domain.StudyCoverageIndex) []domain.LemmaOccurrence {
	eligible := make([]domain.LemmaOccurrence, 0, len(input.Lemmas))
	for _, lemma := range input.Lemmas {
		inStudy, mature := indexes[lemma.Language].Classify(lemma)
		if inStudy {
			result.InStudyTokenCount += lemma.OccurrenceCount
			result.InStudyLemmaCount++
		}
		if mature {
			result.MatureTokenCount += lemma.OccurrenceCount
			result.MatureLemmaCount++
			continue
		}
		result.UnknownLemmaCount++
		if !inStudy {
			eligible = append(eligible, lemma)
		}
	}
	return eligible
}

// sortEligible orders unknown identities by descending occurrence count then
// stable key, the deterministic order the projections consume.
func sortEligible(eligible []domain.LemmaOccurrence) {
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].OccurrenceCount != eligible[j].OccurrenceCount {
			return eligible[i].OccurrenceCount > eligible[j].OccurrenceCount
		}
		return occurrenceKey(eligible[i]) < occurrenceKey(eligible[j])
	})
}

func eligibleTokenCount(eligible []domain.LemmaOccurrence) int64 {
	var total int64
	for _, lemma := range eligible {
		total += lemma.OccurrenceCount
	}
	return total
}

// coverageProjections builds the fixed-size projections of the unknown pool.
func coverageProjections(eligible []domain.LemmaOccurrence, eligibleTokens, matureTokens, analyzableTokens int64) []domain.CoverageProjection {
	projections := make([]domain.CoverageProjection, 0, len(projectionSizes))
	for _, size := range projectionSizes {
		projections = append(projections, projection(eligible, size, eligibleTokens, matureTokens, analyzableTokens))
	}
	return projections
}

// coverageThresholds builds the reachable-target thresholds over the unknown
// pool in its deterministic order.
func coverageThresholds(eligible []domain.LemmaOccurrence, eligibleTokens, matureTokens, analyzableTokens int64) []domain.CoverageThreshold {
	thresholds := make([]domain.CoverageThreshold, 0, len(thresholdTargets))
	for _, target := range thresholdTargets {
		threshold := domain.CoverageThreshold{TargetPercent: target, EligibleTokenCount: eligibleTokens}
		for _, lemma := range eligible {
			if reachesThreshold(matureTokens, threshold.OccurrenceCount, analyzableTokens, target) {
				break
			}
			threshold.LemmaCount++
			threshold.OccurrenceCount += lemma.OccurrenceCount
		}
		threshold.Reachable = reachesThreshold(matureTokens, threshold.OccurrenceCount, analyzableTokens, target)
		thresholds = append(thresholds, threshold)
	}
	return thresholds
}

// coverageToTarget counts the not-yet-in-study words, most frequent first,
// that lift the in-study share to Target coverage (ADR 0136 §2).
func coverageToTarget(eligible []domain.LemmaOccurrence, eligibleTokens, inStudyTokens, analyzableTokens int64) domain.CoverageThreshold {
	target := int(domain.StudyTargetCoveragePercent)
	threshold := domain.CoverageThreshold{TargetPercent: target, EligibleTokenCount: eligibleTokens}
	for _, lemma := range eligible {
		if reachesThreshold(inStudyTokens, threshold.OccurrenceCount, analyzableTokens, target) {
			break
		}
		threshold.LemmaCount++
		threshold.OccurrenceCount += lemma.OccurrenceCount
	}
	threshold.Reachable = reachesThreshold(inStudyTokens, threshold.OccurrenceCount, analyzableTokens, target)
	return threshold
}

func projection(eligible []domain.LemmaOccurrence, size, eligibleTokens, matureTokens, analyzableTokens int64) domain.CoverageProjection {
	result := domain.CoverageProjection{TopLemmaCount: size, EligibleTokenCount: eligibleTokens, ProjectedTokenCount: matureTokens}
	for i := 0; i < len(eligible) && int64(i) < size; i++ {
		result.SelectedLemmaCount++
		result.OccurrenceCount += eligible[i].OccurrenceCount
	}
	result.ProjectedTokenCount = min(result.ProjectedTokenCount+result.OccurrenceCount, analyzableTokens)
	return result
}

func reachesThreshold(matureTokens, selectedTokens, analyzableTokens int64, target int) bool {
	return (matureTokens+selectedTokens)*100 >= analyzableTokens*int64(target)
}

func occurrenceKey(value domain.LemmaOccurrence) string {
	return value.Language + "\x00" + value.CanonicalLemma + "\x00" + value.UPOS + "\x00" + value.LexicalDiscriminator
}
