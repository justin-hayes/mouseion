package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonStanzaAnalyzerConsumesBase64Protobuf(t *testing.T) {
	fixture, err := os.ReadFile("testdata/stanza_python_corpus.pb.b64")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "python")
	script := "#!/bin/sh\ncat >/dev/null\nprintf '%s' '" + strings.TrimSpace(string(fixture)) + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := (PythonStanzaAnalyzer{PythonPath: path}).Analyze(context.Background(), AnalyzeRequest{Language: "de", Document: SourceDocument{ID: "document-1", Text: "Goethe"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Language != "de" || len(result.Sentences) == 0 || result.Sentences[0].Tokens[0].Surface != "Goethe" {
		t.Fatalf("result = %+v", result)
	}
}

func TestPythonStanzaAnalyzerReportsProducerFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "python")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho producer-broke >&2\nexit 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := (PythonStanzaAnalyzer{PythonPath: path}).Analyze(context.Background(), AnalyzeRequest{Language: "de"})
	if err == nil || !strings.Contains(err.Error(), "producer-broke") {
		t.Fatalf("expected stderr in error, got %v", err)
	}
}
