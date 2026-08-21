package analyzer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"google.golang.org/protobuf/proto"
)

const pythonBridgeProgram = `import base64,json,sys
from mouseion_nlp import Producer, SourceDocument
r=json.load(sys.stdin)
d=r["document"]
c=Producer().analyze(r["text"],r["language"],SourceDocument(id=d["id"],source_identifier=d["source_identifier"],title=d["title"]))
sys.stdout.write(base64.b64encode(c.SerializeToString()).decode("ascii"))`

// PythonStanzaAnalyzer invokes the Stanza producer once for a whole document.
// stdin is JSON and stdout is a base64-encoded NormalizedCorpus protobuf.
type PythonStanzaAnalyzer struct {
	PythonPath    string
	PythonPathEnv string
	Dir           string
}

func (a PythonStanzaAnalyzer) Analyze(ctx context.Context, request AnalyzeRequest) (Result, error) {
	python := a.PythonPath
	if python == "" {
		python = os.Getenv("MOUSEION_PYTHON")
	}
	if python == "" {
		python = "python"
	}
	payload, err := json.Marshal(map[string]any{
		"language": request.Language, "text": request.Document.Text,
		"document": map[string]string{"id": request.Document.ID, "source_identifier": request.Document.SourceIdentifier, "title": request.Document.Title},
	})
	if err != nil {
		return Result{}, fmt.Errorf("encode Python analyzer request: %w", err)
	}
	cmd := exec.CommandContext(ctx, python, "-c", pythonBridgeProgram)
	cmd.Dir = a.Dir
	pythonPath := a.PythonPathEnv
	if pythonPath == "" {
		pythonPath = "nlp/src:gen/python"
	}
	cmd.Env = append(os.Environ(), "PYTHONPATH="+pythonPath)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("run Python Stanza producer: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdout.String()))
	if err != nil {
		return Result{}, fmt.Errorf("decode Python Stanza output: %w", err)
	}
	corpus := new(mouseionv1.NormalizedCorpus)
	if err := proto.Unmarshal(raw, corpus); err != nil {
		return Result{}, fmt.Errorf("unmarshal Python Stanza output: %w", err)
	}
	result, err := FromProto(corpus)
	if err != nil {
		return Result{}, fmt.Errorf("convert Python Stanza output: %w", err)
	}
	return result, nil
}
