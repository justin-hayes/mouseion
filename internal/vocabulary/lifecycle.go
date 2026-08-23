// Package vocabulary owns the per-user vocabulary lifecycle state machine.
package vocabulary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type State string

const (
	Candidate State = "candidate"
	Accepted  State = "accepted"
	// Generated is retained as legacy lifecycle bookkeeping. Export exclusion
	// uses the explicit generated_vocabulary record instead.
	Generated State = "generated"
	Ignored   State = "ignored"
	Known     State = "known"
)

var ErrInvalidTransition = errors.New("vocabulary: invalid transition")

type Identity struct {
	Language       string `json:"language"`
	CanonicalLemma string `json:"canonical_lemma"`
	UPOS           string `json:"upos"`
}

type Store interface {
	GetVocabularyStateByIdentity(context.Context, string, string, string, string) (domain.VocabularyState, error)
	PutVocabularyTransition(context.Context, string, string, string, string, string, domain.ProcessingHistory) (domain.VocabularyState, error)
}

type Lifecycle struct{ store Store }

func NewLifecycle(store Store) *Lifecycle { return &Lifecycle{store: store} }

var allowed = map[State]map[State]bool{
	Candidate: {Accepted: true, Ignored: true, Known: true},
	Accepted:  {Generated: true, Candidate: true, Ignored: true, Known: true},
	Generated: {Known: true},
	Ignored:   {Known: true},
	Known:     {},
}

// Transition creates a candidate or performs an ordinary lifecycle transition.
// Asking for the current state is idempotent and does not append audit history.
func (l *Lifecycle) Transition(ctx context.Context, owner string, identity Identity, to State) (domain.VocabularyState, error) {
	return l.transition(ctx, owner, identity, to, false)
}

// Reset explicitly reopens an accepted, generated, ignored, or known item as a candidate.
func (l *Lifecycle) Reset(ctx context.Context, owner string, identity Identity) (domain.VocabularyState, error) {
	return l.transition(ctx, owner, identity, Candidate, true)
}

func (l *Lifecycle) transition(ctx context.Context, owner string, identity Identity, to State, reset bool) (domain.VocabularyState, error) {
	if owner == "" || identity.Language == "" || identity.CanonicalLemma == "" || identity.UPOS == "" || !validState(to) {
		return domain.VocabularyState{}, fmt.Errorf("%w: invalid owner, identity, or state", ErrInvalidTransition)
	}

	current, err := l.store.GetVocabularyStateByIdentity(ctx, owner, identity.Language, identity.CanonicalLemma, identity.UPOS)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return domain.VocabularyState{}, fmt.Errorf("get vocabulary state: %w", err)
	}
	if errors.Is(err, persistence.ErrNotFound) {
		if reset || to != Candidate {
			return domain.VocabularyState{}, fmt.Errorf("%w: new items must enter as candidate", ErrInvalidTransition)
		}
		return l.persist(ctx, owner, identity, "", to, false)
	}

	from := State(current.State)
	if from == to {
		return current, nil
	}
	if reset {
		if to != Candidate {
			return domain.VocabularyState{}, fmt.Errorf("%w: reset target must be candidate", ErrInvalidTransition)
		}
	} else if !allowed[from][to] {
		return domain.VocabularyState{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, from, to)
	}
	return l.persist(ctx, owner, identity, from, to, reset)
}

func validState(state State) bool {
	return state == Candidate || state == Accepted || state == Generated || state == Ignored || state == Known
}

func (l *Lifecycle) persist(ctx context.Context, owner string, identity Identity, from, to State, reset bool) (domain.VocabularyState, error) {
	details, err := json.Marshal(struct {
		Identity
		From          State `json:"from,omitempty"`
		To            State `json:"to"`
		ExplicitReset bool  `json:"explicit_reset"`
	}{identity, from, to, reset})
	if err != nil {
		return domain.VocabularyState{}, fmt.Errorf("marshal transition audit: %w", err)
	}
	completed := time.Now().UTC()
	state, err := l.store.PutVocabularyTransition(ctx, owner, identity.Language, identity.CanonicalLemma, identity.UPOS, string(to), domain.ProcessingHistory{
		OwnerID: owner, Operation: "vocabulary.transition", Status: "completed",
		Details: details, CompletedAt: &completed,
	})
	if err != nil {
		return domain.VocabularyState{}, fmt.Errorf("persist vocabulary transition: %w", err)
	}
	return state, nil
}
