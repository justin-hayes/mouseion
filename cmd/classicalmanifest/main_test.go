package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
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

func TestRunVerifyFailsOnADriftedOpening(t *testing.T) {
	upstream := filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "upstream")
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	args := []string{
		"-layers", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "layers.json"),
		"-overrides", filepath.Join("..", "..", "internal", "classicalmanifest", "testdata", "overrides.jsonl"),
		"-manifest", manifest,
		"-upstream", upstream,
	}
	require.NoError(t, run(append([]string{"generate"}, args...), &bytes.Buffer{}))

	generated, err := os.ReadFile(manifest) //nolint:gosec // test temp path.
	require.NoError(t, err)
	definitions, err := classical.ParseDefinitions(generated)
	require.NoError(t, err)
	definitions[0].Opening = "a drifted Opening"
	tampered, err := classical.RenderDefinitions(definitions)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifest, tampered, 0o600)) //nolint:gosec // test temp path.

	verifyArgs := append([]string{"verify", "-require-upstream"}, args...)
	require.ErrorContains(t, run(verifyArgs, &bytes.Buffer{}), "is stale", "a committed Opening that differs from the derived one fails verification")
}

func TestRunRefusesAnUnknownMode(t *testing.T) {
	require.ErrorContains(t, run([]string{"publish"}, &bytes.Buffer{}), "usage")
	require.ErrorContains(t, run(nil, &bytes.Buffer{}), "usage")
	require.Error(t, run([]string{"verify", "-unknown"}, &bytes.Buffer{}))
}

func TestRunGenerateRefusesADictionaryIndexItCannotOpen(t *testing.T) {
	dir := t.TempDir()
	testdata := filepath.Join("..", "..", "internal", "classicalmanifest", "testdata")
	err := run([]string{
		"generate",
		"-layers", filepath.Join(testdata, "layers.json"),
		"-overrides", filepath.Join(testdata, "overrides.jsonl"),
		"-manifest", filepath.Join(dir, "manifest.jsonl"),
		"-upstream", filepath.Join(testdata, "upstream"),
		"-ls-index", filepath.Join(dir, "missing.sqlite"),
	}, &bytes.Buffer{})
	require.ErrorContains(t, err, "open dictionary index")
}
