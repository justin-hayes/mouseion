package sourcecheck

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func activeTarget() Target {
	return Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet}
}

type fakeStore struct {
	source        domain.SourceMaterial
	sourceErr     error
	current       domain.ExtractedSnapshot
	currentErr    error
	check         domain.SourceStructureCheck
	checkFound    bool
	checkErr      error
	recordErr     error
	records       []domain.SourceStructureCheck
	candidates    []domain.SourceStructureCheckKey
	candidatesErr error
	candidateFunc func(after string, limit int) ([]domain.SourceStructureCheckKey, error)
	candidateArgs []string
}

func (s *fakeStore) GetSourceMaterial(context.Context, string, string) (domain.SourceMaterial, error) {
	return s.source, s.sourceErr
}

func (s *fakeStore) GetCurrentExtractedSnapshot(context.Context, string, string) (domain.ExtractedSnapshot, error) {
	return s.current, s.currentErr
}

func (s *fakeStore) GetSourceStructureCheck(context.Context, domain.SourceStructureCheckKey) (domain.SourceStructureCheck, bool, error) {
	return s.check, s.checkFound, s.checkErr
}

func (s *fakeStore) RecordSourceStructureCheck(_ context.Context, check domain.SourceStructureCheck) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	s.records = append(s.records, check)
	return nil
}

func (s *fakeStore) ListSourceStructureCheckCandidates(_ context.Context, _ int, _ string, after string, limit int) ([]domain.SourceStructureCheckKey, error) {
	s.candidateArgs = append(s.candidateArgs, after)
	if s.candidateFunc != nil {
		return s.candidateFunc(after, limit)
	}
	return s.candidates, s.candidatesErr
}

type fakeClient struct {
	inserted []CheckArgs
	err      error
}

func (c *fakeClient) Insert(_ context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if c.err != nil {
		return nil, c.err
	}
	check, ok := args.(CheckArgs)
	if !ok {
		return nil, errors.New("unexpected job args")
	}
	c.inserted = append(c.inserted, check)
	return &rivertype.JobInsertResult{}, nil
}

func validSource() domain.SourceMaterial {
	return domain.SourceMaterial{ID: "source", OwnerID: "owner", MediaType: epub.MediaType(), ContentRevisionID: "revision", Content: []byte("epub")}
}

func validCurrent() domain.ExtractedSnapshot {
	return domain.ExtractedSnapshot{
		SnapshotID: "snapshot", ContentRevisionID: "revision",
		Units: domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: []domain.ExtractedUnit{{ID: "unit", Text: "text", EndOffset: 4}}},
	}
}

func validCheckArgs() CheckArgs {
	return CheckArgs{
		OwnerID: "owner", SourceMaterialID: "source", ContentRevisionID: "revision", SnapshotID: "snapshot",
		TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet,
	}
}

func runWorker(t *testing.T, ctx context.Context, worker *Worker, args CheckArgs) error {
	t.Helper()
	return worker.Work(ctx, &river.Job[CheckArgs]{Args: args})
}

func TestAddWorkerRegisters(t *testing.T) {
	workers := river.NewWorkers()
	worker := AddWorker(workers, &fakeStore{})
	require.NotNil(t, worker)
	require.NotNil(t, worker.Store)
}

