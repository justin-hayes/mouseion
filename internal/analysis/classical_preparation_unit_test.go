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
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRiverClient satisfies the service's enqueue seam for paths that return
// before a transaction is opened.
type fakeRiverClient struct{}

func (fakeRiverClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, errors.New("unexpected insert")
}

func (fakeRiverClient) JobGet(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, errors.New("unexpected job get")
}

func (fakeRiverClient) JobCancel(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, errors.New("unexpected job cancel")
}

// preparationStoreStub is an in-memory ClassicalPreparationStore for worker
// error paths. The integration test covers the real PostgreSQL store.
type preparationStoreStub struct {
	target      domain.ClassicalPreparationTarget
	loadErr     error
	complete    bool
	completeErr error
	source      domain.ClassicalSource
	sourceFound bool
	sourceErr   error
	putErr      error
	putCalls    int
	record      domain.ClassicalPreparedDefinition
	recordErr   error
}

func (s *preparationStoreStub) LoadClassicalPreparationTarget(context.Context, string, string) (domain.ClassicalPreparationTarget, error) {
	return s.target, s.loadErr
}

func (s *preparationStoreStub) ClassicalPreparationComplete(context.Context, domain.ClassicalPreparationTarget) (bool, error) {
	return s.complete, s.completeErr
}

func (s *preparationStoreStub) GetClassicalSource(context.Context, string) (domain.ClassicalSource, bool, error) {
	return s.source, s.sourceFound, s.sourceErr
}

func (s *preparationStoreStub) PutClassicalSource(_ context.Context, source domain.ClassicalSource) error {
	s.putCalls++
	s.source, s.sourceFound = source, true
	return s.putErr
}

func (s *preparationStoreStub) PutClassicalPreparedDefinition(_ context.Context, record domain.ClassicalPreparedDefinition) error {
	s.record = record
	return s.recordErr
}

type preparationWriterStub struct {
	result ancientgreek.ImportWriteResult
	err    error
	calls  int
	last   ancientgreek.ImportWriteRequest
}

func (w *preparationWriterStub) WriteImportedAnalysis(_ context.Context, in ancientgreek.ImportWriteRequest) (ancientgreek.ImportWriteResult, error) {
	w.calls++
	w.last = in
	return w.result, w.err
}

func preparationFixture(t *testing.T) (domain.ClassicalPreparationTarget, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return domain.ClassicalPreparationTarget{
		OwnerID: "owner-1", BookID: "book-1", ReadingUnitKey: "synthetic-work-1",
		WorkTitle: "Synthetic Work", WorkAuthor: "Author", EditionLabel: "Edition",
		CitationStart: "1.1.1", CitationEnd: "1.1.2",
		SourceLayer: "layer-1", SourceFileName: "work.conllu",
		SourceFileSize: int64(len(data)), SourceFileSHA256: hex.EncodeToString(sum[:]),
		SourceFormat: classical.SourceFormatOGACoNLLU, InterpretationVersion: "oga-agdt-upos/5",
		DefinitionRevision: 1,
	}, data
}

func storedPreparationSource(target domain.ClassicalPreparationTarget, data []byte) domain.ClassicalSource {
	return domain.ClassicalSource{
		SHA256: target.SourceFileSHA256, Layer: target.SourceLayer,
		FileName: target.SourceFileName, Size: int64(len(data)), Content: data,
	}
}

func preparationJob(target domain.ClassicalPreparationTarget) *river.Job[ClassicalPreparationArgs] {
	return &river.Job[ClassicalPreparationArgs]{Args: ClassicalPreparationArgs{
		OwnerID: target.OwnerID, BookID: target.BookID,
		ReadingUnitKey: target.ReadingUnitKey, DefinitionRevision: target.DefinitionRevision,
	}}
}

