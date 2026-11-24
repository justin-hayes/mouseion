package ancientgreek

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinnedDirEnv names the directory that holds local copies of the pinned OGA
// files. The files never enter the repository, so the pinned-file tests are
// opt-in: they skip only when it is unset.
const pinnedDirEnv = "MOUSEION_OGA_SRC_DIR"

const pinnedDigestsPath = "testdata/pinned-span-digests.json"

// pinnedDefinition is the Reading-unit definition the adapter cuts a compiled
// manifest definition with.
func pinnedDefinition(definition classical.Definition) domain.ReadingUnitDefinition {
	return domain.ReadingUnitDefinition{
		Key: definition.Key, Language: definition.Language, WorkTitle: definition.WorkTitle, WorkAuthor: definition.WorkAuthor,
		EditionLabel: definition.EditionLabel, CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
		SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size, SourceFileSHA256: definition.Source.SHA256,
		InterpretationVersion: definition.InterpretationVersion,
	}
}

// readLocalPinnedFile reads a definition's pinned file from the opt-in directory and
// checks its digest. A file that is missing or is not the pinned file fails.
func readLocalPinnedFile(t *testing.T, dir string, definition classical.Definition) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(filepath.Clean(dir), definition.Source.FileName)) //nolint:gosec // the directory is the developer's own opt-in input and file names come from the reviewed manifest.
	require.NoError(t, err, "%s is missing from %s", definition.Source.FileName, dir)
	sum := sha256.Sum256(data)
	require.Equal(t, definition.Source.SHA256, hex.EncodeToString(sum[:]), "the local file is not the pinned file")
	return data
}

// TestPinnedFilesKeepEveryPrintedSpanAndLemma shows, for all compiled
// definitions, that cutting the pinned file gives exactly the sentence and token
// ordinals, offsets, surfaces, raw and canonical lemmas the previous
// interpretation version gave (A2). The expected digests were captured by
// running this test against commit 3391cc7b (oga-agdt-upos/5) with
// MOUSEION_UPDATE_SPAN_BASELINE=1.
func TestPinnedFilesKeepEveryPrintedSpanAndLemma(t *testing.T) {
	dir := os.Getenv(pinnedDirEnv)
	if dir == "" {
		t.Skip(pinnedDirEnv + " is not set")
	}
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	require.NoError(t, err)
	update := os.Getenv("MOUSEION_UPDATE_SPAN_BASELINE") == "1"
	expected := map[string]string{}
	if !update {
		raw, readErr := os.ReadFile(pinnedDigestsPath)
		require.NoError(t, readErr)
		require.NoError(t, json.Unmarshal(raw, &expected))
		require.Len(t, expected, len(definitions))
	}
	parsed := map[string]*ParsedWork{}
	actual := map[string]string{}
	for _, definition := range definitions {
		t.Run(definition.Key, func(t *testing.T) {
			work, ok := parsed[definition.Source.FileName]
			if !ok {
				var parseErr error
				work, parseErr = ParseOgaSource(readLocalPinnedFile(t, dir, definition))
				require.NoError(t, parseErr)
				parsed[definition.Source.FileName] = work
			}
			unit, err := work.CutUnit(pinnedDefinition(definition))
			require.NoError(t, err)
			encoded, err := json.Marshal(baselineOfUnit(unit))
			require.NoError(t, err)
			sum := sha256.Sum256(encoded)
			actual[definition.Key] = hex.EncodeToString(sum[:])
			if !update {
				assert.Equal(t, expected[definition.Key], actual[definition.Key])
			}
			t.Logf("%s: %d sentences, %d tokens, span digest %s", definition.Key, unit.Report.Sentences, unit.Report.Tokens, actual[definition.Key])
		})
	}
	if update {
		encoded, err := json.MarshalIndent(actual, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(pinnedDigestsPath, append(encoded, '\n'), 0o600))
	}
}
