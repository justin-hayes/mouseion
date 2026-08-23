package cardexport

import (
	"context"
	"encoding/csv"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type memoryStore struct {
	entries   []Entry
	generated []Note
	bookID    string
}

func (m *memoryStore) ListAcceptedCurated(_ context.Context, owner string) ([]Entry, error) {
	var out []Entry
	for _, e := range m.entries {
		if e.OwnerID == owner {
			out = append(out, e)
		}
	}
	return out, nil
}
func (m *memoryStore) ListAcceptedCuratedForBook(_ context.Context, owner, bookID string) ([]Entry, error) {
	if bookID != m.bookID {
		return nil, nil
	}
	return m.ListAcceptedCurated(context.Background(), owner)
}
func (m *memoryStore) RecordGenerated(_ context.Context, owner, _ string, e Entry, n Note) error {
	if owner != e.OwnerID {
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

func TestGoldenExport(t *testing.T) {
	store := &memoryStore{entries: []Entry{{OwnerID: "user-123", Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Das Haus ist groß.", Translation: "house", TargetWord: "Haus", Morphology: `{"Gender":"Neut"}`, SourceDocument: "Der Zauberberg", Notes: "chapter\t1\nimportant"}}}
	artifact, err := NewService(store).Export(context.Background(), "user-123", "German")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/anki_export.golden.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.TSV != string(want) {
		t.Fatalf("golden mismatch\nwant: %q\n got: %q", string(want), artifact.TSV)
	}
	if artifact.Count != 1 || len(store.generated) != 1 || !strings.Contains(artifact.NoteType, "Key, Text, Back Extra, Tags") {
		t.Fatalf("artifact=%+v generated=%d", artifact, len(store.generated))
	}
}

func TestConfiguredExportFiltersKnownAndRanks(t *testing.T) {
	store := &memoryStore{bookID: "book-1", entries: []Entry{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "bekannt", UPOS: "ADJ", Sentence: "Das ist bekannt.", TargetWord: "bekannt", Ranking: domain.RankingComponents{GlobalPercentile: .99, CorpusPercentile: .1, CrossText: 1}},
		{OwnerID: "alice", Language: "de", CanonicalLemma: "häufig", UPOS: "ADJ", Sentence: "Das ist häufig.", TargetWord: "häufig", Ranking: domain.RankingComponents{GlobalPercentile: .2, CorpusPercentile: 1, CrossText: 1}},
		{OwnerID: "alice", Language: "de", CanonicalLemma: "weltweit", UPOS: "ADV", Sentence: "Das gilt weltweit.", TargetWord: "weltweit", Ranking: domain.RankingComponents{GlobalPercentile: .9, CorpusPercentile: .3, CrossText: 1}},
		{OwnerID: "bob", Language: "de", CanonicalLemma: "privat", UPOS: "ADJ", Sentence: "Das ist privat.", TargetWord: "privat"},
	}}
	artifact, err := NewService(store).ExportConfigured(context.Background(), "alice", "German", ExportConfig{BookID: "book-1", FilterKnown: true, Ranking: RankingBook, KnownWords: []KnownWord{{Language: "de", CanonicalLemma: "bekannt"}}})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 2 || strings.Contains(artifact.TSV, "bekannt") || strings.Contains(artifact.TSV, "privat") {
		t.Fatalf("unexpected filtered export: count=%d TSV=%q", artifact.Count, artifact.TSV)
	}
	if strings.Index(artifact.TSV, "häufig") > strings.Index(artifact.TSV, "weltweit") {
		t.Fatalf("book ranking was not applied: %q", artifact.TSV)
	}
}

func TestConfiguredExportDefaultsToFirstEncounter(t *testing.T) {
	store := &memoryStore{bookID: "book-1", entries: []Entry{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "anfang", UPOS: "NOUN", Sentence: "Der Anfang ist hier.", TargetWord: "Anfang", FirstEncounter: 5},
		{OwnerID: "alice", Language: "de", CanonicalLemma: "zuerst", UPOS: "ADV", Sentence: "Zuerst kommt dieses Wort.", TargetWord: "Zuerst", FirstEncounter: 1},
	}}
	artifact, err := NewService(store).ExportConfigured(context.Background(), "alice", "German", ExportConfig{BookID: "book-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(artifact.TSV, "Zuerst") > strings.Index(artifact.TSV, "Anfang") {
		t.Fatalf("first-encounter ordering was not applied: %q", artifact.TSV)
	}
}

func TestLegacyExportKeepsAlphabeticalOrder(t *testing.T) {
	store := &memoryStore{entries: []Entry{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "zuerst", UPOS: "ADV", Sentence: "Zuerst kommt dieses Wort.", TargetWord: "Zuerst", FirstEncounter: 1},
		{OwnerID: "alice", Language: "de", CanonicalLemma: "anfang", UPOS: "NOUN", Sentence: "Der Anfang ist hier.", TargetWord: "Anfang", FirstEncounter: 5},
	}}
	artifact, err := NewService(store).Export(context.Background(), "alice", "German")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(artifact.TSV, "Anfang") > strings.Index(artifact.TSV, "Zuerst") {
		t.Fatalf("legacy alphabetical ordering changed: %q", artifact.TSV)
	}
}

func TestConfiguredExportRejectsInvalidInput(t *testing.T) {
	service := NewService(&memoryStore{})
	for _, cfg := range []ExportConfig{{}, {BookID: "book", Ranking: "unknown"}} {
		if _, err := service.ExportConfigured(context.Background(), "alice", "German", cfg); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("config=%+v err=%v", cfg, err)
		}
	}
}