func TestSubmitClassicalPreparationPreTransactionOutcomes(t *testing.T) {
	t.Parallel()
	target, _ := preparationFixture(t)
	cases := map[string]struct {
		service  *Service
		store    *preparationStoreStub
		owner    string
		bookID   string
		wantErr  string
		complete bool
	}{
		"not configured": {service: &Service{}, wantErr: "not configured"},
		"missing owner": {
			service: &Service{pool: &pgxpool.Pool{}, client: fakeRiverClient{}, classicalStore: &preparationStoreStub{target: target}},
			wantErr: "owner and Book are required",
		},
		"load error": {
			service: &Service{pool: &pgxpool.Pool{}, client: fakeRiverClient{}, classicalStore: &preparationStoreStub{loadErr: errors.New("load failed")}},
			owner:   "owner-1", bookID: "book-1", wantErr: "load failed",
		},
		"withdrawn": {
			service: &Service{pool: &pgxpool.Pool{}, client: fakeRiverClient{}, classicalStore: &preparationStoreStub{target: withWithdrawn(target)}},
			owner:   "owner-1", bookID: "book-1", wantErr: ErrWithdrawnClassicalDefinition.Error(),
		},
		"complete check error": {
			service: &Service{pool: &pgxpool.Pool{}, client: fakeRiverClient{}, classicalStore: &preparationStoreStub{target: target, completeErr: errors.New("check failed")}},
			owner:   "owner-1", bookID: "book-1", wantErr: "check failed",
		},
		"already complete": {
			service: &Service{pool: &pgxpool.Pool{}, client: fakeRiverClient{}, classicalStore: &preparationStoreStub{target: target, complete: true}},
			owner:   "owner-1", bookID: "book-1", complete: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			handle, err := tc.service.SubmitClassicalPreparation(context.Background(), tc.owner, tc.bookID)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.complete, handle.Complete)
		})
	}
}

func withWithdrawn(target domain.ClassicalPreparationTarget) domain.ClassicalPreparationTarget {
	target.Withdrawn = true
	return target
}

func TestClassicalPreparationWorkerRefusesMissingDependencies(t *testing.T) {
	t.Parallel()
	target, _ := preparationFixture(t)
	job := preparationJob(target)
	require.Error(t, (&ClassicalPreparationWorker{}).Work(context.Background(), job))
	require.Error(t, (&ClassicalPreparationWorker{Store: &preparationStoreStub{}}).Work(context.Background(), job))
}