func TestServiceEnqueueSourceStructureCheckOutcomes(t *testing.T) {
	storeErr := errors.New("source store failed")
	snapshotErr := errors.New("snapshot store failed")
	checkErr := errors.New("check store failed")
	insertErr := errors.New("insert failed")

	tests := []struct {
		name       string
		store      *fakeStore
		client     *fakeClient
		wantErr    string
		wantInsert int
	}{
		{name: "missing source is ignored", store: &fakeStore{sourceErr: domain.ErrNotFound}, client: &fakeClient{}},
		{name: "source error propagates", store: &fakeStore{sourceErr: storeErr}, client: &fakeClient{}, wantErr: "load source"},
		{name: "imported source is excluded", store: &fakeStore{source: domain.SourceMaterial{ID: "source", MediaType: domain.ImportedSourceMediaType}}, client: &fakeClient{}},
		{name: "missing snapshot is ignored", store: &fakeStore{source: validSource(), currentErr: domain.ErrExtractedUnitsUnavailable}, client: &fakeClient{}},
		{name: "snapshot error propagates", store: &fakeStore{source: validSource(), currentErr: snapshotErr}, client: &fakeClient{}, wantErr: "load current snapshot"},
		{name: "current target needs no work", store: &fakeStore{source: validSource(), current: domain.ExtractedSnapshot{Units: domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV2}, Structure: domain.SnapshotStructure{ConventionSet: domain.StructureConventionSet}}}, client: &fakeClient{}},
		{name: "new evidence is enqueued", store: &fakeStore{source: validSource(), current: validCurrent()}, client: &fakeClient{}, wantInsert: 1},
		{name: "successful exact key is reused", store: &fakeStore{source: validSource(), current: validCurrent(), checkFound: true}, client: &fakeClient{}},
		{name: "check lookup error propagates", store: &fakeStore{source: validSource(), current: validCurrent(), checkErr: checkErr}, client: &fakeClient{}, wantErr: "load existing source structure check"},
		{name: "insert error propagates", store: &fakeStore{source: validSource(), current: validCurrent()}, client: &fakeClient{err: insertErr}, wantErr: "enqueue source structure check"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(test.store, test.client, activeTarget())
			err := service.EnqueueSourceStructureCheck(context.Background(), "owner", "source")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, test.client.inserted, test.wantInsert)
			if test.wantInsert > 0 {
				assert.Equal(t, "source", test.client.inserted[0].SourceMaterialID)
				assert.Equal(t, "snapshot", test.client.inserted[0].SnapshotID)
			}
		})
	}
}

func TestServiceStartupScanPaginationAndErrors(t *testing.T) {
	inactive := NewService(&fakeStore{}, &fakeClient{}, Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV1})
	require.NoError(t, inactive.EnqueueStartupChecks(context.Background()))

	require.ErrorContains(t, NewService(nil, nil, activeTarget()).EnqueueStartupChecks(context.Background()), "not configured")

	storeErr := errors.New("scan failed")
	require.ErrorContains(t, NewService(&fakeStore{candidatesErr: storeErr}, &fakeClient{}, activeTarget()).EnqueueStartupChecks(context.Background()), "scan source structure check candidates")

	key := func(id string) domain.SourceStructureCheckKey {
		return domain.SourceStructureCheckKey{
			OwnerID: "owner", SourceMaterialID: id, ContentRevisionID: "revision", SnapshotID: "snapshot-" + id,
			TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet,
		}
	}
	paged := &fakeStore{candidateFunc: func(after string, _ int) ([]domain.SourceStructureCheckKey, error) {
		switch after {
		case "00000000-0000-0000-0000-000000000000":
			return []domain.SourceStructureCheckKey{key("a"), key("b")}, nil
		case "b":
			return []domain.SourceStructureCheckKey{key("c")}, nil
		default:
			return nil, nil
		}
	}}
	client := &fakeClient{}
	service := NewService(paged, client, activeTarget())
	service.pageSize = 2
	require.NoError(t, service.EnqueueStartupChecks(context.Background()))
	assert.Len(t, client.inserted, 3)
	assert.Equal(t, []string{"00000000-0000-0000-0000-000000000000", "b"}, paged.candidateArgs)

	checkErr := errors.New("check failed")
	require.ErrorContains(t, NewService(&fakeStore{candidates: []domain.SourceStructureCheckKey{key("a")}, checkErr: checkErr}, &fakeClient{}, activeTarget()).EnqueueStartupChecks(context.Background()), "load existing source structure check")

	insertErr := errors.New("insert failed")
	require.ErrorContains(t, NewService(&fakeStore{candidates: []domain.SourceStructureCheckKey{key("a")}}, &fakeClient{err: insertErr}, activeTarget()).EnqueueStartupChecks(context.Background()), "enqueue source structure check")

	defaultPage := NewService(&fakeStore{candidates: []domain.SourceStructureCheckKey{key("a")}}, &fakeClient{}, activeTarget())
	defaultPage.pageSize = 0
	require.NoError(t, defaultPage.EnqueueStartupChecks(context.Background()))
}

