//go:build anki

package cardexport

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// TestRealAnkiImportAcceptance exercises the current Anki collection importer.
// Run with `go test -tags=anki ./internal/cardexport -run TestRealAnkiImportAcceptance`;
// the selected Python must have the official `anki` package installed.
func TestRealAnkiImportAcceptance(t *testing.T) {
	python := os.Getenv("MOUSEION_ANKI_PYTHON")
	if python == "" {
		python = "python3"
	}
	snapshot := ManifestSnapshot{
		SchemaVersion: ManifestSchemaVersion, Owner: "owner", DeckName: "Book A", Filename: DownloadFilename("Book A"),
		Items: []ManifestItem{{
			Ordinal: 0, Disposition: ManifestAccepted,
			Entry: Entry{
				Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN",
				Sentence: "Das Haus ist groß.\nEs steht am Fluss.", TargetWord: "Haus",
				SourceDocument: "Book A", Morphology: `{"Gender":"Neut","Number":"Sing"}`,
				Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster"},
			},
			Quality: SentenceQuality{Accepted: true, Score: 90, Reasons: []string{"target present"}},
		}},
	}
	deck, err := NewPresentation(nil).Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	artifact, _, err := NewPresentation(nil).Finalize(t.Context(), deck, nil, RunFacts{})
	if err != nil {
		t.Fatal(err)
	}
	book := artifact.Generated[0].Note
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
