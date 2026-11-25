package analysis

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// archiveFixture is a synthetic zip of three members and the pins of the two
// the unit reads.
func archiveFixture(t *testing.T) ([]byte, classical.SourcePin) {
	t.Helper()
	members := []struct {
		name string
		data []byte
	}{
		{"pad/readme", bytes.Repeat([]byte("0123456789abcdef"), 400)},
		{"work/one.conllu", []byte("conllu bytes of the work\n")},
		{"work/one.cts.xml", []byte("<paula>citations</paula>")},
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, member := range members {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: member.name, Method: zip.Deflate})
		require.NoError(t, err)
		_, err = entry.Write(member.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	require.NoError(t, err)
	pin := func(index int) classical.ArchiveMember {
		file := reader.File[index]
		offset, err := file.DataOffset()
		require.NoError(t, err)
		sum := sha256.Sum256(members[index].data)
		return classical.ArchiveMember{
			Name: file.Name, HeaderOffset: offset - 30 - int64(len(file.Name)),
			CompressedSize: int64(file.CompressedSize64), Size: int64(file.UncompressedSize64), //nolint:gosec // synthetic member sizes are tiny
			CRC32: fmt.Sprintf("%08x", crc32.ChecksumIEEE(members[index].data)), SHA256: hex.EncodeToString(sum[:]),
		}
	}
	conllu, paula := pin(1), pin(2)
	return buffer.Bytes(), classical.SourcePin{
		Layer: "ola", FileName: conllu.Name, Size: conllu.Size, SHA256: conllu.SHA256,
		Format: classical.SourceFormatOLACoNLLU, Members: []classical.ArchiveMember{conllu, paula},
	}
}

func archiveConfig(archive []byte, pin classical.SourcePin, server *httptest.Server) (ClassicalPreparationConfig, domain.ClassicalPreparationTarget) {
	definition := classical.Definition{Key: "unit", Source: pin}
	config := ClassicalPreparationConfig{
		Layers: classical.Layers{Layers: []classical.Layer{{
			Key: "ola", FetchKind: classical.FetchKindArchiveRange, ArchiveURL: server.URL + "/archive.zip", ArchiveSize: int64(len(archive)),
		}}},
		Definitions: []classical.Definition{definition},
		HTTPClient:  server.Client(),
		IdleTimeout: time.Second,
	}
	return config, domain.ClassicalPreparationTarget{ReadingUnitKey: "unit", SourceLayer: "ola", SourceFileSHA256: pin.SHA256, SourceFormat: pin.Format}
}

func TestFetchArchiveMembersReadsEachMemberByRange(t *testing.T) {
	archive, pin := archiveFixture(t)
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spec := strings.TrimPrefix(r.Header.Get("Range"), "bytes=")
		ranges = append(ranges, spec)
		first, last, _ := strings.Cut(spec, "-")
		start, startErr := strconv.Atoi(first)
		end, endErr := strconv.Atoi(last)
		if startErr != nil || endErr != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(archive)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(archive[start : end+1]) //nolint:errcheck // a closed test connection is the test's concern
	}))
	t.Cleanup(server.Close)
	config, target := archiveConfig(archive, pin, server)

	members, err := config.fetchArchiveMembers(t.Context(), target, time.Minute)
	require.NoError(t, err)
	require.Len(t, members, 2)
	assert.Equal(t, "conllu bytes of the work\n", string(members[0]))
	assert.Equal(t, "<paula>citations</paula>", string(members[1]))
	assert.Len(t, ranges, 2)
}

func TestFetchArchiveMembersRefusesAServerThatIgnoresRange(t *testing.T) {
	archive, pin := archiveFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive) //nolint:errcheck // a closed test connection is the test's concern
	}))
	t.Cleanup(server.Close)
	config, target := archiveConfig(archive, pin, server)

	_, err := config.fetchArchiveMembers(t.Context(), target, time.Minute)
	var refusal *classical.PreparationError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, classical.CodePinMismatch, refusal.Code)
}

func TestFetchArchiveMembersRefusesADisagreeingContentRange(t *testing.T) {
	archive, pin := archiveFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-9/"+strconv.Itoa(len(archive)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(archive[:10]) //nolint:errcheck // a closed test connection is the test's concern
	}))
	t.Cleanup(server.Close)
	config, target := archiveConfig(archive, pin, server)

	_, err := config.fetchArchiveMembers(t.Context(), target, time.Minute)
	var refusal *classical.PreparationError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, classical.CodePinMismatch, refusal.Code)
}

func TestFetchArchiveMembersRefusesAnUnknownPin(t *testing.T) {
	archive, pin := archiveFixture(t)
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	config, target := archiveConfig(archive, pin, server)

	t.Run("no definition", func(t *testing.T) {
		config := config
		config.Definitions = nil
		_, err := config.fetchArchiveMembers(t.Context(), target, time.Minute)
		var refusal *classical.PreparationError
		require.ErrorAs(t, err, &refusal)
		assert.Equal(t, classical.CodePinMismatch, refusal.Code)
	})
	t.Run("not an archive layer", func(t *testing.T) {
		config := config
		config.Layers = classical.Layers{Layers: []classical.Layer{{Key: "ola", BaseURL: server.URL + "/"}}}
		_, err := config.fetchArchiveMembers(t.Context(), target, time.Minute)
		var refusal *classical.PreparationError
		require.ErrorAs(t, err, &refusal)
		assert.Equal(t, classical.CodeSourceLayerUnknown, refusal.Code)
	})
}