func extractionFixture(text, convention string) epub.Extraction {
	return epub.Extraction{
		Book: epub.ExtractedBook{ExtractedUnits: domain.ExtractedUnits{
			SchemaVersion: domain.ExtractedUnitsSchemaVersionV2,
			Units:         []domain.ExtractedUnit{{ID: "unit", Text: text, EndOffset: uint64(len([]rune(text)))}},
		}},
		Structure: domain.SnapshotStructure{Recorded: true, ConventionSet: convention},
	}
}

func TestWorkerSkipsObsoleteAndInertWork(t *testing.T) {
	require.ErrorContains(t, runWorker(t, context.Background(), &Worker{}, validCheckArgs()), "worker store is required")

	inactive := validCheckArgs()
	inactive.TargetContractVersion = domain.ExtractedUnitsSchemaVersionV1
	require.NoError(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{}}, inactive))

	invalid := validCheckArgs()
	invalid.OwnerID = ""
	require.ErrorContains(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{}}, invalid), "identity is incomplete")

	checkErr := errors.New("check lookup failed")
	require.ErrorContains(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{checkErr: checkErr}}, validCheckArgs()), "load source structure check")
	require.NoError(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{checkFound: true}}, validCheckArgs()))

	require.NoError(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{sourceErr: domain.ErrNotFound}}, validCheckArgs()))
	sourceErr := errors.New("source load failed")
	require.ErrorContains(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{sourceErr: sourceErr}}, validCheckArgs()), "load source")

	imported := validSource()
	imported.MediaType = domain.ImportedSourceMediaType
	require.NoError(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{source: imported, current: validCurrent()}}, validCheckArgs()))

	changedRevision := validSource()
	changedRevision.ContentRevisionID = "newer"
	require.NoError(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{source: changedRevision, current: validCurrent()}}, validCheckArgs()))

	require.NoError(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{source: validSource(), currentErr: domain.ErrExtractedUnitsUnavailable}}, validCheckArgs()))
	currentErr := errors.New("snapshot load failed")
	require.ErrorContains(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{source: validSource(), currentErr: currentErr}}, validCheckArgs()), "load current snapshot")

	obsolete := validCurrent()
	obsolete.SnapshotID = "other"
	require.NoError(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{source: validSource(), current: obsolete}}, validCheckArgs()))

	currentTarget := validCurrent()
	currentTarget.Units.SchemaVersion = domain.ExtractedUnitsSchemaVersionV2
	currentTarget.Structure.ConventionSet = domain.StructureConventionSet
	require.NoError(t, runWorker(t, context.Background(), &Worker{Store: &fakeStore{source: validSource(), current: currentTarget}}, validCheckArgs()))
}

func TestWorkerEvaluationFailures(t *testing.T) {
	extractErr := errors.New("invalid retained bytes")
	args := validCheckArgs()
	require.ErrorContains(t, runWorker(t, context.Background(), &Worker{
		Store: &fakeStore{source: validSource(), current: validCurrent()},
		Extract: func([]byte) (epub.Extraction, error) {
			return epub.Extraction{}, extractErr
		},
	}, args), "extract retained source")

	wrongConvention := extractionFixture("text", "epub-structure/other")
	require.ErrorContains(t, runWorker(t, context.Background(), &Worker{
		Store:   &fakeStore{source: validSource(), current: validCurrent()},
		Extract: func([]byte) (epub.Extraction, error) { return wrongConvention, nil },
	}, args), "extractor produced convention set")

	recordErr := errors.New("record failed")
	store := &fakeStore{source: validSource(), current: validCurrent(), recordErr: recordErr}
	require.ErrorContains(t, runWorker(t, context.Background(), &Worker{
		Store: store,
		Extract: func([]byte) (epub.Extraction, error) {
			return extractionFixture("text", domain.StructureConventionSet), nil
		},
	}, args), "record failed")
}
