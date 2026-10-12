package persistence

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Readers filter on a literal builder version so sqlc and plain SQL stay
// statically analyzable. This keeps every literal in step with the builder.
func TestBrowseCountReaderVersionLiteralsMatchBuilderVersion(t *testing.T) {
	pattern := regexp.MustCompile(`builder_version\s*=\s*(\d+)`)
	roots := []string{".", filepath.Join("..", "analysis"), filepath.Join("..", "..", "sqlc", "queries")}
	found := 0
	for _, dir := range roots {
		root, err := os.OpenRoot(dir)
		require.NoError(t, err)
		testutil.Cleanup(t, "source root", root.Close)
		require.NoError(t, fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
			require.NoError(t, err)
			ext := filepath.Ext(path)
			if entry.IsDir() || (ext != ".go" && ext != ".sql") || regexp.MustCompile(`_test\.go$`).MatchString(path) {
				return nil
			}
			content, readErr := root.ReadFile(path)
			require.NoError(t, readErr)
			for _, match := range pattern.FindAllStringSubmatch(string(content), -1) {
				found++
				assert.Equal(t, strconv.Itoa(vocabularyBrowseCountBuilderVersion), match[1], "%s", filepath.Join(dir, path))
			}
			return nil
		}))
	}
	assert.Positive(t, found)
}