func TestClassicalPreparationWorkerStoreOutcomes(t *testing.T) {
	t.Parallel()
	target, data := preparationFixture(t)
	withdrawn := target
	withdrawn.Withdrawn = true
	cases := map[string]struct {
		target domain.ClassicalPreparationTarget
		store  *preparationStoreStub
	}{
		"load error":         {target: target, store: &preparationStoreStub{loadErr: errors.New("load failed")}},
		"withdrawn":          {target: withdrawn, store: &preparationStoreStub{target: withdrawn}},
		"complete check err": {target: target, store: &preparationStoreStub{target: target, completeErr: errors.New("check failed")}},
		"already complete":   {target: target, store: &preparationStoreStub{target: target, complete: true}},
		"source read error":  {target: target, store: &preparationStoreStub{target: target, sourceErr: errors.New("source failed")}},
		"stored bytes bad":   {target: target, store: &preparationStoreStub{target: target, sourceFound: true, source: storedPreparationSource(target, append([]byte(nil), data...))}},
	}
	cases["stored bytes bad"].store.source.Content[0] = 'x'
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			worker := &ClassicalPreparationWorker{Store: tc.store, Writer: &preparationWriterStub{}, Config: ClassicalPreparationConfig{}}
			err := worker.Work(context.Background(), preparationJob(tc.target))
			if name == "already complete" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestClassicalPreparationWorkerDownloadRefusals(t *testing.T) {
	t.Parallel()
	target, data := preparationFixture(t)
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failingServer.Close)
	mismatchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(make([]byte, len(data))); err != nil {
			return
		}
	}))
	t.Cleanup(mismatchServer.Close)
	oversizeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(append(append([]byte(nil), data...), '!')); err != nil {
			return
		}
	}))
	t.Cleanup(oversizeServer.Close)
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(data); err != nil {
			return
		}
	}))
	t.Cleanup(successServer.Close)

	cases := map[string]struct {
		target domain.ClassicalPreparationTarget
		config ClassicalPreparationConfig
		code   string
	}{
		"unknown layer": {target: target, config: ClassicalPreparationConfig{}},
		"layer without url": {
			target: target,
			config: ClassicalPreparationConfig{Layers: classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: "://bad"}}}},
			code:   ancientgreek.CodeSourceLayerUnknown,
		},
		"oversize pin": {
			target: func() domain.ClassicalPreparationTarget {
				copied := target
				copied.SourceFileSize = maxClassicalSourceBytes + 1
				return copied
			}(),
			config: ClassicalPreparationConfig{Layers: classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: failingServer.URL + "/"}}}},
			code:   ancientgreek.CodeOversize,
		},
		"http failure": {
			target: target,
			config: ClassicalPreparationConfig{Layers: classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: failingServer.URL + "/"}}}},
			code:   ancientgreek.CodeSourceDownloadFailed,
		},
		"pin mismatch": {
			target: target,
			config: ClassicalPreparationConfig{Layers: classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: mismatchServer.URL + "/"}}}},
			code:   ancientgreek.CodePinMismatch,
		},
		"oversize body": {
			target: target,
			config: ClassicalPreparationConfig{Layers: classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: oversizeServer.URL + "/"}}}},
			code:   ancientgreek.CodeOversize,
		},
		"temp directory": {
			target: target,
			config: ClassicalPreparationConfig{
				Layers:  classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: successServer.URL + "/"}}},
				TempDir: filepath.Join(t.TempDir(), "missing"),
			},
			code: ancientgreek.CodeSourceDownloadFailed,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := &preparationStoreStub{target: tc.target}
			worker := &ClassicalPreparationWorker{Store: store, Writer: &preparationWriterStub{}, Config: tc.config}
			err := worker.Work(context.Background(), preparationJob(tc.target))
			require.Error(t, err)
			if tc.code == "" {
				return
			}
			var importErr *ancientgreek.ImportError
			require.ErrorAs(t, err, &importErr)
			assert.Equal(t, tc.code, importErr.Code)
		})
	}
}

func TestClassicalPreparationWorkerPublishesFromStoredSource(t *testing.T) {
	t.Parallel()
	target, data := preparationFixture(t)
	store := &preparationStoreStub{target: target, sourceFound: true, source: storedPreparationSource(target, data)}
	writer := &preparationWriterStub{result: ancientgreek.ImportWriteResult{RunID: "run-1"}}
	worker := &ClassicalPreparationWorker{Store: store, Writer: writer, Config: ClassicalPreparationConfig{}}

	require.NoError(t, worker.Work(context.Background(), preparationJob(target)))
	require.Equal(t, 1, writer.calls)
	assert.Equal(t, target.OwnerID, store.record.OwnerID)
	assert.Equal(t, target.BookID, store.record.BookID)
	assert.Equal(t, target.SourceFileSHA256, store.record.SourceFileSHA256)
	assert.Equal(t, "run-1", store.record.AnalysisRunID)
	assert.Empty(t, store.putCalls, "a stored source is reused, not re-inserted")
	assert.Equal(t, domain.ClassicalUnitID(target.ReadingUnitKey), writer.last.UnitID)
	assert.Equal(t, []byte(writer.last.FullText), writer.last.SourceBytes)
	assert.NotContains(t, writer.last.FullText, "Ἀρταξέρξης")

	// The parsed work is reused for the next job with the same pin and version.
	require.NoError(t, worker.Work(context.Background(), preparationJob(target)))
	assert.Equal(t, 2, writer.calls)
}

