//go:build integration

package webapp

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/stretchr/testify/require"
)

const (
	latinLoopUnitKey = "caesar-gallic-war"
	latinLoopArchive = "/opera_latina_adnotata_v0.2.0.zip"
)

// latinLoopConllu is a synthetic OLA CoNLL-U for one Work: a preface sentence
// cited 1.pr.1 and a chapter sentence whose Citation carries a letter suffix.
const latinLoopConllu = `# absolute_id = 0
# clause_id = c_1
# file = synthetic.xml
1	Gallia	Gallia	n	_	Case=n|Gender=f|Number=s	4	SBJ	_	t_1
2	est	sum	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	0	PRED	_	t_2
3	omnis	omnis	a	_	Case=n|Gender=f|Number=s	1	ATR	_	t_3
4	diuisa	diuido	v	_	Case=n|Gender=f|Mood=p|Number=s|Tense=r|Voice=p	2	PNOM	_	t_4
5	.	.	u	_	_	2	AuxK	_	t_5

# absolute_id = 1
# clause_id = c_2
# file = synthetic.xml
1	Belgae	Belgae	n	_	Case=n|Gender=m|Number=p	3	SBJ	_	t_6
2	et	et1	c	_	_	3	COORD	_	t_7
3	Aquitani	Aquitani	n	_	Case=n|Gender=m|Number=p	4	SBJ_CO	_	t_8
4	incolunt	incolo	v	_	Mood=i|Number=p|Person=3|Tense=p|Voice=a	0	PRED	_	t_9
5	.	.	u	_	_	4	AuxK	_	t_10

`

// latinLoopPaula is the synthetic PAULA Citation layer for latinLoopConllu.
func latinLoopPaula() string {
	citations := []string{"1.pr.1", "1.pr.1", "1.pr.1", "1.pr.1", "1.pr.1", "1.1a", "1.1a", "1.1a", "1.1a", "1.1a"}
	var layer strings.Builder
	layer.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>` + "\n" +
		`<paula version="1.1"><header paula_id="synthetic"/><featList xmlns:xlink="http://www.w3.org/1999/xlink" type="cts" xml:base="x.tok01.xml">` + "\n")
	for index, citation := range citations {
		fmt.Fprintf(&layer, `<feat id="u_%d" xlink:href="#t_%d" value="%s"/>`+"\n", index+1, index+1, citation)
	}
	layer.WriteString("</featList></paula>\n")
	return layer.String()
}

// latinLoopFiller is incompressible, deterministic filler that makes the
// archive much larger than the Work's two members.
func latinLoopFiller(label string) []byte {
	var filler []byte
	sum := sha256.Sum256([]byte(label))
	for len(filler) < 32<<10 {
		filler = append(filler, sum[:]...)
		sum = sha256.Sum256(sum[:])
	}
	return filler
}

// latinLoopArchiveBuild is a synthetic OLA zip with the Work's two members among
// unrelated ones, and the pins the manifest generator would record for them.
type latinLoopArchiveBuild struct {
	archive []byte
	source  classical.SourcePin
}

func buildLatinLoopArchive(t *testing.T, layer string) latinLoopArchiveBuild {
	t.Helper()
	return buildLatinLoopArchiveFrom(t, layer, latinLoopConllu, latinLoopPaula())
}

// buildLatinLoopArchiveFrom is buildLatinLoopArchive for a Work's own synthetic
// CoNLL-U and PAULA members.
func buildLatinLoopArchiveFrom(t *testing.T, layer, conlluText, paulaText string) latinLoopArchiveBuild {
	t.Helper()
	const prefix = "opera_latina_adnotata_v0.2.0/"
	conlluName := prefix + "workspace/conllu/synthetic.conllu"
	paulaName := prefix + "workspace/ola/synthetic.tok01_cts01.xml"
	contents := map[string][]byte{
		prefix + "README.md": bytes.Repeat([]byte("readme\n"), 50),
		conlluName:           []byte(conlluText),
		prefix + "workspace/ola/other.tok01_cts01.xml": latinLoopFiller("other"),
		paulaName: []byte(paulaText),
	}
	order := []string{prefix + "README.md", conlluName, prefix + "workspace/ola/other.tok01_cts01.xml", paulaName}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	extra := make([]byte, 24)
	binary.LittleEndian.PutUint16(extra[0:], 0x7875)
	binary.LittleEndian.PutUint16(extra[2:], 20)
	for _, name := range order {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Extra: extra})
		require.NoError(t, err)
		_, err = entry.Write(contents[name])
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	require.NoError(t, err)
	pins := map[string]classical.ArchiveMember{}
	for _, file := range reader.File {
		dataOffset, err := file.DataOffset()
		require.NoError(t, err)
		sum := sha256.Sum256(contents[file.Name])
		pins[file.Name] = classical.ArchiveMember{
			Name:           file.Name,
			HeaderOffset:   dataOffset - 30 - int64(len(file.Name)) - int64(len(extra)),
			CompressedSize: int64(file.CompressedSize64),   //nolint:gosec // synthetic member sizes are tiny
			Size:           int64(file.UncompressedSize64), //nolint:gosec // synthetic member sizes are tiny
			CRC32:          fmt.Sprintf("%08x", crc32.ChecksumIEEE(contents[file.Name])),
			SHA256:         hex.EncodeToString(sum[:]),
		}
	}
	conllu, paula := pins[conlluName], pins[paulaName]
	return latinLoopArchiveBuild{archive: buffer.Bytes(), source: classical.SourcePin{
		Layer: layer, FileName: conllu.Name, Size: conllu.Size, SHA256: conllu.SHA256,
		Format: classical.SourceFormatOLACoNLLU, Members: []classical.ArchiveMember{conllu, paula},
	}}
}

// rangeServer serves one archive and honours a single Range request, as Zenodo
// does. It records every request so a test can prove only the members' byte
// windows were ever served.
type rangeServer struct {
	archive []byte
	mu      sync.Mutex
	ranges  []string
	served  int
}

func (s *rangeServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spec, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes=")
		first, last, found := strings.Cut(spec, "-")
		start, startErr := strconv.Atoi(first)
		end, endErr := strconv.Atoi(last)
		s.mu.Lock()
		s.ranges = append(s.ranges, r.Header.Get("Range"))
		s.mu.Unlock()
		if r.URL.Path != latinLoopArchive || !ok || !found || startErr != nil || endErr != nil || start > end || end >= len(s.archive) {
			http.Error(w, "range required", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		s.mu.Lock()
		s.served += end - start + 1
		s.mu.Unlock()
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(s.archive)))
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(s.archive[start : end+1]); err != nil {
			return
		}
	})
}

func (s *rangeServer) requests() ([]string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ranges...), s.served
}

// TestClassicalLatinCatalogLoop walks a Latin Work end to end through the real
// routes, store and workers with no NLP service: the Catalog filtered to Latin,
// Add, preparation fetching the Work's two archive members by Range, Reading and
// Sentence study with widened Citations and stored AGDT relations, the Latin
// study language, and no Construction views.
