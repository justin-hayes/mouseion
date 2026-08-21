package sentences

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

type memoryStore struct{ examples []domain.ExampleSentence }

func (m *memoryStore) ReplaceSelectedSentences(_ context.Context, _, _, _, _, _ string, examples []domain.ExampleSentence) error {
	m.examples = examples
	return nil
}

func ref(index int, text string) selection.SentenceReference {
	return selection.SentenceReference{SentenceIndex: index, Text: text, Location: analyzer.SourceLocation{SourceDocumentID: "book", Chapter: "1", StartOffset: uint64(index * 100), EndOffset: uint64(index*100 + len([]rune(text)))}}
}
func candidate(refs ...selection.SentenceReference) selection.Candidate {
	return selection.Candidate{Identity: selection.Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}, ObservedForms: []string{"Haus", "Häuser"}, SentenceReferences: refs}
}

func TestSelectScoresExcludesAndPersistsRankedResults(t *testing.T) {
	store := &memoryStore{}
	svc := NewService(store)
	best := ref(3, "Vor dem kleinen Haus spielt heute ein fröhliches Kind.")
	repeated := ref(1, "Das Haus steht neben dem Haus am ruhigen See.")
	quoted := ref(2, `Er sagte: “Das Haus steht dort am ruhigen See.”`)
	duplicate := ref(4, best.Text)
	missing := ref(5, "Der Garten liegt heute still unter alten Bäumen.")
	malformed := ref(6, " Haus steht auf dem Hügel.")
	tooLong := ref(7, "Das Haus "+strings.Repeat("liegt sehr weit entfernt ", 14)+".")
	result, err := svc.Select(context.Background(), "alice", "corpus", candidate(repeated, quoted, best, duplicate, missing, malformed, tooLong), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if result.Chosen == nil || result.Chosen.Text != best.Text {
		t.Fatalf("chosen=%+v", result.Chosen)
	}
	if len(result.Alternatives) != 2 {
		t.Fatalf("alternatives=%d", len(result.Alternatives))
	}
	if len(store.examples) != 3 || !store.examples[0].Chosen || store.examples[1].Chosen {
		t.Fatalf("persisted=%+v", store.examples)
	}
	if store.examples[0].SelectionScore <= store.examples[1].SelectionScore || len(store.examples[0].SelectionReasons) == 0 {
		t.Fatalf("scores/reasons=%+v", store.examples)
	}
}

func TestSelectConfigAndDeterministicTieBreak(t *testing.T) {
	store := &memoryStore{}
	svc := NewService(store)
	second := ref(2, "Dieses Haus steht seit Jahren ruhig am See.")
	first := ref(1, "Unser Haus steht seit Jahren ruhig am See.")
	cfg := DefaultConfig()
	cfg.MaxAlternatives = 0
	one, err := svc.Select(context.Background(), "alice", "corpus", candidate(second, first), cfg)
	if err != nil {
		t.Fatal(err)
	}
	two, err := svc.Select(context.Background(), "alice", "corpus", candidate(first, second), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if one.Chosen == nil || two.Chosen == nil || one.Chosen.Text != first.Text || two.Chosen.Text != first.Text || len(one.Alternatives) != 0 {
		t.Fatalf("one=%+v two=%+v", one, two)
	}
	cfg.MaxWords = 2
	if _, err = svc.Select(context.Background(), "alice", "corpus", candidate(first), cfg); err != ErrInvalidInput {
		t.Fatalf("invalid config err=%v", err)
	}
}

func TestGoldenSentenceCases(t *testing.T) {
	cases := []struct {
		name string
		refs []selection.SentenceReference
	}{
		{"repeated_forms", []selection.SentenceReference{ref(0, "Das Haus steht neben dem Haus am ruhigen See.")}},
		{"long_sentence", []selection.SentenceReference{ref(0, "Dieses außergewöhnlich alte Haus steht seit vielen Jahrzehnten hoch über dem ruhigen Tal und bietet seinen Bewohnern trotz der langen kalten Winter einen erstaunlich schönen Blick auf die fernen Berge und grünen Wälder der gesamten Region.")}},
		{"ties", []selection.SentenceReference{ref(2, "Dieses Haus steht seit Jahren ruhig am See."), ref(1, "Unser Haus steht seit Jahren ruhig am See.")}},
		{"missing_candidates", []selection.SentenceReference{ref(0, "Der Garten liegt heute still unter alten Bäumen.")}},
	}
	type summary struct {
		Name, Chosen string
		Alternatives int
		Score        int
	}
	got := make([]summary, 0, len(cases))
	for _, tc := range cases {
		result, err := NewService(&memoryStore{}).Select(context.Background(), "alice", "corpus", candidate(tc.refs...), DefaultConfig())
		if err != nil {
			t.Fatal(err)
		}
		item := summary{Name: tc.name, Alternatives: len(result.Alternatives)}
		if result.Chosen != nil {
			item.Chosen, item.Score = result.Chosen.Text, result.Chosen.Score
		}
		got = append(got, item)
	}
	encoded, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	want, err := os.ReadFile("testdata/sentences.golden")
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(want) {
		t.Fatalf("golden mismatch\ngot:\n%s\nwant:\n%s", encoded, want)
	}
}
