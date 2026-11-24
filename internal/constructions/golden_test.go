package constructions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/syntax/agdt"
)

var update = flag.Bool("update", false, "rewrite testdata/golden.jsonl after bumping AGDTDetectorVersion")

const (
	goldenPath = "testdata/golden.jsonl"
	corpusPath = "testdata/oga-src-constructions.conllu"
)

type goldenHeader struct {
	DetectorVersion int `json:"detector_version"`
}

type goldenLine struct {
	Sentence     int    `json:"sentence"`
	Construction ID     `json:"construction"`
	Subtype      string `json:"subtype"`
	Anchor       int    `json:"anchor"`
	Extent       []int  `json:"extent"`
}

// goldenOutput runs the synthetic corpus through the import code, the adapter
// and Detect, without a database.
func goldenOutput(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(corpusPath)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	parsed, err := ancientgreek.ParseOgaSource(data)
	require.NoError(t, err)
	unit, err := parsed.CutUnit(domain.ReadingUnitDefinition{
		Key: "xenophon-anabasis-1", Language: ancientgreek.Language, WorkTitle: "Anabasis", WorkAuthor: "Xenophon",
		EditionLabel: "Marchant 1904", CitationStart: "1.1.1", CitationEnd: "1.10.19",
		SourceFileName: "tlg0032.tlg006.perseus-grc2_oga020_src.conllu", SourceFileSize: int64(len(data)),
		SourceFileSHA256: hex.EncodeToString(sum[:]), InterpretationVersion: ancientgreek.PosMappingVersion,
	})
	require.NoError(t, err)

	var out bytes.Buffer
	encode := func(v any) {
		line, err := json.Marshal(v)
		require.NoError(t, err)
		out.Write(line)
		out.WriteByte('\n')
	}
	encode(goldenHeader{DetectorVersion: AGDTDetectorVersion})
	for index, sentence := range unit.Result.Sentences {
		for _, occurrence := range Detect(agdt.FromSentence(sentence)) {
			encode(goldenLine{
				Sentence: index, Construction: occurrence.Construction, Subtype: string(occurrence.Subtype),
				Anchor: occurrence.Anchor, Extent: occurrence.Extent,
			})
		}
	}
	return out.Bytes()
}

func TestGolden(t *testing.T) {
	got := goldenOutput(t)
	want, err := os.ReadFile(goldenPath)
	if err != nil && !*update {
		require.NoError(t, err, "read golden")
	}
	recorded := goldenHeader{}
	if len(want) > 0 {
		require.NoError(t, json.Unmarshal(bytes.SplitN(want, []byte("\n"), 2)[0], &recorded), "golden header")
	}

	if *update {
		require.NotEqual(t, AGDTDetectorVersion, recorded.DetectorVersion,
			"bump AGDTDetectorVersion before updating the golden file")
		require.NoError(t, os.WriteFile(goldenPath, got, 0o600))
		return
	}
	if recorded.DetectorVersion == AGDTDetectorVersion {
		assert.Equal(t, string(want), string(got),
			"the detectors' output changed: bump AGDTDetectorVersion, then run `go test ./internal/constructions -run Golden -update`")
		return
	}
	assert.Fail(t, "AGDTDetectorVersion was bumped but the golden file is stale",
		"run `go test ./internal/constructions -run Golden -update`")
}
