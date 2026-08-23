package selection

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type memoryStore struct {
	states map[string]string
	known  map[string]bool
	saved  []domain.SelectionCandidate
}

func (m *memoryStore) key(owner, lang, lemma, upos string) string {
	return owner + "/" + lang + "/" + lemma + "/" + upos
}
func (m *memoryStore) GetVocabularyStateByIdentity(_ context.Context, o, l, x, p string) (domain.VocabularyState, error) {
	v, ok := m.states[m.key(o, l, x, p)]
	if !ok {
		return domain.VocabularyState{}, persistence.ErrNotFound
	}
	return domain.VocabularyState{State: v}, nil
}
func (m *memoryStore) IsKnownVocabularyIdentity(_ context.Context, o, l, x, p string) (bool, error) {
	return m.known[m.key(o, l, x, p)], nil
}
func (m *memoryStore) PutSelectionCandidate(_ context.Context, c domain.SelectionCandidate) (bool, error) {
	m.saved = append(m.saved, c)
	m.states[m.key(c.OwnerID, c.Language, c.CanonicalLemma, c.UPOS)] = "candidate"
	return true, nil
}

func tok(surface, lemma, upos string, ner bool) analyzer.Token {
	var entity *string
	if ner {
		v := "PERSON"
		entity = &v
	}
	return analyzer.Token{Surface: surface, CanonicalLemma: lemma, UPOS: upos, NamedEntity: entity}
}
func fixture(tokens ...analyzer.Token) analyzer.Result {
	sentences := make([]analyzer.Sentence, len(tokens))
	for i, t := range tokens {
		sentences[i] = analyzer.Sentence{Text: t.Surface, Tokens: []analyzer.Token{t}, Location: analyzer.SourceLocation{SourceDocumentID: "book"}}
	}
	return analyzer.Result{Language: "de", Sentences: sentences}
}

func TestDefaultRulesFiltersAggregationAndDeterminism(t *testing.T) {
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}}
	svc := NewService(store)
	cfg := DefaultConfig("corpus-1")
	corpus := fixture(tok("Häuser", "Haus", "NOUN", false), tok("Haus", "Haus", "noun", false), tok("selten", "selten", "ADJ", false), tok("der", "der", "DET", false), tok("Berlin", "Berlin", "PROPN", false), tok("Anna", "Anna", "NOUN", true))
	got, err := svc.Select(context.Background(), "alice", corpus, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Identity.CanonicalLemma != "Haus" || got[1].Identity.CanonicalLemma != "selten" {
		t.Fatalf("candidates=%+v", got)
	}
	if !reflect.DeepEqual(got[0].ObservedForms, []string{"Haus", "Häuser"}) || got[0].OccurrenceCount != 2 || len(got[0].SentenceReferences) != 2 {
		t.Fatalf("aggregation=%+v", got[0])
	}
}

func TestDefaultIncludesSingletonAndConfigOverridesFilters(t *testing.T) {
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}}
	cfg := DefaultConfig("c")
	if cfg.MinOccurrences != 1 {
		t.Fatalf("MinOccurrences=%d, want 1", cfg.MinOccurrences)
	}
	cfg.AllowedPOS["PROPN"] = true
	cfg.IncludeNamedEntities = true
	got, err := NewService(store).Select(context.Background(), "alice", fixture(tok("Berlin", "Berlin", "PROPN", true)), cfg)
	if err != nil || len(got) != 1 || got[0].OccurrenceCount != 1 || len(store.saved) != 1 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestOwnerScopedExclusions(t *testing.T) {
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}}
	for _, state := range []string{"known", "ignored", "generated"} {
		store.states[store.key("alice", "de", state, "NOUN")] = state
	}
	store.known[store.key("alice", "de", "importiert", "NOUN")] = true
	corpus := fixture(tok("known", "known", "NOUN", false), tok("known", "known", "NOUN", false), tok("ignored", "ignored", "NOUN", false), tok("ignored", "ignored", "NOUN", false), tok("generated", "generated", "NOUN", false), tok("generated", "generated", "NOUN", false), tok("importiert", "importiert", "NOUN", false), tok("importiert", "importiert", "NOUN", false))
	svc := NewService(store)
	alice, err := svc.Select(context.Background(), "alice", corpus, DefaultConfig("a"))
	if err != nil || len(alice) != 0 {
		t.Fatalf("alice=%+v err=%v", alice, err)
	}
	bob, err := svc.Select(context.Background(), "bob", corpus, DefaultConfig("b"))
	if err != nil || len(bob) != 4 {
		t.Fatalf("bob=%+v err=%v", bob, err)
	}
}

func TestInvalidConfig(t *testing.T) {
	_, err := NewService(&memoryStore{}).Select(context.Background(), "", analyzer.Result{}, SelectionConfig{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
}
