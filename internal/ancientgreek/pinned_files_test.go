package ancientgreek

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinnedDirEnv names the directory that holds local copies of the pinned OGA
// files. The files never enter the repository, so the test is opt-in.
const pinnedDirEnv = "MOUSEION_OGA_SRC_DIR"

// TestPinnedFilesKeepPrintedSpansAndCarryCleanNativeSyntax cuts every compiled
// definition from its pinned file and shows, for each, that the native-syntax
// interpretation prints exactly the tokens, lemmas and offsets of the file's
// printed words (A2) and that the stored syntax is well formed (A1). It is
// skipped when MOUSEION_OGA_SRC_DIR is unset, and a definition whose file is not
// in the directory is skipped by name.
func TestPinnedFilesKeepPrintedSpansAndCarryCleanNativeSyntax(t *testing.T) {
	dir := os.Getenv(pinnedDirEnv)
	if dir == "" {
		t.Skip(pinnedDirEnv + " is not set")
	}
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	require.NoError(t, err)
	parsed := map[string]*ParsedWork{}
	documents := map[string]*sourceDocument{}
	for _, definition := range definitions {
		t.Run(definition.Key, func(t *testing.T) {
			//nolint:gosec // the directory is the developer's own opt-in input and file names come from the reviewed manifest.
			data, err := os.ReadFile(filepath.Join(filepath.Clean(dir), definition.Source.FileName))
			if os.IsNotExist(err) {
				t.Skipf("%s is not in %s", definition.Source.FileName, dir)
			}
			require.NoError(t, err)
			sum := sha256.Sum256(data)
			require.Equal(t, definition.Source.SHA256, hex.EncodeToString(sum[:]), "the local file is not the pinned file")
			work, ok := parsed[definition.Source.FileName]
			if !ok {
				work, err = ParseOgaSource(data)
				require.NoError(t, err)
				parsed[definition.Source.FileName] = work
				documents[definition.Source.FileName] = work.doc
			}
			unit, err := work.CutUnit(domain.ReadingUnitDefinition{
				Key: definition.Key, Language: definition.Language, WorkTitle: definition.WorkTitle, WorkAuthor: definition.WorkAuthor,
				EditionLabel: definition.EditionLabel, CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
				SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size, SourceFileSHA256: definition.Source.SHA256,
				InterpretationVersion: definition.InterpretationVersion,
			})
			require.NoError(t, err)
			assert.Equal(t, SyntaxStateNativeAGDT, unit.SyntaxState)
			assertPrintedWordsMatchSource(t, documents[definition.Source.FileName], unit)
			t.Logf("%s: %d sentences, %d tokens, %d unprinted nodes", definition.Key, unit.Report.Sentences, unit.Report.Tokens, countUnprintedNodes(unit))
		})
	}
}

func countUnprintedNodes(unit PreparedUnit) int {
	total := 0
	for _, sentence := range unit.Result.Sentences {
		total += len(sentence.UnprintedNodes)
	}
	return total
}

// assertPrintedWordsMatchSource compares the cut unit with the source file's own
// printed tokens: the sentences that start inside the unit, in file order.
func assertPrintedWordsMatchSource(t *testing.T, doc *sourceDocument, unit PreparedUnit) {
	t.Helper()
	var printed [][]sourceToken
	for _, sentence := range doc.sentences {
		kept := reconstruct(sentence).kept
		if len(kept) > 0 {
			printed = append(printed, kept)
		}
	}
	// The unit holds a contiguous run of the file's printed sentences. A
	// sentence can repeat, so the run is the first one that matches throughout.
	offset := -1
	for start := 0; start+len(unit.Result.Sentences) <= len(printed) && offset < 0; start++ {
		if sentencesMatch(printed[start:start+len(unit.Result.Sentences)], unit.Result.Sentences) {
			offset = start
		}
	}
	require.GreaterOrEqual(t, offset, 0, "the unit is not a contiguous run of the file's printed sentences")
	for sentenceIndex, sentence := range unit.Result.Sentences {
		source := printed[offset+sentenceIndex]
		require.Len(t, sentence.Tokens, len(source))
		nodeIDs := map[string]bool{}
		for _, node := range sentence.UnprintedNodes {
			nodeIDs[node.SourceID] = true
			assert.NotEmpty(t, node.Dependency)
		}
		for ordinal, token := range sentence.Tokens {
			assert.Equal(t, source[ordinal].surface, token.Surface)
			assert.Equal(t, source[ordinal].lemma, token.RawLemma)
			assert.NotEmpty(t, token.Dependency)
			assert.Less(t, int(token.Head), len(sentence.Tokens))
			assert.Equal(t, 0, indexNonASCII(token.Dependency), "the stored relation is ASCII, so the Zeta defect is gone: %q", token.Dependency)
			if token.HeadUnprintedNode != "" {
				assert.Equal(t, uint32(ordinal), token.Head, "a token headed by a node is self-headed")
				assert.True(t, nodeIDs[token.HeadUnprintedNode], "the head node is in the sentence")
			}
		}
	}
}

// indexNonASCII returns the count of non-ASCII runes in value.
func indexNonASCII(value string) int {
	count := 0
	for _, r := range value {
		if r > unicode.MaxASCII {
			count++
		}
	}
	return count
}

// sentencesMatch reports whether every cut sentence prints the source's tokens.
func sentencesMatch(source [][]sourceToken, cut []analyzer.Sentence) bool {
	for index, sentence := range cut {
		if len(sentence.Tokens) != len(source[index]) {
			return false
		}
		for ordinal, token := range sentence.Tokens {
			if token.Surface != source[index][ordinal].surface || token.RawLemma != source[index][ordinal].lemma {
				return false
			}
		}
	}
	return true
}
