package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/constructions/releasecheck"
)

func TestSourceURLJoinsTheLayerBaseAndFileName(t *testing.T) {
	got, err := sourceURL("https://example.test/data", "a.conllu")
	require.NoError(t, err)
	assert.Equal(t, "https://example.test/data/a.conllu", got)
	_, err = sourceURL("not a url", "a.conllu")
	require.Error(t, err)
}

func TestGateAdapterReadsTheShippingGate(t *testing.T) {
	gate := gateAdapter{constructions.ShippingGate}
	assert.True(t, gate.Ships(string(constructions.Conditional)))
	assert.False(t, gate.Ships("unknown"))
	assert.True(t, gate.Ships(string(constructions.Purpose)))
	assert.True(t, gate.ShipsLabel(string(constructions.Result), string(constructions.SubtypeNatural)))
	assert.False(t, gate.ShipsLabel(string(constructions.Result), string(constructions.SubtypeActual)))
}

func TestCachedFetcherDownloadsVerifiesAndCaches(t *testing.T) {
	body := []byte("source bytes")
	sum := sha256.Sum256(body)
	pin := classical.SourcePin{Layer: "layer", FileName: "f.conllu", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/data/f.conllu" {
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write(body); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	newFetcher := func(base string) *cachedFetcher {
		return &cachedFetcher{
			dir: filepath.Join(dir, "cache"), client: server.Client(),
			layers: classical.Layers{Layers: []classical.Layer{{Key: "layer", BaseURL: base}}},
		}
	}

	t.Run("downloads once and then serves the cache", func(t *testing.T) {
		fetcher := newFetcher(server.URL + "/data/")
		for range 2 {
			got, err := fetcher.Fetch(context.Background(), pin)
			require.NoError(t, err)
			assert.Equal(t, body, got)
		}
		assert.Equal(t, 1, requests)
	})
	t.Run("a corrupt cache entry is downloaded again", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cache", "f.conllu"), []byte("bad"), 0o600))
		got, err := newFetcher(server.URL+"/data/").Fetch(context.Background(), pin)
		require.NoError(t, err)
		assert.Equal(t, body, got)
	})
	t.Run("a download that does not match its pin is refused and not cached", func(t *testing.T) {
		wrong := pin
		wrong.FileName = "g.conllu"
		wrong.Size = 3
		_, err := newFetcher(server.URL+"/other/").Fetch(context.Background(), wrong)
		require.Error(t, err)
		assert.NoFileExists(t, filepath.Join(dir, "cache", "g.conllu"))
	})
	t.Run("an unknown layer is refused", func(t *testing.T) {
		unknown := pin
		unknown.Layer = "missing"
		unknown.FileName = "h.conllu"
		_, err := newFetcher(server.URL).Fetch(context.Background(), unknown)
		require.Error(t, err)
	})
	t.Run("a bad base URL is refused", func(t *testing.T) {
		other := pin
		other.FileName = "i.conllu"
		_, err := newFetcher("nope").Fetch(context.Background(), other)
		require.Error(t, err)
	})
	t.Run("a mismatching body is refused", func(t *testing.T) {
		other := pin
		other.FileName = "j.conllu"
		other.SHA256 = strings.Repeat("0", 64)
		_, err := newFetcher(server.URL+"/data/").Fetch(context.Background(), other)
		require.Error(t, err)
	})
}

func TestWriteBaselineRecordsEmissionAndHitCounts(t *testing.T) {
	dir := t.TempDir()
	judgments := []releasecheck.Judgment{
		{Work: "w", SentID: "1", Anchor: "t_1", Construction: "result", Measurement: "2615", Item: "R1", Stratum: "hit", Judgment: true, Judge: "agent"},
		{Work: "w", SentID: "2", Anchor: "t_2", Construction: "result", Measurement: "2615", Item: "R2", Stratum: "hit", Judgment: false, Judge: "agent", EmittedBaseline: true},
	}
	detected := releasecheck.Detected{
		Emitted: map[releasecheck.Key]string{{Work: "w", SentID: "1", Anchor: "t_1", Construction: "result"}: ""},
		Hits:    map[string]int{"result": 7},
	}
	judgmentsPath, baselinePath := filepath.Join(dir, "j.tsv"), filepath.Join(dir, "b.tsv")
	require.NoError(t, writeBaseline(judgmentsPath, baselinePath, judgments, detected, []string{"result"}))

	written, err := os.ReadFile(judgmentsPath) //nolint:gosec // test temp path.
	require.NoError(t, err)
	back, err := releasecheck.ParseJudgments(bytes.NewReader(written))
	require.NoError(t, err)
	require.Len(t, back, 2)
	assert.True(t, back[0].EmittedBaseline)
	assert.False(t, back[1].EmittedBaseline)

	base, err := readBaseline(baselinePath)
	require.NoError(t, err)
	assert.Equal(t, constructions.AGDTDetectorVersion, base.DetectorVersion)
	assert.Equal(t, 7, base.Hits["result"])
}

func TestReadersReportMissingFiles(t *testing.T) {
	_, err := readCounts(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	_, err = readBaseline(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-baseline")
}

func TestRunRefusesAnUnknownFlag(t *testing.T) {
	require.Error(t, run(context.Background(), []string{"-nope"}, &bytes.Buffer{}))
}

func TestRejectCacheInsideRepository(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0o750))
	require.NoError(t, os.Symlink(filepath.Join(root, "sub"), filepath.Join(outside, "link")))

	t.Run("a path in the repository is refused", func(t *testing.T) {
		err := rejectCacheInside(filepath.Join(root, "sub", "cache"), root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "inside the repository")
	})
	t.Run("the repository root itself is refused", func(t *testing.T) {
		require.Error(t, rejectCacheInside(root, root))
	})
	t.Run("a symlink into the repository is refused", func(t *testing.T) {
		require.Error(t, rejectCacheInside(filepath.Join(outside, "link", "cache"), root))
	})
	t.Run("a path outside is accepted, existing or not", func(t *testing.T) {
		require.NoError(t, rejectCacheInside(outside, root))
		require.NoError(t, rejectCacheInside(filepath.Join(outside, "new", "cache"), root))
	})
	t.Run("a sibling with the same prefix is accepted", func(t *testing.T) {
		require.NoError(t, rejectCacheInside(root+"-cache", root))
	})
}

func TestRepositoryRootFindsGoMod(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o600))
	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o750))
	got, err := repositoryRoot(nested)
	require.NoError(t, err)
	assert.Equal(t, root, got)
	_, err = repositoryRoot(t.TempDir())
	require.Error(t, err)
}

func TestRunRefusesACacheInsideTheRepository(t *testing.T) {
	err := run(context.Background(), []string{"-cache", "."}, &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inside the repository")
}
