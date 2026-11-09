package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunGeneratesAndVerifiesTheManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.jsonl")
	var output bytes.Buffer
	err := run([]string{
		"generate",
		"-layers", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "layers.json"),
		"-overrides", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "overrides.jsonl"),
		"-manifest", manifest,
		"-upstream", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "upstream"),
	}, &output)
	require.NoError(t, err)
	assert.Contains(t, output.String(), "1 definitions")
	generated, err := os.ReadFile(manifest) //nolint:gosec // test temp path.
	require.NoError(t, err)

	output.Reset()
	err = run([]string{
		"verify",
		"-layers", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "layers.json"),
		"-overrides", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "overrides.jsonl"),
		"-manifest", manifest,
	}, &output)
	require.NoError(t, err)
	assert.Contains(t, output.String(), "verified")

	require.NoError(t, os.WriteFile(manifest, append(generated, '\n'), 0o600)) //nolint:gosec // test temp path.
	err = run([]string{
		"verify",
		"-layers", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "layers.json"),
		"-overrides", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "overrides.jsonl"),
		"-manifest", manifest,
	}, &bytes.Buffer{})
	require.ErrorContains(t, err, "is stale")
}

func TestRunRefusesAnUnknownMode(t *testing.T) {
	require.ErrorContains(t, run([]string{"publish"}, &bytes.Buffer{}), "usage")
	require.ErrorContains(t, run(nil, &bytes.Buffer{}), "usage")
	require.Error(t, run([]string{"verify", "-unknown"}, &bytes.Buffer{}))
}
