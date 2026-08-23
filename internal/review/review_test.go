package review

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/ranking"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/sentences"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
)

type fakeStore struct {
	states   map[string]domain.VocabularyState
	examples map[string][]domain.ExampleSentence
	analysis map[string]domain.ExampleSentence
	curated  []domain.CuratedSentence
	history  []domain.ProcessingHistory
}

func newFakeStore() *fakeStore {
	return &fakeStore{states: map[string]domain.VocabularyState{}, examples: map[string][]domain.ExampleSentence{}, analysis: map[string]domain.ExampleSentence{}}
}
func fakeKey(owner, lang, lemma, upos string) string {
	return owner + ":" + lang + ":" + lemma + ":" + upos
}
func (s *fakeStore) GetVocabularyStateByIdentity(_ context.Context, owner, lang, lemma, upos string) (domain.VocabularyState, error) {
	v, ok := s.states[fakeKey(owner, lang, lemma, upos)]
	if !ok {
		return v, persistence.ErrNotFound
	}
	return v, nil
}
func (s *fakeStore) ListReviewSentences(_ context.Context, owner, lang, lemma, upos string) ([]domain.ExampleSentence, error) {
	return append([]domain.ExampleSentence(nil), s.examples[fakeKey(owner, lang, lemma, upos)]...), nil
}
func (s *fakeStore) PersistReviewSentenceFromAnalysis(_ context.Context, owner, _ string, lang, lemma, upos string) (domain.ExampleSentence, error) {
	key := fakeKey(owner, lang, lemma, upos)
	example, ok := s.analysis[key]
	if !ok || example.Text == "" {
		return domain.ExampleSentence{}, persistence.ErrNotFound
	}
	s.examples[key] = []domain.ExampleSentence{example}
	return example, nil
}
func (s *fakeStore) CurateReviewSentence(_ context.Context, owner, example, lang, lemma, upos, edited, notes string, history domain.ProcessingHistory) (domain.CuratedSentence, error) {
	key := fakeKey(owner, lang, lemma, upos)
	items := s.examples[key]
	found := false
	for i := range items {
		if items[i].ID == example {
			found = true
			if edited != "" {
				items[i].Text = edited
			}
		}
	}
	if !found {
		return domain.CuratedSentence{}, persistence.ErrNotFound
	}
	s.examples[key] = items
	v := domain.CuratedSentence{OwnerID: owner, ExampleSentenceID: example, Language: lang, CanonicalLemma: lemma, UPOS: upos, Notes: notes}
	s.curated = append(s.curated, v)
	s.history = append(s.history, history)
	return v, nil
}

type fakeLifecycle struct{ store *fakeStore }

func (l fakeLifecycle) Transition(_ context.Context, owner string, id vocabulary.Identity, to vocabulary.State) (domain.VocabularyState, error) {
	key := fakeKey(owner, id.Language, id.CanonicalLemma, id.UPOS)
	v, ok := l.store.states[key]
	if !ok {
		return v, vocabulary.ErrInvalidTransition
	}
	v.State = string(to)
	l.store.states[key] = v
	return v, nil
}
func (l fakeLifecycle) Reset(_ context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	key := fakeKey(owner, id.Language, id.CanonicalLemma, id.UPOS)
	v, ok := l.store.states[key]
	if !ok {
		return v, vocabulary.ErrInvalidTransition
	}
	v.State = string(vocabulary.Candidate)
	l.store.states[key] = v
	return v, nil
}

func aggregate(id vocabulary.Identity) Input {
	selectionID := selection.Identity{Language: id.Language, CanonicalLemma: id.CanonicalLemma, UPOS: id.UPOS}
	candidate := selection.Candidate{Identity: selectionID, OccurrenceCount: 3, Provenance: selection.Provenance{OccurrenceCount: 3, Priority: true}}
	chosen := sentences.ScoredSentence{Text: "Das Haus ist alt.", Score: 75}
	return Input{
		Candidate:  ranking.RankedCandidate{Candidate: candidate, Components: domain.RankingComponents{GlobalPercentile: .9, CorpusPercentile: .6, Priority: true, CrossText: 2, Score: .82}},
		Enrichment: enrichment.Result{Candidate: enrichment.Candidate{Identity: enrichment.Identity{Language: id.Language, CanonicalLemma: id.CanonicalLemma, UPOS: id.UPOS}}, Translation: enrichment.Field[string]{Value: "house", Available: true}},
		Sentences:  sentences.Result{Chosen: &chosen, Alternatives: []sentences.ScoredSentence{{Text: "Ein Haus steht dort.", Score: 70}}},
	}
}

