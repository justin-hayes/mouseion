package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunConvertsTwoTables(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.tsv")
	second := filepath.Join(dir, "b.tsv")
	require.NoError(t, os.WriteFile(first, []byte("item\twork\tsent_id\tanchor\tconstruction\tsample\tdetector_subtype\tagent_true\tagent_subtype\tjustin_true\tjustin_subtype\n"), 0o600))
	require.NoError(t, os.WriteFile(second, []byte("item\twork\tsent_id\tanchor\tconstruction\tsample\tdetector_subtype\tprior_item_2607\tagent_true\tagent_subtype\tjustin_true\tjustin_subtype\n"), 0o600))

	var out, summary bytes.Buffer
	require.NoError(t, run([]string{first, second}, &out, &summary))
	assert.Contains(t, out.String(), "work\tsent_id")
	assert.Contains(t, summary.String(), "0 kept")
	require.Error(t, run([]string{first}, &out, &summary))
	require.Error(t, run([]string{first, filepath.Join(dir, "missing")}, &out, &summary))
	require.Error(t, run([]string{filepath.Join(dir, "missing"), second}, &out, &summary))
}