func TestClassicalPreparationWorkerParseAndWriteErrors(t *testing.T) {
	t.Parallel()
	target, data := preparationFixture(t)
	cases := map[string]struct {
		target domain.ClassicalPreparationTarget
		writer *preparationWriterStub
		record error
	}{
		"unknown source format": {
			target: func() domain.ClassicalPreparationTarget {
				copied := target
				copied.SourceFormat = "graded-reader"
				return copied
			}(),
		},
		"invalid citation": {
			target: func() domain.ClassicalPreparationTarget {
				copied := target
				copied.CitationStart = "not-a-citation"
				return copied
			}(),
		},
		"writer error": {
			target: target, writer: &preparationWriterStub{err: errors.New("write failed")},
		},
		"record error": {
			target: target, writer: &preparationWriterStub{result: ancientgreek.ImportWriteResult{RunID: "run-1"}}, record: errors.New("record failed"),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := &preparationStoreStub{target: tc.target, sourceFound: true, source: storedPreparationSource(tc.target, data), recordErr: tc.record}
			writer := tc.writer
			if writer == nil {
				writer = &preparationWriterStub{}
			}
			worker := &ClassicalPreparationWorker{Store: store, Writer: writer, Config: ClassicalPreparationConfig{}}
			require.Error(t, worker.Work(context.Background(), preparationJob(tc.target)))
		})
	}
}

func TestClassicalPreparationWorkerDownloadsAndStores(t *testing.T) {
	t.Parallel()
	target, data := preparationFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(data); err != nil {
			return
		}
	}))
	t.Cleanup(server.Close)
	store := &preparationStoreStub{target: target}
	writer := &preparationWriterStub{result: ancientgreek.ImportWriteResult{RunID: "run-1"}}
	worker := &ClassicalPreparationWorker{
		Store: store, Writer: writer,
		Config: ClassicalPreparationConfig{
			Layers:     classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: server.URL + "/"}}},
			HTTPClient: server.Client(), TempDir: t.TempDir(),
		},
	}
	require.NoError(t, worker.Work(context.Background(), preparationJob(target)))
	assert.Equal(t, 1, store.putCalls, "a downloaded source is stored once")
	assert.Equal(t, 1, writer.calls)
}

func TestClassicalPreparationWorkerPutSourceFailure(t *testing.T) {
	t.Parallel()
	target, data := preparationFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(data); err != nil {
			return
		}
	}))
	t.Cleanup(server.Close)
	store := &preparationStoreStub{target: target, putErr: errors.New("insert failed")}
	worker := &ClassicalPreparationWorker{
		Store: store, Writer: &preparationWriterStub{},
		Config: ClassicalPreparationConfig{
			Layers:     classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: server.URL + "/"}}},
			HTTPClient: server.Client(), TempDir: t.TempDir(),
		},
	}
	err := worker.Work(context.Background(), preparationJob(target))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert failed")
}

func TestClassicalPreparationWorkerDownloadTransportFailure(t *testing.T) {
	t.Parallel()
	target, _ := preparationFixture(t)
	store := &preparationStoreStub{target: target}
	worker := &ClassicalPreparationWorker{
		Store: store, Writer: &preparationWriterStub{},
		Config: ClassicalPreparationConfig{
			Layers:     classical.Layers{Layers: []classical.Layer{{Key: "layer-1", BaseURL: "https://example.test/"}}},
			HTTPClient: &http.Client{Transport: errorTransport{}},
		},
	}
	err := worker.Work(context.Background(), preparationJob(target))
	var importErr *ancientgreek.ImportError
	require.ErrorAs(t, err, &importErr)
	assert.Equal(t, ancientgreek.CodeSourceDownloadFailed, importErr.Code)
}

type errorTransport struct{}

func (errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("transport failed")
}

func TestClassicalSourceURLRejectsInvalidBase(t *testing.T) {
	t.Parallel()
	_, err := classicalSourceURL("not-a-url", strings.Repeat("x", 1))
	require.Error(t, err)
	_, err = classicalSourceURL("https://[::1", "work.conllu")
	require.Error(t, err)
}
