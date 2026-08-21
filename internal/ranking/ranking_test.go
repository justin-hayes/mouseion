package ranking

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

type fakeFrequency struct {
	values map[string]float64
	calls  []string
}

func (f *fakeFrequency) FrequencyPercentile(_ context.Context, language, lemma, upos string) (float64, bool, error) {
	key := language + "/" + lemma + "/" + upos
	f.calls = append(f.calls, key)
	value, ok := f.values[key]
	return value, ok, nil
}

type memoryStore struct {
	saved map[string]domain.RankingComponents
}

func (m *memoryStore) PutRankingComponents(_ context.Context, owner, corpus, language, lemma, upos string, c domain.RankingComponents) error {
	if m.saved == nil {
		m.saved = map[string]domain.RankingComponents{}
	}
	m.saved[owner+"/"+corpus+"/"+language+"/"+lemma+"/"+upos] = c
	return nil
}
func candidate(language, lemma, upos string, count int, priority bool, sources ...string) selection.Candidate {
	refs := make([]selection.SentenceReference, len(sources))
	for i, source := range sources {
		refs[i].Location = analyzer.SourceLocation{SourceDocumentID: source}
	}
	return selection.Candidate{Identity: selection.Identity{Language: language, CanonicalLemma: lemma, UPOS: upos}, OccurrenceCount: count, Provenance: selection.Provenance{Priority: priority}, SentenceReferences: refs}
}

func TestGoldenRanking(t *testing.T) {
	candidates := []selection.Candidate{candidate("de", "global", "NOUN", 1, false, "a"), candidate("de", "corpus", "NOUN", 10, false, "a"), candidate("de", "priority", "ADJ", 2, true, "a", "b"), candidate("de", "missing", "VERB", 5, false, "a"), candidate("it", "globale", "NOUN", 1, false, "a")}
	freq := &fakeFrequency{values: map[string]float64{"de/global/NOUN": .99, "de/corpus/NOUN": .2, "de/priority/ADJ": .96, "it/globale/NOUN": .98}}
	got, err := NewService(&memoryStore{}, freq).Rank(context.Background(), "alice", candidates, DefaultConfig("book"))
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for _, ranked := range got {
		c := ranked.Components
		fmt.Fprintf(&output, "%s/%s/%s global=%.3f corpus=%.3f priority=%t cross=%d score=%.3f\n", ranked.Candidate.Identity.Language, ranked.Candidate.Identity.CanonicalLemma, ranked.Candidate.Identity.UPOS, c.GlobalPercentile, c.CorpusPercentile, c.Priority, c.CrossText, c.Score)
	}
	want, err := os.ReadFile("testdata/ranking.golden")
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != string(want) {
		t.Fatalf("golden mismatch\n--- got ---\n%s--- want ---\n%s", output.String(), want)
	}
	if got[1].Candidate.Identity.CanonicalLemma != "global" || got[3].Candidate.Identity.CanonicalLemma != "corpus" {
		t.Fatalf("global-first ordering: %+v", got)
	}
	if len(freq.calls) != 5 || freq.calls[4] != "it/globale/NOUN" {
		t.Fatalf("language-scoped calls: %v", freq.calls)
	}
}

func TestTieBreakerCrossTextCapAndConfigOverride(t *testing.T) {
	cfg := DefaultConfig("book")
	cfg.GlobalWeight = 0
	cfg.CorpusWeight = 0
	cfg.PriorityWeight = 0
	cfg.CrossTextWeight = .2
	cfg.CrossTextCap = 2
	cfg.GlobalFrequencyCutoff = 0
	got, err := NewService(&memoryStore{}, &fakeFrequency{values: map[string]float64{}}).Rank(context.Background(), "alice", []selection.Candidate{candidate("de", "zeta", "NOUN", 1, false, "a", "b", "c"), candidate("de", "alpha", "NOUN", 1, false, "a", "b")}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Candidate.Identity.CanonicalLemma != "alpha" || got[0].Components.CrossText != 2 || got[0].Components.Score != .2 {
		t.Fatalf("got=%+v", got)
	}
}

func TestInvalidConfig(t *testing.T) {
	_, err := NewService(&memoryStore{}, &fakeFrequency{}).Rank(context.Background(), "", nil, Config{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
}