func TestPresentAssemblesCompleteDecisionList(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	service := NewService(fakeLifecycle{store}, store)
	ids := []vocabulary.Identity{
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"},
		{Language: "de", CanonicalLemma: "wissen", UPOS: "VERB"},
	}
	store.states[fakeKey("alice", "de", "Haus", "NOUN")] = domain.VocabularyState{State: "candidate"}
	store.states[fakeKey("alice", "de", "gehen", "VERB")] = domain.VocabularyState{State: "accepted"}
	store.states[fakeKey("alice", "de", "wissen", "VERB")] = domain.VocabularyState{State: "known"}
	inputs := []Input{aggregate(ids[0]), aggregate(ids[1]), aggregate(ids[2])}
	items, err := service.Present(ctx, "alice", inputs)
	if err != nil || len(items) != 3 || items[2].State != vocabulary.Known || items[0].Candidate.Components.Score != .82 || items[0].Enrichment.Translation.Value != "house" || items[0].Sentences.Chosen.Text == "" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if bob, err := service.Present(ctx, "bob", inputs); err != nil || len(bob) != 0 {
		t.Fatalf("bob=%+v err=%v", bob, err)
	}
}

func TestLifecycleReviewOperations(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	service := NewService(fakeLifecycle{store}, store)
	id := vocabulary.Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}
	key := fakeKey("alice", "de", "Haus", "NOUN")
	store.states[key] = domain.VocabularyState{OwnerID: "alice", State: "candidate"}
	store.examples[key] = []domain.ExampleSentence{{ID: "chosen", OwnerID: "alice", Chosen: true, Text: "Das Haus ist alt."}}
	if got, err := service.Accept(ctx, "alice", id); err != nil || got.State != "accepted" || len(store.curated) != 1 {
		t.Fatalf("accept=%+v %v", got, err)
	}
	if got, err := service.Ignore(ctx, "alice", id); err != nil || got.State != "ignored" {
		t.Fatalf("ignore=%+v %v", got, err)
	}
	if got, err := service.MarkKnown(ctx, "alice", id); err != nil || got.State != "known" {
		t.Fatalf("known=%+v %v", got, err)
	}
	if got, err := service.Reset(ctx, "alice", id); err != nil || got.State != "candidate" {
		t.Fatalf("reset=%+v %v", got, err)
	}
	if _, err := service.Accept(ctx, "", id); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid owner: %v", err)
	}
}

func TestAcceptFallsBackToAnalysisExample(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	service := NewService(fakeLifecycle{store}, store)
	id := vocabulary.Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}
	key := fakeKey("alice", "de", "Haus", "NOUN")
	store.states[key] = domain.VocabularyState{OwnerID: "alice", State: "candidate"}
	store.analysis[key] = domain.ExampleSentence{ID: "analysis", OwnerID: "alice", Text: "Das Haus ist alt.", Chosen: true}

	got, err := service.Accept(ctx, "alice", id)
	if err != nil || got.State != "accepted" || len(store.curated) != 1 || store.curated[0].ExampleSentenceID != "analysis" {
		t.Fatalf("accept=%+v curated=%+v err=%v", got, store.curated, err)
	}
}

func TestAcceptReturnsClearErrorWithoutAnyExample(t *testing.T) {
	store := newFakeStore()
	id := vocabulary.Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}
	store.states[fakeKey("alice", "de", "Haus", "NOUN")] = domain.VocabularyState{OwnerID: "alice", State: "candidate"}

	_, err := NewService(fakeLifecycle{store}, store).Accept(context.Background(), "alice", id)
	if !errors.Is(err, ErrNoAnalysisExample) {
		t.Fatalf("accept error=%v", err)
	}
}

func TestEditAndChooseAlternateAreOwnerScopedAndAudited(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	service := NewService(fakeLifecycle{store}, store)
	id := vocabulary.Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}
	key := fakeKey("alice", "de", "Haus", "NOUN")
	store.examples[key] = []domain.ExampleSentence{{ID: "chosen", OwnerID: "alice", Chosen: true, Text: "old"}, {ID: "alt", OwnerID: "alice", Text: "alternate"}}
	edited, err := service.EditExample(ctx, "alice", id, "My edited sentence.")
	if err != nil || edited.ExampleSentenceID != "chosen" || store.examples[key][0].Text != "My edited sentence." {
		t.Fatalf("edited=%+v err=%v", edited, err)
	}
	alternate, err := service.ChooseAlternate(ctx, "alice", id, 0)
	if err != nil || alternate.ExampleSentenceID != "alt" {
		t.Fatalf("alternate=%+v err=%v", alternate, err)
	}
	if len(store.history) != 2 || store.history[0].OwnerID != "alice" || store.history[0].CompletedAt == nil {
		t.Fatalf("history=%+v", store.history)
	}
	if _, err = service.ChooseAlternate(ctx, "alice", id, 1); !errors.Is(err, ErrInvalidAlternate) {
		t.Fatalf("bad alternate: %v", err)
	}
	if _, err = service.EditExample(ctx, "bob", id, "stolen"); !errors.Is(err, ErrNoExample) {
		t.Fatalf("bob edit: %v", err)
	}
}
