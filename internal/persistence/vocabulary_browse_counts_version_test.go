package persistence

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Readers filter on a literal builder version so sqlc and plain SQL stay
// statically analyzable. This keeps every literal in step with the builder.
func TestBrowseCountReaderVersionLiteralsMatchBuilderVersion(t *testing.T) {
	pattern := regexp.MustCompile(`builder_version\s*=\s*(\d+)`)
	roots := []string{".", filepath.Join("..", "analysis"), filepath.Join("..", "..", "sqlc", "queries")}
	found := 0
	for _, root := range roots {
		require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			require.NoError(t, err)
			ext := filepath.Ext(path)
			if entry.IsDir() || (ext != ".go" && ext != ".sql") || regexp.MustCompile(`_test\.go$`).MatchString(path) {
				return nil
			}
			content, readErr := os.ReadFile(path) //nolint:gosec // walks fixed in-repo source directories
			require.NoError(t, readErr)
			for _, match := range pattern.FindAllStringSubmatch(string(content), -1) {
				found++
				assert.Equal(t, strconv.Itoa(vocabularyBrowseCountBuilderVersion), match[1], "%s", path)
			}
			return nil
		}))
	}
	assert.Positive(t, found)
}
