package olaarchive_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"hash/crc32"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/olaarchive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realisticExtra is a local extra field like the 24 bytes the real archive's
// members carry, which the pin does not record.
var realisticExtra = []byte{0x55, 0x58, 0x14, 0x00, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

// buildArchive writes a zip of named members and returns it with each member's
// pin, as the manifest generator would record them from the central directory.
func buildArchive(t *testing.T, method uint16, members map[string][]byte, order []string) ([]byte, map[string]classical.ArchiveMember) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range order {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: method, Extra: realisticExtra})
		require.NoError(t, err)
		_, err = entry.Write(members[name])
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	require.NoError(t, err)
	pins := map[string]classical.ArchiveMember{}
	for _, file := range reader.File {
		dataOffset, err := file.DataOffset()
		require.NoError(t, err)
		offset := dataOffset - 30 - int64(len(file.Name)) - int64(len(realisticExtra))
		sum := sha256.Sum256(members[file.Name])
		pins[file.Name] = classical.ArchiveMember{
			Name:           file.Name,
			HeaderOffset:   offset,
			CompressedSize: int64(file.CompressedSize64),
			Size:           int64(file.UncompressedSize64),
			CRC32:          crc32Hex(members[file.Name]),
			SHA256:         hex.EncodeToString(sum[:]),
		}
	}
	return buffer.Bytes(), pins
}

func crc32Hex(data []byte) string {
	sum := crc32.ChecksumIEEE(data)
	return hex.EncodeToString([]byte{byte(sum >> 24), byte(sum >> 16), byte(sum >> 8), byte(sum)})
}

// fetchWindow cuts the requested window out of the archive the way a Range
// server would.
func fetchWindow(t *testing.T, archive []byte, member classical.ArchiveMember) []byte {
	t.Helper()
	start, end, err := olaarchive.Window(member, int64(len(archive)))
	require.NoError(t, err)
	return archive[start : end+1]
}

func TestExtractInflatesAndVerifiesDeflatedMember(t *testing.T) {
	text := bytes.Repeat([]byte("arma uirumque cano\n"), 200)
	archive, pins := buildArchive(t, zip.Deflate, map[string][]byte{"a/one.conllu": text, "a/two.xml": []byte("<paula/>")}, []string{"a/one.conllu", "a/two.xml"})

	got, err := olaarchive.Extract(fetchWindow(t, archive, pins["a/two.xml"]), pins["a/two.xml"])
	require.NoError(t, err)
	assert.Equal(t, []byte("<paula/>"), got)

	got, err = olaarchive.Extract(fetchWindow(t, archive, pins["a/one.conllu"]), pins["a/one.conllu"])
	require.NoError(t, err)
	assert.Equal(t, text, got)
	assert.Less(t, pins["a/one.conllu"].CompressedSize, pins["a/one.conllu"].Size)
}

func TestExtractReadsStoredMember(t *testing.T) {
	archive, pins := buildArchive(t, zip.Store, map[string][]byte{"s.txt": []byte("stored bytes")}, []string{"s.txt"})
	got, err := olaarchive.Extract(fetchWindow(t, archive, pins["s.txt"]), pins["s.txt"])
	require.NoError(t, err)
	assert.Equal(t, []byte("stored bytes"), got)
}

func TestWindowIsClampedToTheArchive(t *testing.T) {
	archive, pins := buildArchive(t, zip.Deflate, map[string][]byte{"only": []byte("x")}, []string{"only"})
	_, end, err := olaarchive.Window(pins["only"], int64(len(archive)))
	require.NoError(t, err)
	assert.Less(t, end, int64(len(archive)))
}

func TestWindowRefusesAMemberOutsideTheArchive(t *testing.T) {
	member := classical.ArchiveMember{Name: "x", HeaderOffset: 1000, CompressedSize: 10, Size: 10}
	_, _, err := olaarchive.Window(member, 500)
	assertPinMismatch(t, err)
}

func assertPinMismatch(t *testing.T, err error) {
	t.Helper()
	var refusal *classical.PreparationError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, classical.CodePinMismatch, refusal.Code)
}

func TestExtractRefusesMismatches(t *testing.T) {
	archive, pins := buildArchive(t, zip.Deflate, map[string][]byte{"m.txt": []byte("the pinned bytes")}, []string{"m.txt"})
	good := pins["m.txt"]
	window := fetchWindow(t, archive, good)

	tests := map[string]func(classical.ArchiveMember) classical.ArchiveMember{
		"crc":  func(m classical.ArchiveMember) classical.ArchiveMember { m.CRC32 = "00000000"; return m },
		"sha":  func(m classical.ArchiveMember) classical.ArchiveMember { m.SHA256 = hex.EncodeToString(make([]byte, 32)); return m },
		"size": func(m classical.ArchiveMember) classical.ArchiveMember { m.Size++; return m },
		"name": func(m classical.ArchiveMember) classical.ArchiveMember { m.Name = "other.txt"; return m },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := olaarchive.Extract(window, mutate(good))
			assertPinMismatch(t, err)
		})
	}
	t.Run("signature", func(t *testing.T) {
		broken := append([]byte(nil), window...)
		broken[0] = 'X'
		_, err := olaarchive.Extract(broken, good)
		assertPinMismatch(t, err)
	})
	t.Run("truncated", func(t *testing.T) {
		_, err := olaarchive.Extract(window[:30+len(good.Name)+len(realisticExtra)+5], good)
		assertPinMismatch(t, err)
	})
	t.Run("corrupt data", func(t *testing.T) {
		broken := append([]byte(nil), window...)
		broken[30+len(good.Name)+len(realisticExtra)+2] ^= 0xff
		_, err := olaarchive.Extract(broken, good)
		assertPinMismatch(t, err)
	})
}

func TestExtractRefusesAnInflatedBodyLongerThanPinned(t *testing.T) {
	archive, pins := buildArchive(t, zip.Deflate, map[string][]byte{"m.txt": bytes.Repeat([]byte("a"), 4096)}, []string{"m.txt"})
	short := pins["m.txt"]
	short.Size = 16
	_, err := olaarchive.Extract(fetchWindow(t, archive, pins["m.txt"]), short)
	assertPinMismatch(t, err)
}
