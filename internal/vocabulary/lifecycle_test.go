package vocabulary

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type memoryStore struct {
	states  map[string]domain.VocabularyState
	history []domain.ProcessingHistory
}

func newMemoryStore() *memoryStore {
	return &memoryStore{states: make(map[string]domain.VocabularyState)}
}
func key(owner, lang, lemma, upos string) string {
	return owner + "\x00" + lang + "\x00" + lemma + "\x00" + upos
}
func (s *memoryStore) GetVocabularyStateByIdentity(_ context.Context, owner, lang, lemma, upos string) (domain.VocabularyState, error) {
	v, ok := s.states[key(owner, lang, lemma, upos)]
	if !ok {
		return v, persistence.ErrNotFound
	}
	return v, nil
}
func (s *memoryStore) PutVocabularyTransition(_ context.Context, owner, lang, lemma, upos, state string, history domain.ProcessingHistory) (domain.VocabularyState, error) {
	v := domain.VocabularyState{ID: key(owner, lang, lemma, upos), OwnerID: owner, Language: lang, CanonicalLemma: lemma, UPOS: upos, State: state}
	s.states[v.ID] = v
	s.history = append(s.history, history)
	return v, nil
}

func TestLifecycleForwardReversibleAndIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store)
	id := Identity{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"}
	steps := []State{Candidate, Accepted, Generated}
	for _, state := range steps {
		got, err := lifecycle.Transition(ctx, "alice", id, state)
		if err != nil || got.State != string(state) {
			t.Fatalf("transition to %s: %+v, %v", state, got, err)
		}
	}
	if _, err := lifecycle.Transition(ctx, "alice", id, Generated); err != nil {
		t.Fatal(err)
	}
	if len(store.history) != 3 {
		t.Fatalf("idempotent transition wrote history: %d", len(store.history))
	}
	if _, err := lifecycle.Transition(ctx, "alice", id, Candidate); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("implicit reopen: %v", err)
	}
	if got, err := lifecycle.Reset(ctx, "alice", id); err != nil || got.State != string(Candidate) {
		t.Fatalf("reset: %+v, %v", got, err)
	}
	if _, err := lifecycle.Transition(ctx, "alice", id, Ignored); err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Reset(ctx, "alice", id); err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Transition(ctx, "alice", id, Known); err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Transition(ctx, "alice", id, Candidate); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("implicit known reset: %v", err)
	}
	if _, err := lifecycle.Reset(ctx, "alice", id); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleRejectsInvalidTransitionsAndIdentity(t *testing.T) {
	ctx := context.Background()
	lifecycle := NewLifecycle(newMemoryStore())
	id := Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}
	if _, err := lifecycle.Transition(ctx, "alice", id, Accepted); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("new accepted: %v", err)
	}
	if _, err := lifecycle.Reset(ctx, "alice", id); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("reset missing: %v", err)
	}
	if _, err := lifecycle.Transition(ctx, "", id, Candidate); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("empty owner: %v", err)
	}
}

func TestLifecycleIdentityIncludesOwnerLanguageLemmaAndUPOS(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store)
	identities := []struct {
		owner string
		id    Identity
	}{
		{"alice", Identity{"de", "reisen", "VERB"}},
		{"bob", Identity{"de", "reisen", "VERB"}},
		{"alice", Identity{"en", "reisen", "VERB"}},
		{"alice", Identity{"de", "reisen", "NOUN"}},
	}
	for _, item := range identities {
		if _, err := lifecycle.Transition(ctx, item.owner, item.id, Candidate); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.states) != len(identities) {
		t.Fatalf("states = %d", len(store.states))
	}
}
