package analysis

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type acquisitionStoreStub struct {
	book       domain.MyBook
	bookErr    error
	current    domain.CurrentReading
	currentErr error
	records    []domain.EPUBAcquisitionFailureRecord
	recordErr  error
	clears     []string
	clearErr   error
}

func (s *acquisitionStoreStub) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return s.book, s.bookErr
}

func (s *acquisitionStoreStub) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return s.current, s.currentErr
}

func (s *acquisitionStoreStub) RecordEPUBAcquisitionFailure(_ context.Context, record domain.EPUBAcquisitionFailureRecord) error {
	s.records = append(s.records, record)
	return s.recordErr
}

func (s *acquisitionStoreStub) ClearEPUBAcquisitionFailure(_ context.Context, owner, bookID string) error {
	s.clears = append(s.clears, owner+"/"+bookID)
	return s.clearErr
}

type acquisitionInserterStub struct {
	result *rivertype.JobInsertResult
	err    error
	args   river.JobArgs
	opts   *river.InsertOpts
}

func (s *acquisitionInserterStub) Insert(_ context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	s.args, s.opts = args, opts
	return s.result, s.err
}

type acquisitionAnalysisStub struct {
	calls    int
	sourceID string
	err      error
}

func (s *acquisitionAnalysisStub) SubmitToReadBookAnalysis(_ context.Context, _, _, sourceID string) (Handle, error) {
	s.calls++
	s.sourceID = sourceID
	return Handle{ID: 42}, s.err
}

func TestEPUBAcquisitionArgsKindAndTimeout(t *testing.T) {
	t.Parallel()
	assert.Equal(t, EPUBAcquisitionQueue, (EPUBAcquisitionArgs{}).Kind())
	worker := &EPUBAcquisitionWorker{}
	assert.Equal(t, epubAcquisitionTimeout, worker.Timeout(nil))
}

func TestSubmitEPUBAcquisitionRefusesAnUnconfiguredOrIncompleteSubmission(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		service  *Service
		owner    string
		bookID   string
		inserter *acquisitionInserterStub
		wantErr  string
	}{
		{name: "nil service", service: nil, owner: "owner", bookID: "book"},
		{name: "no inserter", service: NewService(nil, nil), owner: "owner", bookID: "book"},
		{
			name: "missing owner", service: &Service{acquisitionInserter: &acquisitionInserterStub{}},
			owner: " ", bookID: "book",
		},
		{
			name: "missing Book", service: &Service{acquisitionInserter: &acquisitionInserterStub{}},
			owner: "owner", bookID: " ",
		},
		{
			name:    "insert failed",
			service: &Service{acquisitionInserter: &acquisitionInserterStub{err: errors.New("river down")}},
			owner:   "owner", bookID: "book", wantErr: "river down",
		},
		{
			name:    "no job returned",
			service: &Service{acquisitionInserter: &acquisitionInserterStub{}},
			owner:   "owner", bookID: "book", wantErr: "River returned no job",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.service.SubmitEPUBAcquisition(context.Background(), tt.owner, tt.bookID)
			require.Error(t, err)
			if tt.wantErr != "" {
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestSubmitEPUBAcquisitionCarriesOwnerAndBookOnce(t *testing.T) {
	t.Parallel()
	inserter := &acquisitionInserterStub{result: &rivertype.JobInsertResult{Job: &rivertype.JobRow{ID: 91}}}
	service := &Service{acquisitionInserter: inserter}

	handle, err := service.SubmitEPUBAcquisition(context.Background(), "owner", "book")

	require.NoError(t, err)
	assert.Equal(t, int64(91), handle.JobID)
	args, ok := inserter.args.(EPUBAcquisitionArgs)
	require.True(t, ok)
	assert.Equal(t, "owner", args.OwnerID)
	assert.Equal(t, "book", args.BookID)
	require.NotNil(t, inserter.opts)
	assert.Equal(t, EPUBAcquisitionQueue, inserter.opts.Queue)
	assert.Equal(t, 1, inserter.opts.MaxAttempts)
}

func TestEPUBAcquisitionWorkerRefusesMissingCollaborators(t *testing.T) {
	t.Parallel()
	job := &river.Job[EPUBAcquisitionArgs]{}
	tests := []struct {
		name   string
		worker *EPUBAcquisitionWorker
	}{
		{name: "nil worker", worker: nil},
		{name: "store", worker: &EPUBAcquisitionWorker{}},
		{name: "catalogue", worker: &EPUBAcquisitionWorker{Store: &acquisitionStoreStub{}}},
		{name: "analysis", worker: &EPUBAcquisitionWorker{Store: &acquisitionStoreStub{}, FindTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			return cataloguesync.AcquisitionTarget{}, nil
		}, Acquire: func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
			return epub.ImportResult{}, nil
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, tt.worker.Work(context.Background(), job))
		})
	}
}

func TestEPUBAcquisitionWorkerSkipBranches(t *testing.T) {
	t.Parallel()
	toRead := domain.MyBook{Book: domain.Book{ID: "book", LanguageTag: "de"}, Disposition: domain.BookDispositionToRead}
	tests := []struct {
		name      string
		store     *acquisitionStoreStub
		wantSkip  bool
		wantError bool
	}{
		{name: "missing Book", store: &acquisitionStoreStub{bookErr: domain.ErrNotFound}, wantSkip: true},
		{name: "detail read failed", store: &acquisitionStoreStub{bookErr: errors.New("database down")}, wantError: true},
		{name: "not To Read", store: &acquisitionStoreStub{book: domain.MyBook{Book: domain.Book{ID: "book"}, Disposition: domain.BookDispositionInbox}}, wantSkip: true},
		{name: "no language", store: &acquisitionStoreStub{book: domain.MyBook{Book: domain.Book{ID: "book"}, Disposition: domain.BookDispositionToRead}}},
		{name: "current read failed", store: &acquisitionStoreStub{book: toRead, currentErr: errors.New("database down")}, wantError: true},
		{name: "current Book", store: &acquisitionStoreStub{book: toRead, current: domain.CurrentReading{BookID: "book"}}, wantSkip: true},
		{name: "another current Book", store: &acquisitionStoreStub{book: toRead, current: domain.CurrentReading{BookID: "other"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worker := &EPUBAcquisitionWorker{Store: tt.store}
			skip, err := worker.skip(context.Background(), "owner", "book")
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSkip, skip)
		})
	}
}

func TestEPUBAcquisitionWorkerAcquireBranches(t *testing.T) {
	t.Parallel()
	target := cataloguesync.AcquisitionTarget{
		ConnectionID: "connection", Language: "de", Entry: opds.Entry{ID: "entry"},
	}
	tests := []struct {
		name       string
		findTarget func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
		acquire    func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
		store      *acquisitionStoreStub
		wantRecord bool
		wantClears int
	}{
		{
			name: "target lookup failed",
			findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
				return cataloguesync.AcquisitionTarget{}, errors.New("index read failed")
			},
		},
		{
			name: "missing entry",
			findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
				return target, cataloguesync.ErrCatalogEntryMissing
			},
			store: &acquisitionStoreStub{}, wantRecord: true,
		},
		{
			name: "download failed",
			findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
				return target, nil
			},
			acquire: func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
				return epub.ImportResult{}, opds.ErrDownloadTooLarge
			},
			store: &acquisitionStoreStub{}, wantRecord: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := tt.store
			if store == nil {
				store = &acquisitionStoreStub{}
			}
			worker := &EPUBAcquisitionWorker{
				Store: store, FindTarget: tt.findTarget, Acquire: tt.acquire,
				Analysis: &acquisitionAnalysisStub{},
			}
			require.Error(t, worker.acquireAndSubmit(context.Background(), "owner", "book"))
			assert.Equal(t, tt.wantRecord, len(store.records) == 1)
			assert.Len(t, store.clears, tt.wantClears)
		})
	}
}

