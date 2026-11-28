package download

import (
	"crypto/md5" //nolint:gosec // reference digest for a test.
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func partialMD5Of(t *testing.T, data []byte, chunk int) string {
	t.Helper()
	writer := NewPartialMD5()
	for len(data) > 0 {
		n := min(chunk, len(data))
		written, err := writer.Write(data[:n])
		require.NoError(t, err)
		require.Equal(t, n, written)
		data = data[n:]
	}
	return writer.Sum()
}

// reference samples the bytes directly with random access, as KOReader does.
func partialMD5Reference(data []byte) string {
	offsets := []int{0, 1 << 10, 1 << 12, 1 << 14, 1 << 16, 1 << 18, 1 << 20, 1 << 22, 1 << 24, 1 << 26, 1 << 28, 1 << 30}
	sum := md5.New() //nolint:gosec // reference digest for a test.
	for _, offset := range offsets {
		if offset >= len(data) {
			break
		}
		sum.Write(data[offset:min(offset+1024, len(data))])
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func TestPartialMD5MatchesKOReaderVectors(t *testing.T) {
	// Vectors from KOReader's spec/unit/util_spec.lua; the files come from
	// koreader/test-data (see testdata/PROVENANCE.md).
	for name, want := range map[string]string{
		"tall.pdf":    "41cce710f34e5ec21315e19c99821415",
		"leaves.epub": "59d481d168cca6267322f150c5f6a2a3",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // fixed fixture names.
			require.NoError(t, err)
			for _, chunk := range []int{1, 7, 1024, 32 * 1024, len(data)} {
				assert.Equal(t, want, partialMD5Of(t, data, chunk), "chunk %d", chunk)
			}
		})
	}
}

func TestPartialMD5Boundaries(t *testing.T) {
	data := make([]byte, 5000)
	for i := range data {
		data[i] = byte(i*31 + i/251)
	}
	cases := map[string]int{
		"empty":                      0,
		"under one KiB":              100,
		"exactly one sample":         1024,
		"ends inside a sample":       1024 + 500,
		"ends exactly on an offset":  4096,
		"one byte past an offset":    4097,
		"ends inside the third":      4096 + 700,
		"past the second sample end": 2049,
	}
	for name, size := range cases {
		t.Run(name, func(t *testing.T) {
			want := partialMD5Reference(data[:size])
			for _, chunk := range []int{1, 3, 1000, 1024, 4096, 10000} {
				assert.Equal(t, want, partialMD5Of(t, data[:size], chunk), "chunk %d", chunk)
			}
		})
	}
	assert.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", partialMD5Of(t, nil, 1))
}

func TestPartialMD5SamplesFarOffsets(t *testing.T) {
	data := make([]byte, 20<<20)
	for i := range data {
		data[i] = byte((i ^ i>>8 ^ i>>16) & 0xff)
	}
	assert.Equal(t, partialMD5Reference(data), partialMD5Of(t, data, 64*1024))
}
