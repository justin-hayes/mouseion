// Package ranking deterministically orders selected vocabulary candidates.
package ranking

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

var ErrInvalidConfig = errors.New("ranking: invalid configuration")

type FrequencyLookup interface {
	FrequencyPercentile(context.Context, string, string, string) (float64, bool, error)
}

type Store interface {
	PutRankingComponents(context.Context, string, string, string, string, string, domain.RankingComponents) error
}

type Config struct {
	CorpusID                                   string
	GlobalWeight, CorpusWeight, PriorityWeight float64
	CrossTextWeight, GlobalFrequencyCutoff     float64
	CrossTextCap                               int
}

func DefaultConfig(corpusID string) Config {
	return Config{CorpusID: corpusID, GlobalWeight: .6, CorpusWeight: .3, PriorityWeight: .1, CrossTextWeight: .05, GlobalFrequencyCutoff: .95, CrossTextCap: 5}
}

type RankedCandidate struct {
	Candidate  selection.Candidate
	Components domain.RankingComponents
}

type Service struct {
	store     Store
	frequency FrequencyLookup
}

func NewService(store Store, frequency FrequencyLookup) *Service {
	return &Service{store: store, frequency: frequency}
}

// Rank uses occurrence_count/max_occurrence_count for deterministic in-corpus
// normalization. Cross-text recurrence is the number of distinct source IDs.
func (s *Service) Rank(ctx context.Context, owner string, candidates []selection.Candidate, cfg Config) ([]RankedCandidate, error) {
	if owner == "" || cfg.CorpusID == "" || cfg.CrossTextCap < 1 || invalidUnit(cfg.GlobalFrequencyCutoff) || invalidWeight(cfg.GlobalWeight) || invalidWeight(cfg.CorpusWeight) || invalidWeight(cfg.PriorityWeight) || invalidWeight(cfg.CrossTextWeight) {
		return nil, ErrInvalidConfig
	}
	maxOccurrence := 0
	for _, candidate := range candidates {
		if candidate.OccurrenceCount < 1 || strings.TrimSpace(candidate.Identity.Language) == "" || strings.TrimSpace(candidate.Identity.CanonicalLemma) == "" || strings.TrimSpace(candidate.Identity.UPOS) == "" {
			return nil, ErrInvalidConfig
		}
		maxOccurrence = max(maxOccurrence, candidate.OccurrenceCount)
	}
	result := make([]RankedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		id := candidate.Identity
		global, found, err := s.frequency.FrequencyPercentile(ctx, id.Language, id.CanonicalLemma, id.UPOS)
		if err != nil {
			return nil, fmt.Errorf("frequency lookup %s/%s/%s: %w", id.Language, id.CanonicalLemma, id.UPOS, err)
		}
		if !found || global < cfg.GlobalFrequencyCutoff {
			global = 0
		}
		corpus := float64(candidate.OccurrenceCount) / float64(maxOccurrence)
		priority := candidate.Provenance.Priority
		crossText := distinctSources(candidate)
		crossText = min(crossText, cfg.CrossTextCap)
		priorityValue := 0.0
		if priority {
			priorityValue = 1
		}
		score := cfg.GlobalWeight*global + cfg.CorpusWeight*corpus + cfg.PriorityWeight*priorityValue + cfg.CrossTextWeight*float64(crossText-1)
		components := domain.RankingComponents{GlobalPercentile: global, CorpusPercentile: corpus, Priority: priority, CrossText: crossText, Score: score}
		if err = s.store.PutRankingComponents(ctx, owner, cfg.CorpusID, id.Language, id.CanonicalLemma, id.UPOS, components); err != nil {
			return nil, fmt.Errorf("persist ranking %s/%s/%s: %w", id.Language, id.CanonicalLemma, id.UPOS, err)
		}
		result = append(result, RankedCandidate{Candidate: candidate, Components: components})
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Components.Score != b.Components.Score {
			return a.Components.Score > b.Components.Score
		}
		if a.Candidate.Identity.Language != b.Candidate.Identity.Language {
			return a.Candidate.Identity.Language < b.Candidate.Identity.Language
		}
		if a.Candidate.Identity.CanonicalLemma != b.Candidate.Identity.CanonicalLemma {
			return a.Candidate.Identity.CanonicalLemma < b.Candidate.Identity.CanonicalLemma
		}
		return a.Candidate.Identity.UPOS < b.Candidate.Identity.UPOS
	})
	return result, nil
}

func distinctSources(candidate selection.Candidate) int {
	sources := map[string]struct{}{}
	for _, ref := range candidate.SentenceReferences {
		if id := strings.TrimSpace(ref.Location.SourceDocumentID); id != "" {
			sources[id] = struct{}{}
		}
	}
	return max(1, len(sources))
}

func invalidWeight(value float64) bool { return math.IsNaN(value) || math.IsInf(value, 0) || value < 0 }
func invalidUnit(value float64) bool   { return invalidWeight(value) || value > 1 }
