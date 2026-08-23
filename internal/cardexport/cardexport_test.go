package cardexport

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type memoryStore struct {
	entries    []Entry
	candidates []domain.SelectionCandidate
	known      []domain.KnownVocabulary
	generated  []Note
	bookID     string
}

func (m *memoryStore) ListSelectionCandidatesForBook(_ context.Context, _ string, bookID string) ([]domain.SelectionCandidate, error) {
	if bookID != m.bookID {
		return nil, nil
	}
	return append([]domain.SelectionCandidate(nil), m.candidates...), nil
}
func (m *memoryStore) ListKnownVocabulary(_ context.Context, _ string, language string) ([]domain.KnownVocabulary, error) {
	var out []domain.KnownVocabulary
	for _, word := range m.known {
		if word.Language == language {
			out = append(out, word)
		}
	}
	return out, nil
}
func (m *memoryStore) GetCoverageEntryForBook(_ context.Context, owner, _ string, candidate domain.SelectionCandidate) (Entry, error) {
	for _, entry := range m.entries {
		if entry.OwnerID == owner && entry.Language == candidate.Language && entry.CanonicalLemma == candidate.CanonicalLemma && entry.UPOS == candidate.UPOS {
			return entry, nil
		}
	}
	return Entry{}, errors.New("missing entry")
}

func (m *memoryStore) RecordGeneratedForBook(_ context.Context, owner, bookID, _ string, e Entry, n Note) error {
	if owner != e.OwnerID {
		return ErrInvalidInput
	}
	if bookID != m.bookID {
		return ErrInvalidInput
	}
	m.generated = append(m.generated, n)
	return nil
}

func TestDedupKeyStableAndOwnerScoped(t *testing.T) {
	a := DedupKey("de", "Haus", "NOUN", "alice")
	if a != DedupKey("de", "Haus", "NOUN", "alice") || len(a) != 64 {
		t.Fatalf("unstable key %q", a)
	}
	for _, b := range []string{DedupKey("de", "Haus", "NOUN", "bob"), DedupKey("fr", "Haus", "NOUN", "alice"), DedupKey("de", "haus", "NOUN", "alice"), DedupKey("de", "Haus", "VERB", "alice")} {
		if a == b {
			t.Fatal("distinct identity produced same key")
		}
	}
}

func TestClozeWithAndWithoutHint(t *testing.T) {
	got, err := Cloze("Das Haus ist groß.", "Haus", "house")
	if err != nil || got != "Das {{c1::Haus::house}} ist groß." {
		t.Fatalf("got=%q err=%v", got, err)
	}
	got, err = Cloze("Das Haus ist groß.", "haus", "")
	if err != nil || got != "Das {{c1::Haus}} ist groß." {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err = Cloze("Kein Treffer.", "Haus", ""); err == nil {
		t.Fatal("missing target accepted")
	}
}

func TestRenderTSVEscapesAndOrdersFields(t *testing.T) {
	n := Note{Key: "key", Text: "Grüße\t{{c1::Welt}}", BackExtra: "line one\nline two", Tags: []string{"mouseion", "de", "My_Book"}}
	got, err := RenderTSV([]Note{n})
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(got))
	r.Comma = '\t'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 4 || rows[0][0] != "key" || rows[0][1] != n.Text || rows[0][2] != n.BackExtra || rows[0][3] != "mouseion de My_Book" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestCoverageCandidatesExcludesKnownAndStopsAt97Percent(t *testing.T) {
	store := &memoryStore{known: []domain.KnownVocabulary{{Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}}}
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 100},
		{Language: "de", CanonicalLemma: "one", UPOS: "NOUN", OccurrenceCount: 96},
		{Language: "de", CanonicalLemma: "singleton-a", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-b", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-c", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-d", UPOS: "NOUN", OccurrenceCount: 1},
	}
	got, err := NewService(store).coverageCandidates(context.Background(), "alice", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].CanonicalLemma != "one" || got[1].CanonicalLemma != "singleton-a" {
		t.Fatalf("coverage candidates = %#v", got)
	}
}