func TestEPUBAcquisitionWorkerSubmitAnalysisBranches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		store     *acquisitionStoreStub
		analysis  *acquisitionAnalysisStub
		wantError bool
	}{
		{name: "reload failed", store: &acquisitionStoreStub{bookErr: errors.New("database down")}, wantError: true},
		{name: "no current source", store: &acquisitionStoreStub{}, wantError: true},
		{name: "analysis failed", store: &acquisitionStoreStub{book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source"}}}}, analysis: &acquisitionAnalysisStub{err: errors.New("analysis unavailable")}, wantError: true},
		{name: "submitted", store: &acquisitionStoreStub{book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source"}}}}, analysis: &acquisitionAnalysisStub{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.analysis == nil {
				tt.analysis = &acquisitionAnalysisStub{}
			}
			worker := &EPUBAcquisitionWorker{Store: tt.store, Analysis: tt.analysis}
			err := worker.submitAnalysis(context.Background(), "owner", "book")
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, tt.analysis.calls)
			assert.Equal(t, "source", tt.analysis.sourceID)
		})
	}
}

func TestEPUBAcquisitionWorkerRecordsAWriteFailure(t *testing.T) {
	t.Parallel()
	store := &acquisitionStoreStub{recordErr: errors.New("database down")}
	worker := &EPUBAcquisitionWorker{Store: store}

	worker.recordFailure(context.Background(), "owner", "book", cataloguesync.AcquisitionTarget{
		ConnectionID: "connection", Entry: opds.Entry{ID: "entry"},
	}, opds.ErrNoEPUB)

	require.Len(t, store.records, 1)
	assert.Equal(t, domain.BookPreparationStageSource, store.records[0].Stage)
	assert.Equal(t, domain.BookPreparationCodeSourceFormatUnknown, store.records[0].Code)
}

func TestClassifyAcquisitionFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err   error
		stage domain.BookPreparationStage
		code  domain.BookPreparationCode
	}{
		{err: cataloguesync.ErrCatalogEntryMissing, stage: domain.BookPreparationStageEntry, code: domain.BookPreparationCodeCatalogEntryMissing},
		{err: opds.ErrDownloadTooLarge, stage: domain.BookPreparationStageSource, code: domain.BookPreparationCodeSourceOversize},
		{err: opds.ErrRetainedEPUBTooLarge, stage: domain.BookPreparationStageSource, code: domain.BookPreparationCodeSourceOversize},
		{err: opds.ErrNoEPUB, stage: domain.BookPreparationStageSource, code: domain.BookPreparationCodeSourceFormatUnknown},
		{err: epub.ErrInvalidEPUB, stage: domain.BookPreparationStageSource, code: domain.BookPreparationCodeSourceFormatUnknown},
		{err: opds.ErrIncompatibleMediaType, stage: domain.BookPreparationStageSource, code: domain.BookPreparationCodeSourceFormatUnknown},
		{err: opds.ErrImportFailed, stage: domain.BookPreparationStageImport, code: domain.BookPreparationCodeEPUBImportFailed},
		{err: errors.New("catalog unreachable"), stage: domain.BookPreparationStageFetch, code: domain.BookPreparationCodeSourceDownloadFailed},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			t.Parallel()
			stage, code := classifyAcquisitionFailure(tt.err)
			assert.Equal(t, tt.stage, stage)
			assert.Equal(t, tt.code, code)
		})
	}
}
