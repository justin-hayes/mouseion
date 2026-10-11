package fixtures

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixturesDoNotImportPersistence keeps the fixtures store a peer of the
// PostgreSQL adapter rather than a wrapper around it: the contract types and
// sentinels both adapters share live in domain.
func TestFixturesDoNotImportPersistence(t *testing.T) {
	const persistencePath = "github.com/justin-hayes/mouseion/internal/persistence"
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	fset := token.NewFileSet()
	for _, name := range files {
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		require.NoError(t, err)
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			assert.NotEqual(t, persistencePath, path, "%s imports the PostgreSQL adapter", name)
		}
	}
}
