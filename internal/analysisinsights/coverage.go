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
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
	ListReservedVocabulary(context.Context, string, string) ([]domain.DeckPreparationVocabulary, error)
	ListUnattachedGeneratedVocabulary(context.Context, string, string) ([]domain.GeneratedVocabulary, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

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

	return s.coverage(ctx, owner, projected.AnalysisCorpusVocabulary, false)
}

func (s *Service) coverage(ctx context.Context, owner string, input domain.AnalysisCorpusVocabulary, reservedIsKnown bool) (domain.AnalysisCoverage, error) {
	vocabularies := make(map[string]vocabulary)
	for _, lemma := range input.Lemmas {
		if _, ok := vocabularies[lemma.Language]; ok {
			continue
		}
		loaded, err := s.loadVocabulary(ctx, owner, lemma.Language)
		if err != nil {
			return domain.AnalysisCoverage{}, err
		}
		vocabularies[lemma.Language] = loaded
	}
	result, _, err := coverageWithVocabulary(input, vocabularies, reservedIsKnown)
	return result, err
}

type vocabulary struct {
	known     map[string]bool
	reserved  map[string]bool
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

	generated, err := s.store.ListUnattachedGeneratedVocabulary(ctx, owner, language)
	if err != nil {
		return vocabulary{}, fmt.Errorf("list generated vocabulary for %s: %w", language, err)
	}
	reservedWords, err := s.store.ListReservedVocabulary(ctx, owner, language)
	if err != nil {
		return vocabulary{}, fmt.Errorf("list reserved vocabulary for %s: %w", language, err)
	}
	reserved := make(map[string]bool, len(reservedWords))
	for _, word := range reservedWords {
		reserved[identity(word.CanonicalLemma, word.UPOS)] = true
	}
	return vocabulary{known: known, reserved: reserved, generated: generated}, nil
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

// coverageWithVocabulary is the shared arithmetic path for coverage results.
// eligible contains the complete eligible identity list;
// the public per-book result retains its existing top-five presentation cap.
func coverageWithVocabulary(input domain.AnalysisCorpusVocabulary, vocabularies map[string]vocabulary, reservedIsKnown bool) (domain.AnalysisCoverage, []domain.LemmaOccurrence, error) {
	result := domain.AnalysisCoverage{
		SourceMaterialID:     input.SourceMaterialID,
		AnalysisRunID:        input.AnalysisRunID,
		AnalyzableTokenCount: input.Statistics.AnalyzableTokenCount,
		DistinctLemmaCount:   input.Statistics.DistinctLemmaCount,
		TextProfile:          input.Statistics.TextProfile,
	}
	eligible := make([]domain.LemmaOccurrence, 0, len(input.Lemmas))
	for _, lemma := range input.Lemmas {
		vocab := vocabularies[lemma.Language]

		key := identity(lemma.CanonicalLemma, lemma.UPOS)
		reservedMatch := vocab.reserved[key] || vocab.reserved[identity(lemma.CanonicalLemma, "")]
		if vocab.known[key] || vocab.known[identity(lemma.CanonicalLemma, "")] || (reservedIsKnown && reservedMatch) {
			result.KnownTokenCount += lemma.OccurrenceCount
			result.KnownLemmaCount++
			continue
		}
		result.UnknownLemmaCount++
		if reservedMatch {
			result.ReservedTokenCount += lemma.OccurrenceCount
			result.ReservedLemmaCount++
		}
		if !vocab.generatedFor(input.SourceMaterialID, key) && !reservedMatch {
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
