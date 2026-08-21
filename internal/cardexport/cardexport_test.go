package cardexport

import (
	"context"
	"encoding/csv"
	"os"
	"strings"
	"testing"
)

type memoryStore struct {
	entries   []Entry
	generated []Note
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
