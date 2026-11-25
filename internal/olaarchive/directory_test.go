package olaarchive_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/justin-hayes/mouseion/internal/olaarchive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryPinsMatchTheMemberPinsTheTestsComputeIndependently(t *testing.T) {
	members := map[string][]byte{"a/one.conllu": bytes.Repeat([]byte("arma\n"), 300), "a/two.xml": []byte("<paula/>")}
	archive, want := buildArchive(t, zip.Deflate, members, []string{"a/one.conllu", "a/two.xml"})

	directory, err := olaarchive.ReadDirectory(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	for name, expected := range want {
		got, data, err := directory.Pin(bytes.NewReader(archive), int64(len(archive)), name)
		require.NoError(t, err)
		assert.Equal(t, expected, got)
		assert.Equal(t, members[name], data)
	}
	_, _, err = directory.Pin(bytes.NewReader(archive), int64(len(archive)), "missing")
	require.ErrorContains(t, err, "no member")
}

func TestReadDirectoryRefusesWhatItCannotPin(t *testing.T) {
	_, err := olaarchive.ReadDirectory(bytes.NewReader([]byte("not a zip file at all, just text")), 32)
	require.ErrorContains(t, err, "no zip end record")
	_, err = olaarchive.ReadDirectory(bytes.NewReader([]byte("tiny")), 4)
	require.Error(t, err)
}
