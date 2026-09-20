// Package selection generates deterministic, owner-scoped vocabulary candidates.
package selection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
)

var ErrInvalidConfig = errors.New("selection: invalid configuration")

type Store interface {
	IsKnownVocabularyIdentity(context.Context, string, string, string, string) (bool, error)
	IsReservedVocabulary(context.Context, string, string, string, string) (bool, error)
	PutSelectionCandidate(context.Context, domain.SelectionCandidate) (bool, error)
}
type Identity struct{ Language, CanonicalLemma, UPOS string }
type SentenceReference struct {
	SentenceIndex int                     `json:"sentence_index"`
	Text          string                  `json:"text"`
	Location      analyzer.SourceLocation `json:"location"`
}
type Provenance struct {
	MinOccurrences  int `json:"min_occurrences"`
	OccurrenceCount int `json:"occurrence_count"`
}
type Candidate struct {
	Identity           Identity
	OccurrenceCount    int
	ObservedForms      []string
	SentenceReferences []SentenceReference
	Provenance         Provenance
}

type SelectionConfig struct {
	CorpusID       string
	MinOccurrences int
	AllowedPOS     map[string]bool
}

func DefaultConfig(corpusID string) SelectionConfig {
	return SelectionConfig{CorpusID: corpusID, MinOccurrences: 1,
		AllowedPOS: map[string]bool{"NOUN": true, "VERB": true, "ADJ": true, "ADV": true}}
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

type aggregate struct {
	count int
	forms map[string]bool
	refs  []SentenceReference
}

// AnalyzableStatistics calculates the immutable coverage denominator using the
// same token filters as candidate selection. Distinct lemmas are the
// lemma-and-UPOS identities used by vocabulary selection.
func AnalyzableStatistics(corpus analyzer.Result, cfg SelectionConfig) domain.AnalysisStatistics {
	aggs := aggregateTokens(corpus, cfg)
	statistics := domain.AnalysisStatistics{DistinctLemmaCount: int64(len(aggs)), TextProfile: &domain.TextProfile{SentenceCount: int64(len(corpus.Sentences))}}
	profile := statistics.TextProfile
	for _, aggregate := range aggs {
		statistics.AnalyzableTokenCount += int64(aggregate.count)
	}
	lengths := make([]int, 0, len(corpus.Sentences))
	for _, sentence := range corpus.Sentences {
		length := len(sentence.Tokens)
		lengths = append(lengths, length)
		profile.NormalizedTokenCount += int64(length)
		if length == 0 {
			profile.EmptySentenceCount++
		}
		if length > 35 {
			profile.LongSentenceCount++
		}
	}
	if len(lengths) > 0 {
		sort.Ints(lengths)
		middle := len(lengths) / 2
		profile.MedianSentenceTokenCount = float64(lengths[middle])
		if len(lengths)%2 == 0 {
			profile.MedianSentenceTokenCount = float64(lengths[middle-1]+lengths[middle]) / 2
		}
		profile.P90SentenceTokenCount = int64(lengths[(9*len(lengths)+9)/10-1])
	}
	return statistics
}

func aggregateTokens(corpus analyzer.Result, cfg SelectionConfig) map[Identity]*aggregate {
	aggs := map[Identity]*aggregate{}
	for si, sentence := range corpus.Sentences {
		for _, token := range sentence.Tokens {
			id := Identity{corpus.Language, strings.TrimSpace(token.CanonicalLemma), strings.ToUpper(strings.TrimSpace(token.UPOS))}
			if token.Dependency == "compound:prt" || !lexical.IsLemma(id.CanonicalLemma) || !cfg.AllowedPOS[id.UPOS] {
				continue
			}
			a := aggs[id]
			if a == nil {
				a = &aggregate{forms: map[string]bool{}}
				aggs[id] = a
			}
			a.count++
			a.forms[token.Surface] = true
			a.refs = append(a.refs, SentenceReference{si, sentence.Text, token.Location})
		}
	}
	return aggs
}

func (s *Service) Select(ctx context.Context, owner string, corpus analyzer.Result, cfg SelectionConfig) ([]Candidate, error) {
	if owner == "" || corpus.Language == "" || cfg.CorpusID == "" || cfg.MinOccurrences < 1 || cfg.AllowedPOS == nil {
		return nil, ErrInvalidConfig
	}
	aggs := aggregateTokens(corpus, cfg)
	ids := make([]Identity, 0, len(aggs))
	for id := range aggs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].CanonicalLemma != ids[j].CanonicalLemma {
			return ids[i].CanonicalLemma < ids[j].CanonicalLemma
		}
		return ids[i].UPOS < ids[j].UPOS
	})
	out := make([]Candidate, 0)
	for _, id := range ids {
		a := aggs[id]
		known, err := s.store.IsKnownVocabularyIdentity(ctx, owner, id.Language, id.CanonicalLemma, id.UPOS)
		if err != nil {
			return nil, fmt.Errorf("get known vocabulary: %w", err)
		}
		reserved, err := s.store.IsReservedVocabulary(ctx, owner, id.Language, id.CanonicalLemma, id.UPOS)
		if err != nil {
			return nil, fmt.Errorf("get reserved vocabulary: %w", err)
		}
		if !allowsIdentity(a.count, cfg.MinOccurrences, known, reserved) {
			continue
		}
		forms := make([]string, 0, len(a.forms))
		for form := range a.forms {
			forms = append(forms, form)
		}
		sort.Strings(forms)
		p := Provenance{cfg.MinOccurrences, a.count}
		c := Candidate{id, a.count, forms, a.refs, p}
		formsJSON, err := json.Marshal(forms)
		if err != nil {
			return nil, fmt.Errorf("encode observed forms: %w", err)
		}
		refsJSON, err := json.Marshal(a.refs)
		if err != nil {
			return nil, fmt.Errorf("encode sentence references: %w", err)
		}
		provenanceJSON, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("encode candidate provenance: %w", err)
		}
		kept, err := s.store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner, CorpusID: cfg.CorpusID, Language: id.Language, CanonicalLemma: id.CanonicalLemma, UPOS: id.UPOS, OccurrenceCount: a.count, ObservedForms: formsJSON, SentenceReferences: refsJSON, Provenance: provenanceJSON})
		if err != nil {
			return nil, fmt.Errorf("persist candidate: %w", err)
		}
		if kept {
			out = append(out, c)
		}
	}
	return out, nil
}
