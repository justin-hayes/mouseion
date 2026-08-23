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
	"github.com/justin-hayes/mouseion/internal/persistence"
)

var ErrInvalidConfig = errors.New("selection: invalid configuration")

type Store interface {
	GetVocabularyStateByIdentity(context.Context, string, string, string, string) (domain.VocabularyState, error)
	IsKnownVocabularyIdentity(context.Context, string, string, string, string) (bool, error)
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
	CorpusID             string
	MinOccurrences       int
	AllowedPOS           map[string]bool
	IncludeNamedEntities bool
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

func (s *Service) Select(ctx context.Context, owner string, corpus analyzer.Result, cfg SelectionConfig) ([]Candidate, error) {
	if owner == "" || corpus.Language == "" || cfg.CorpusID == "" || cfg.MinOccurrences < 1 || cfg.AllowedPOS == nil {
		return nil, ErrInvalidConfig
	}
	aggs := map[Identity]*aggregate{}
	for si, sentence := range corpus.Sentences {
		for _, token := range sentence.Tokens {
			id := Identity{corpus.Language, strings.TrimSpace(token.CanonicalLemma), strings.ToUpper(strings.TrimSpace(token.UPOS))}
			if id.CanonicalLemma == "" || !cfg.AllowedPOS[id.UPOS] || (token.NamedEntity != nil && !cfg.IncludeNamedEntities) {
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
		if a.count < cfg.MinOccurrences {
			continue
		}
		state, err := s.store.GetVocabularyStateByIdentity(ctx, owner, id.Language, id.CanonicalLemma, id.UPOS)
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return nil, fmt.Errorf("get state: %w", err)
		}
		if err == nil && (state.State == "known" || state.State == "ignored" || state.State == "generated") {
			continue
		}
		known, err := s.store.IsKnownVocabularyIdentity(ctx, owner, id.Language, id.CanonicalLemma, id.UPOS)
		if err != nil {
			return nil, fmt.Errorf("get known vocabulary: %w", err)
		}
		if known {
			continue
		}
		forms := make([]string, 0, len(a.forms))
		for form := range a.forms {
			forms = append(forms, form)
		}
		sort.Strings(forms)
		p := Provenance{cfg.MinOccurrences, a.count}
		c := Candidate{id, a.count, forms, a.refs, p}
		formsJSON, _ := json.Marshal(forms)
		refsJSON, _ := json.Marshal(a.refs)
		provenanceJSON, _ := json.Marshal(p)
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
