//go:build integration

package analyzer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGRPCAnalyzerRealPythonServer(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-m", "mouseion_nlp.server")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(repo, "nlp", "src")+":"+filepath.Join(repo, "gen", "python"), "MOUSEION_NLP_ADDR=127.0.0.1:0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		body, _ := io.ReadAll(stdout)
		t.Fatalf("start Python server: %v: %s", err, body)
	}
	const prefix = "mouseion NLP gRPC server listening on "
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("unexpected server output: %q", line)
	}
	analyzer, err := NewGRPCAnalyzer(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = analyzer.Close() })
	result, err := analyzer.Analyze(ctx, AnalyzeRequest{Language: "de", Document: SourceDocument{ID: "real-grpc", Text: "Das Haus ist groß."}})
	if err != nil {
		t.Fatal(fmt.Errorf("real gRPC analysis: %w", err))
	}
	if result.Language != "de" || len(result.Sentences) == 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
