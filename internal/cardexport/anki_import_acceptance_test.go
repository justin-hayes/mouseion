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
	customInput := base
	customInput.SourceDocument = "Book B"
	custom, err := makeNote("owner", customInput)
	if err != nil {
		t.Fatal(err)
	}
	changedInput := customInput
	changedInput.Sentence = "Das alte Haus ist groß."
	changed, err := makeNote("owner", changedInput)
	if err != nil {
		t.Fatal(err)
	}
	deckID := "018f7e7a-9d20-7c8e-b131-3a5310e10dd7"
	stableID := CustomDeckAnkiID(deckID)
	customAnkiDeckName := DeckName("de", CustomDeckName(deckID))
	packages := []struct {
		filename, deckName, description string
		deckID                          int64
		note                            Note
	}{
		{"book.apkg", DeckName("de", "Book A"), "", 0, book},
		{"custom.apkg", customAnkiDeckName, customDeckAnkiDescription("Study"), stableID, custom},
		{"renamed-prepare-again.apkg", customAnkiDeckName, customDeckAnkiDescription("Renamed"), stableID, changed},
	}
	// Use one shared temp directory for the package sequence and real importer.
	work := t.TempDir()
	for _, item := range packages {
		apkg, renderErr := renderAPKGWithDeckID(t.Context(), item.deckName, item.deckID, []Note{item.note}, item.description)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		if writeErr := os.WriteFile(filepath.Join(work, item.filename), apkg, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	script, err := filepath.Abs("testdata/anki_import_acceptance.py")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), python, script, work, book.Key[:20], custom.Identity, changed.Identity, customAnkiDeckName)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		t.Fatalf("real Anki import acceptance failed: %v\n%s", runErr, output)
	} else {
		t.Log(string(output))
	}
}
