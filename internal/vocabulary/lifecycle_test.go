package vocabulary

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.NoError(t, err)
		assert.Equal(t, string(state), got.State, "transition to %s", state)
	}
	_, err := lifecycle.Transition(ctx, "alice", id, Generated)
	require.NoError(t, err)
	assert.Len(t, store.history, 3, "idempotent transition wrote history")
	_, err = lifecycle.Transition(ctx, "alice", id, Candidate)
	assert.ErrorIs(t, err, ErrInvalidTransition, "implicit reopen")
	got, err := lifecycle.Reset(ctx, "alice", id)
	require.NoError(t, err)
	assert.Equal(t, string(Candidate), got.State)
	_, err = lifecycle.Transition(ctx, "alice", id, Ignored)
	require.NoError(t, err)
	_, err = lifecycle.Reset(ctx, "alice", id)
	require.NoError(t, err)
	_, err = lifecycle.Transition(ctx, "alice", id, Known)
	require.NoError(t, err)
	_, err = lifecycle.Transition(ctx, "alice", id, Candidate)
	assert.ErrorIs(t, err, ErrInvalidTransition, "implicit known reset")
	_, err = lifecycle.Reset(ctx, "alice", id)
	require.NoError(t, err)
}

func TestLifecycleRejectsInvalidTransitionsAndIdentity(t *testing.T) {
	ctx := context.Background()
	lifecycle := NewLifecycle(newMemoryStore())
	id := Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}
	_, err := lifecycle.Transition(ctx, "alice", id, Accepted)
	assert.ErrorIs(t, err, ErrInvalidTransition, "new accepted")
	_, err = lifecycle.Reset(ctx, "alice", id)
	assert.ErrorIs(t, err, ErrInvalidTransition, "reset missing")
	_, err = lifecycle.Transition(ctx, "", id, Candidate)
	assert.ErrorIs(t, err, ErrInvalidTransition, "empty owner")
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
		_, err := lifecycle.Transition(ctx, item.owner, item.id, Candidate)
		require.NoError(t, err)
	}
	assert.Len(t, store.states, len(identities), "states = %d", len(store.states))
}
