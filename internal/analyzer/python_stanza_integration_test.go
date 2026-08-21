//go:build integration

package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPythonStanzaAnalyzerRealProducer(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(repo, ".venv", "bin", "python")
	modelDir := filepath.Join(os.Getenv("HOME"), "stanza_resources", "de")
	if _, err := os.Stat(python); err != nil {
		t.Skip("Python virtualenv unavailable")
	}
	if _, err := os.Stat(modelDir); err != nil {
		t.Skip("Stanza German model unavailable")
	}
	result, err := (PythonStanzaAnalyzer{PythonPath: python, PythonPathEnv: filepath.Join(repo, "nlp", "src") + ":" + filepath.Join(repo, "gen", "python"), Dir: repo}).Analyze(context.Background(), AnalyzeRequest{Language: "de", Document: SourceDocument{ID: "real-bridge", Text: "Das Haus ist groß."}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Language != "de" || len(result.Sentences) == 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
