//go:build integration

package analyzer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGRPCAnalyzerRealPythonServer(t *testing.T) {
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	python := filepath.Join(repo, ".venv", "bin", "python")
	modelDir := filepath.Join(os.Getenv("HOME"), "stanza_resources", "de")
	if _, err := os.Stat(python); err != nil {
		t.Skip("Python virtualenv unavailable")
	}
	if _, err := os.Stat(modelDir); err != nil { //nolint:gosec // modelDir is assembled from the test repository HOME fixture.
		t.Skip("Stanza German model unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-m", "mouseion_nlp.server") //nolint:gosec // the executable is the repository's test virtualenv.
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(repo, "nlp", "src")+":"+filepath.Join(repo, "gen", "python"), "MOUSEION_NLP_ADDR=127.0.0.1:0")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = cmd.Stdout
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		killErr := cmd.Process.Kill()
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			t.Errorf("stop Python server: %v", killErr)
		}
		if waitErr := cmd.Wait(); errors.Is(killErr, os.ErrProcessDone) && waitErr != nil {
			t.Errorf("wait for Python server: %v", waitErr)
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		body, readErr := io.ReadAll(stdout)
		require.NoError(t, readErr)
		require.FailNow(t, fmt.Sprintf("start Python server: %v: %s", err, body))
	}
	const prefix = "mouseion NLP gRPC server listening on "
	require.True(t, strings.HasPrefix(line, prefix), "unexpected server output: %q", line)
	analyzer, err := NewGRPCAnalyzer(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := analyzer.Close(); err != nil {
			t.Errorf("close gRPC analyzer: %v", err)
		}
	})
	result, err := analyzer.Analyze(ctx, AnalyzeRequest{Language: "de", Document: SourceDocument{ID: "real-grpc", Text: "Das Haus ist groß."}})
	require.NoError(t, err, "real gRPC analysis: %v", err)
	assert.Equal(t, "de", result.Language)
	assert.NotEmpty(t, result.Sentences)
	capabilities, err := analyzer.GetCapabilities(ctx)
	require.NoError(t, err, "real gRPC capabilities")
	require.Len(t, capabilities.Languages, 1)
	assert.Equal(t, "de", capabilities.Languages[0].Language)
	assert.True(t, capabilities.Languages[0].Ready)
}
