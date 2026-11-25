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

// nestedArchive wraps an inner zip as one member of an outer zip, between other
// members, with the given method.
func nestedArchive(t *testing.T, innerMethod uint16, outerMethod uint16, members map[string][]byte, order []string) []byte {
	t.Helper()
	inner, _ := buildArchive(t, innerMethod, members, order)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, member := range []struct {
		name   string
		method uint16
		data   []byte
	}{{"workspace/before.txt", zip.Deflate, bytes.Repeat([]byte("pad "), 40)}, {"workspace/conllu.zip", outerMethod, inner}, {"workspace/after.txt", zip.Store, []byte("tail")}} {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: member.name, Method: member.method, Extra: realisticExtra})
		require.NoError(t, err)
		_, err = entry.Write(member.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func TestPinNestedGivesAnAbsolutePinTheSingleLevelReaderFetches(t *testing.T) {
	text := bytes.Repeat([]byte("arma uirumque cano\n"), 100)
	outer := nestedArchive(t, zip.Deflate, zip.Store, map[string][]byte{"w/a.conllu": text, "w/b.xml": []byte("<paula/>")}, []string{"w/a.conllu", "w/b.xml"})
	reader := bytes.NewReader(outer)
	directory, err := olaarchive.ReadDirectory(reader, int64(len(outer)))
	require.NoError(t, err)

	for name, want := range map[string][]byte{"w/a.conllu": text, "w/b.xml": []byte("<paula/>")} {
		member, data, err := directory.PinNested(reader, int64(len(outer)), "workspace/conllu.zip", name)
		require.NoError(t, err)
		assert.Equal(t, want, data)
		assert.Equal(t, name, member.Name)
		start, end, err := olaarchive.Window(member, int64(len(outer)))
		require.NoError(t, err)
		got, err := olaarchive.Extract(outer[start:end+1], member)
		require.NoError(t, err)
		assert.Equal(t, want, got, "the pin is absolute in the outer archive")
	}
}

func TestPinNestedRefusesWhatCannotBeAddressedByOffset(t *testing.T) {
	members := map[string][]byte{"w/a.conllu": []byte("x")}
	compressed := nestedArchive(t, zip.Store, zip.Deflate, members, []string{"w/a.conllu"})
	reader := bytes.NewReader(compressed)
	directory, err := olaarchive.ReadDirectory(reader, int64(len(compressed)))
	require.NoError(t, err)
	_, _, err = directory.PinNested(reader, int64(len(compressed)), "workspace/conllu.zip", "w/a.conllu")
	require.ErrorContains(t, err, "not stored")

	stored := nestedArchive(t, zip.Store, zip.Store, members, []string{"w/a.conllu"})
	reader = bytes.NewReader(stored)
	directory, err = olaarchive.ReadDirectory(reader, int64(len(stored)))
	require.NoError(t, err)
	_, _, err = directory.PinNested(reader, int64(len(stored)), "workspace/missing.zip", "w/a.conllu")
	require.ErrorContains(t, err, "no member")
	_, _, err = directory.PinNested(reader, int64(len(stored)), "workspace/conllu.zip", "w/missing")
	require.ErrorContains(t, err, "no member")
	_, _, err = directory.PinNested(reader, int64(len(stored)), "workspace/before.txt", "w/a.conllu")
	require.ErrorContains(t, err, "not stored")
}
