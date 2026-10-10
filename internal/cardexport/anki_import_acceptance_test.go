//go:build anki

package cardexport

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRealAnkiImportAcceptance exercises the current Anki collection importer.
// Run with `go test -tags=anki ./internal/cardexport -run TestRealAnkiImportAcceptance`;
// the selected Python must have the official `anki` package installed.
func TestRealAnkiImportAcceptance(t *testing.T) {
	python := os.Getenv("MOUSEION_ANKI_PYTHON")
	if python == "" {
		python = "python3"
	}
	base := RenderInput{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Das Haus ist groß.", TargetWord: "Haus", SourceDocument: "Book A", Morphology: `{"Gender":"Neut","Number":"Sing"}`}
	book, err := makeNote("owner", base)
	if err != nil {
		t.Fatal(err)
	}
	apkg, err := renderAPKG(t.Context(), DeckName("de", "Book A"), []Note{book}, "")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "book.apkg"), apkg, 0o600); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("testdata/anki_import_acceptance.py")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), python, script, work, book.Key[:20])
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		t.Fatalf("real Anki import acceptance failed: %v\n%s", runErr, output)
	} else {
		t.Log(string(output))
	}
}
