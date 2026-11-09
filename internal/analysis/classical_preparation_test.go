package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseClassicalSourceChoosesTheAdapterByFormat proves preparation never
// hard-wires the OGA adapter: the definition's reviewed source format selects
// it and an unknown format is refused with a stable code.
func TestParseClassicalSourceChoosesTheAdapterByFormat(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)

	parsed, err := parseClassicalSource(classical.SourceFormatOGACoNLLU, data)
	require.NoError(t, err)
	assert.NotNil(t, parsed)

	_, err = parseClassicalSource("graded-reader", data)
	require.Error(t, err)
	var importErr *ancientgreek.ImportError
	require.ErrorAs(t, err, &importErr)
	assert.Equal(t, ancientgreek.CodeUnknownSourceFormat, importErr.Code)
}

func TestClassicalSourceURLJoinsTheReviewedLayer(t *testing.T) {
	t.Parallel()
	resolved, err := classicalSourceURL("https://example.test/data/v1/", "work.conllu")
	require.NoError(t, err)
	assert.Equal(t, "https://example.test/data/v1/work.conllu", resolved)

	// A base URL without a trailing slash still addresses a file, not a sibling.
	resolved, err = classicalSourceURL("https://example.test/data/v1", "work.conllu")
	require.NoError(t, err)
	assert.Equal(t, "https://example.test/data/v1/work.conllu", resolved)
}

func TestVerifyClassicalSourceRefusesBadBytes(t *testing.T) {
	t.Parallel()
	data := []byte("reviewed bytes")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	cases := map[string]struct {
		data   []byte
		size   int64
		digest string
		code   string
	}{
		"accepts the reviewed bytes": {data: data, size: int64(len(data)), digest: digest},
		"refuses a size mismatch":    {data: data, size: int64(len(data)) + 1, digest: digest, code: ancientgreek.CodePinMismatch},
		"refuses a digest mismatch":  {data: data, size: int64(len(data)), digest: "deadbeef", code: ancientgreek.CodePinMismatch},
		"refuses oversize bytes":     {data: append(append([]byte(nil), data...), '!'), size: int64(len(data)), digest: digest, code: ancientgreek.CodeOversize},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := verifyClassicalSource(tc.data, tc.size, tc.digest)
			if tc.code == "" {
				require.NoError(t, err)
				return
			}
			var importErr *ancientgreek.ImportError
			require.ErrorAs(t, err, &importErr)
			assert.Equal(t, tc.code, importErr.Code)
		})
	}
}
