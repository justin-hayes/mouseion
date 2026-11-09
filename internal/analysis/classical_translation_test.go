package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type translationStoreStub struct {
	exists  bool
	hasErr  error
	putErr  error
	putPin  string
	putLang string
	put     map[string]string
	hasCall int
	putCall int
}

func (s *translationStoreStub) HasReferenceTranslations(context.Context, string) (bool, error) {
	s.hasCall++
	return s.exists, s.hasErr
}

func (s *translationStoreStub) PutReferenceTranslations(_ context.Context, pin, language string, translations map[string]string) (int, error) {
	s.putCall++
	s.putPin, s.putLang, s.put = pin, language, translations
	return len(translations), s.putErr
}

func translationFixture(t *testing.T) ([]byte, ClassicalTranslationArgs) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "perseus-eng2-section.xml"))
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return data, ClassicalTranslationArgs{
		Layer: "translation-layer", FileName: "translation.xml", Size: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), CitationStart: "1.1.1", CitationEnd: "1.2.18",
		Language: classical.Language,
	}
}

func translationLayer(t *testing.T, body []byte, requests *atomic.Int64) ClassicalPreparationConfig {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if _, err := w.Write(body); err != nil {
			return
		}
	}))
	t.Cleanup(server.Close)
	return ClassicalPreparationConfig{
		Layers: classical.Layers{Layers: []classical.Layer{{
			Key: "translation-layer", BaseURL: server.URL + "/",
			LicenceClass: classical.LicenceClassSourceContent, Restriction: classical.RestrictionShareAlike,
			DeployerRegisterRow: "synthetic test layer",
		}}},
		HTTPClient: server.Client(), TempDir: t.TempDir(),
	}
}

func translationJob(args ClassicalTranslationArgs) *river.Job[ClassicalTranslationArgs] {
	return &river.Job[ClassicalTranslationArgs]{Args: args}
}

func TestClassicalTranslationWorkerImportsPinnedSections(t *testing.T) {
	t.Parallel()
	data, args := translationFixture(t)
	var requests atomic.Int64
	store := &translationStoreStub{}
	worker := &ClassicalTranslationWorker{Store: store, Config: translationLayer(t, data, &requests)}
	require.NoError(t, worker.Work(context.Background(), translationJob(args)))
	assert.Equal(t, int64(1), requests.Load())
	assert.Equal(t, 1, store.putCall)
	assert.Equal(t, args.SHA256, store.putPin)
	assert.Equal(t, args.Language, store.putLang)
	assert.Len(t, store.put, 4)
	assert.Equal(t, "He halted his chariot in front of the phalanx and gave orders.", store.put["1.2.17"])
}

func TestClassicalTranslationWorkerSkipsExistingPinWithoutDownloading(t *testing.T) {
	t.Parallel()
	data, args := translationFixture(t)
	var requests atomic.Int64
	store := &translationStoreStub{exists: true}
	worker := &ClassicalTranslationWorker{Store: store, Config: translationLayer(t, data, &requests)}
	require.NoError(t, worker.Work(context.Background(), translationJob(args)))
	assert.Zero(t, requests.Load(), "rows for the pin already exist")
	assert.Zero(t, store.putCall)
}

func TestClassicalTranslationWorkerFailurePaths(t *testing.T) {
	t.Parallel()
	data, args := translationFixture(t)
	badPin := args
	badPin.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	badPin.Size = int64(len(data))
	oversize := args
	oversize.Size = 16
	unknownLayer := args
	unknownLayer.Layer = "missing"
	cases := map[string]struct {
		store *translationStoreStub
		args  ClassicalTranslationArgs
	}{
		"store exists error": {store: &translationStoreStub{hasErr: errors.New("has failed")}, args: args},
		"unknown layer":      {store: &translationStoreStub{}, args: unknownLayer},
		"oversize":           {store: &translationStoreStub{}, args: oversize},
		"pin mismatch":       {store: &translationStoreStub{}, args: badPin},
		"put error":          {store: &translationStoreStub{putErr: errors.New("put failed")}, args: args},
		"invalid pin":        {store: &translationStoreStub{}, args: ClassicalTranslationArgs{Layer: args.Layer, FileName: args.FileName, Size: args.Size, SHA256: "short", CitationStart: args.CitationStart, CitationEnd: args.CitationEnd, Language: args.Language}},
		"missing language":   {store: &translationStoreStub{}, args: ClassicalTranslationArgs{Layer: args.Layer, FileName: args.FileName, Size: args.Size, SHA256: args.SHA256, CitationStart: args.CitationStart, CitationEnd: args.CitationEnd}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int64
			config := translationLayer(t, data, &requests)
			worker := &ClassicalTranslationWorker{Store: tc.store, Config: config}
			require.Error(t, worker.Work(context.Background(), translationJob(tc.args)))
		})
	}

	// A server failure and a malformed body are both retryable job errors.
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failingServer.Close)
	worker := &ClassicalTranslationWorker{Store: &translationStoreStub{}, Config: ClassicalPreparationConfig{
		Layers: classical.Layers{Layers: []classical.Layer{{
			Key: "translation-layer", BaseURL: failingServer.URL + "/",
			LicenceClass: classical.LicenceClassSourceContent, Restriction: classical.RestrictionShareAlike,
			DeployerRegisterRow: "synthetic test layer",
		}}},
		HTTPClient: failingServer.Client(),
	}}
	require.Error(t, worker.Work(context.Background(), translationJob(args)))

	malformed := []byte(`<?xml version="1.0"?><TEI><div`)
	malformedArgs := args
	sum := sha256.Sum256(malformed)
	malformedArgs.Size = int64(len(malformed))
	malformedArgs.SHA256 = hex.EncodeToString(sum[:])
	var malformedRequests atomic.Int64
	malformedWorker := &ClassicalTranslationWorker{Store: &translationStoreStub{}, Config: translationLayer(t, malformed, &malformedRequests)}
	require.Error(t, malformedWorker.Work(context.Background(), translationJob(malformedArgs)))
}

func TestClassicalTranslationWorkerRefusesMissingStore(t *testing.T) {
	t.Parallel()
	_, args := translationFixture(t)
	require.Error(t, (&ClassicalTranslationWorker{}).Work(context.Background(), translationJob(args)))
	assert.Equal(t, ClassicalTranslationQueue, (ClassicalTranslationArgs{}).Kind())
}

func TestClassicalTranslationWorkerUsesGeneralisedParserForShallowCitations(t *testing.T) {
	t.Parallel()
	body := []byte(`<TEI><div type="textpart" subtype="section" n="1"><p>First.</p></div></TEI>`)
	sum := sha256.Sum256(body)
	args := ClassicalTranslationArgs{
		Layer: "translation-layer", FileName: "translation.xml", Size: int64(len(body)),
		SHA256: hex.EncodeToString(sum[:]), CitationStart: "1", CitationEnd: "1",
		Language: ancientgreek.Language,
	}
	var requests atomic.Int64
	store := &translationStoreStub{}
	worker := &ClassicalTranslationWorker{Store: store, Config: translationLayer(t, body, &requests)}
	require.NoError(t, worker.Work(context.Background(), translationJob(args)))
	assert.Equal(t, map[string]string{"1": "First."}, store.put)
}
