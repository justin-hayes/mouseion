// Package review exposes UI-independent, owner-scoped vocabulary review and
// curation operations.
package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/ranking"
	"github.com/justin-hayes/mouseion/internal/sentences"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
)

var (
	ErrInvalidInput     = errors.New("review: invalid input")
	ErrNoExample        = errors.New("review: no persisted example sentence")
	ErrInvalidAlternate = errors.New("review: invalid alternate index")
)

// Item is the complete aggregate consumed by a review UI.
type Item struct {
	Candidate  ranking.RankedCandidate
	Enrichment enrichment.Result
	Sentences  sentences.Result
	State      vocabulary.State
}

// Input groups the independently produced pipeline results for one identity.
type Input struct {
	Candidate  ranking.RankedCandidate
	Enrichment enrichment.Result
	Sentences  sentences.Result
}

type Lifecycle interface {
	Transition(context.Context, string, vocabulary.Identity, vocabulary.State) (domain.VocabularyState, error)
	Reset(context.Context, string, vocabulary.Identity) (domain.VocabularyState, error)
}

type Store interface {
	GetVocabularyStateByIdentity(context.Context, string, string, string, string) (domain.VocabularyState, error)
	ListReviewSentences(context.Context, string, string, string, string) ([]domain.ExampleSentence, error)
	CurateReviewSentence(context.Context, string, string, string, string, string, string, string, domain.ProcessingHistory) (domain.CuratedSentence, error)
}

type Service struct {
	lifecycle Lifecycle
	store     Store
	now       func() time.Time
}

func NewService(lifecycle Lifecycle, store Store) *Service {
	return &Service{lifecycle: lifecycle, store: store, now: time.Now}
}

// Present assembles only actionable candidate and accepted items. Handled
// ignored, known, and generated identities therefore do not reappear.
func (s *Service) Present(ctx context.Context, owner string, inputs []Input) ([]Item, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, ErrInvalidInput
	}
	items := make([]Item, 0, len(inputs))
	for _, input := range inputs {
		id := identity(input.Candidate)
		if !validIdentity(id) || !matches(id, input.Enrichment) {
			return nil, ErrInvalidInput
		}
		state, err := s.store.GetVocabularyStateByIdentity(ctx, owner, id.Language, id.CanonicalLemma, id.UPOS)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("get review state: %w", err)
		}
		if state.State == string(vocabulary.Candidate) || state.State == string(vocabulary.Accepted) {
			items = append(items, Item{Candidate: input.Candidate, Enrichment: input.Enrichment, Sentences: input.Sentences, State: vocabulary.State(state.State)})
		}
	}
	return items, nil
}

func (s *Service) Accept(ctx context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	return s.transition(ctx, owner, id, vocabulary.Accepted)
}
func (s *Service) Ignore(ctx context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	return s.transition(ctx, owner, id, vocabulary.Ignored)
}
func (s *Service) MarkKnown(ctx context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	return s.transition(ctx, owner, id, vocabulary.Known)
}
func (s *Service) Reset(ctx context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	if !valid(owner, id) {
		return domain.VocabularyState{}, ErrInvalidInput
	}
	return s.lifecycle.Reset(ctx, owner, id)
}

func (s *Service) transition(ctx context.Context, owner string, id vocabulary.Identity, to vocabulary.State) (domain.VocabularyState, error) {
	if !valid(owner, id) {
		return domain.VocabularyState{}, ErrInvalidInput
	}
	return s.lifecycle.Transition(ctx, owner, id, to)
}

func (s *Service) EditExample(ctx context.Context, owner string, id vocabulary.Identity, newSentence string) (domain.CuratedSentence, error) {
	if strings.TrimSpace(newSentence) == "" {
		return domain.CuratedSentence{}, ErrInvalidInput
	}
	examples, err := s.examples(ctx, owner, id)
	if err != nil {
		return domain.CuratedSentence{}, err
	}
	return s.curate(ctx, owner, id, examples[0].ID, newSentence, "edited", "review.edit_example")
}

func (s *Service) ChooseAlternate(ctx context.Context, owner string, id vocabulary.Identity, alternateIndex int) (domain.CuratedSentence, error) {
	examples, err := s.examples(ctx, owner, id)
	if err != nil {
		return domain.CuratedSentence{}, err
	}
	index := alternateIndex + 1
	if alternateIndex < 0 || index >= len(examples) {
		return domain.CuratedSentence{}, ErrInvalidAlternate
	}
	return s.curate(ctx, owner, id, examples[index].ID, "", fmt.Sprintf("alternate:%d", alternateIndex), "review.choose_alternate")
}

func (s *Service) examples(ctx context.Context, owner string, id vocabulary.Identity) ([]domain.ExampleSentence, error) {
	if !valid(owner, id) {
		return nil, ErrInvalidInput
	}
	examples, err := s.store.ListReviewSentences(ctx, owner, id.Language, id.CanonicalLemma, id.UPOS)
	if err != nil {
		return nil, fmt.Errorf("list review sentences: %w", err)
	}
	if len(examples) == 0 {
		return nil, ErrNoExample
	}
	return examples, nil
}

func (s *Service) curate(ctx context.Context, owner string, id vocabulary.Identity, exampleID, editedText, provenance, operation string) (domain.CuratedSentence, error) {
	details, err := json.Marshal(struct {
		vocabulary.Identity
		ExampleID  string `json:"example_sentence_id"`
		Provenance string `json:"provenance"`
	}{id, exampleID, provenance})
	if err != nil {
		return domain.CuratedSentence{}, err
	}
	completed := s.now().UTC()
	curated, err := s.store.CurateReviewSentence(ctx, owner, exampleID, id.Language, id.CanonicalLemma, id.UPOS, editedText, provenance, domain.ProcessingHistory{OwnerID: owner, Operation: operation, Status: "completed", Details: details, CompletedAt: &completed})
	if err != nil {
		return domain.CuratedSentence{}, fmt.Errorf("persist review curation: %w", err)
	}
	return curated, nil
}

func identity(candidate ranking.RankedCandidate) vocabulary.Identity {
	id := candidate.Candidate.Identity
	return vocabulary.Identity{Language: id.Language, CanonicalLemma: id.CanonicalLemma, UPOS: id.UPOS}
}
func matches(id vocabulary.Identity, result enrichment.Result) bool {
	got := result.Candidate.Identity
	return got.Language == id.Language && got.CanonicalLemma == id.CanonicalLemma && got.UPOS == id.UPOS
}
func valid(owner string, id vocabulary.Identity) bool {
	return strings.TrimSpace(owner) != "" && validIdentity(id)
}
func validIdentity(id vocabulary.Identity) bool {
	return strings.TrimSpace(id.Language) != "" && strings.TrimSpace(id.CanonicalLemma) != "" && strings.TrimSpace(id.UPOS) != ""
}
