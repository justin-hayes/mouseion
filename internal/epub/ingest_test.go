package epub

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	source    domain.SourceMaterial
	history   domain.ProcessingHistory
	units     ExtractedUnits
	puts      int
	histories int
	bookID    string
	resolved  int
	linked    int
}

func (m *memoryStore) ResolveOrCreateBookForAcquisition(_ context.Context, _, _, _, _ string) (string, error) {
	m.resolved++
	if m.bookID == "" {
		m.bookID = "book-id"
	}
	return m.bookID, nil
}
func (m *memoryStore) LinkSourceToBook(_ context.Context, _, _, _ string) error {
	m.linked++
	return nil
}

func (m *memoryStore) GetExtractedUnitSnapshot(_ context.Context, _, _ string) (string, domain.ExtractedUnits, error) {
	return "snapshot:test", m.units, nil
}
func (m *memoryStore) PutSourceMaterialWithExtractedSnapshot(_ context.Context, v domain.SourceMaterial, units ExtractedUnits, structure domain.SnapshotStructure) (domain.SourceMaterial, error) {
	m.puts++
	v.ID = "source-id"
	m.source = v
	m.units = units
	return v, nil
}
func (m *memoryStore) PutProcessingHistory(_ context.Context, v domain.ProcessingHistory) (domain.ProcessingHistory, error) {
	m.histories++
	v.ID = "history-id"
	m.history = v
	return v, nil
}

type acquisitionMemoryStore struct {
	memoryStore
	existing domain.SourceMaterial
}

func (m *acquisitionMemoryStore) FindSourceMaterialForAcquisition(context.Context, string, string, string) (domain.SourceMaterial, bool, error) {
	return m.existing, true, nil
}

type recordingStructureChecks struct {
	calls [][2]string
	err   error
}

func (r *recordingStructureChecks) EnqueueSourceStructureCheck(_ context.Context, ownerID, sourceMaterialID string) error {
	r.calls = append(r.calls, [2]string{ownerID, sourceMaterialID})
	return r.err
}

func TestImportPersistsOwnerScopedArtifactsAndHistory(t *testing.T) {
	store := &memoryStore{}
	result, err := NewService(store).Import(context.Background(), "owner-id", "de", fixture(t))
	require.NoError(t, err)
	assert.Equal(t, "owner-id", result.Source.OwnerID)
	assert.Equal(t, "owner-id", result.History.OwnerID)
	assert.Equal(t, 1, store.resolved)
	assert.Equal(t, 1, store.linked)
	assert.Equal(t, MediaType(), result.Source.MediaType)
	assert.NotEmpty(t, result.Source.Content)
	assert.NotEmpty(t, result.Source.FullText)
	require.NoError(t, store.units.ValidateOffsets(result.Source.FullText))
	assert.Equal(t, "epub.import", result.History.Operation)
	assert.Equal(t, "complete", result.History.Status)
	assert.NotEmpty(t, result.History.Details)
	assert.Equal(t, ContentDigest(fixture(t)), result.Source.ContentHash)
}

func TestImportRequiresAuthenticatedOwner(t *testing.T) {
	_, err := NewService(&memoryStore{}).Import(context.Background(), "", "de", fixture(t))
	assert.ErrorIs(t, err, ErrUnauthenticated)
}

func TestImportForAcquisitionReturnsExistingSourceWithoutRewritingIt(t *testing.T) {
	store := &acquisitionMemoryStore{existing: domain.SourceMaterial{ID: "existing-source", OwnerID: "owner-id", SourceIdentifier: "existing-source", Title: "Existing"}}
	result, err := NewService(store).ImportForAcquisition(context.Background(), "owner-id", "de", fixture(t))
	require.NoError(t, err)
	assert.True(t, result.AlreadyPresent)
	assert.Equal(t, "existing-source", result.Source.ID)
	assert.Equal(t, 0, store.puts)
	assert.Equal(t, 0, store.histories)
	assert.Equal(t, 1, store.resolved)
	assert.Equal(t, 1, store.linked)
}

func TestImportEnqueuesSourceStructureCheckAfterEvidenceChange(t *testing.T) {
	store := &memoryStore{}
	checks := &recordingStructureChecks{}
	result, err := NewService(store).WithSourceStructureCheckEnqueuer(checks).Import(context.Background(), "owner-id", "de", fixture(t))
	require.NoError(t, err)
	require.Len(t, checks.calls, 1)
	assert.Equal(t, [2]string{"owner-id", result.Source.ID}, checks.calls[0])
}

func TestImportSucceedsWhenStructureCheckSchedulingFails(t *testing.T) {
	store := &memoryStore{}
	checks := &recordingStructureChecks{err: errors.New("river unavailable")}
	result, err := NewService(store).WithSourceStructureCheckEnqueuer(checks).Import(context.Background(), "owner-id", "de", fixture(t))
	require.NoError(t, err, "an already-committed import must not fail because check scheduling failed")
	assert.NotEmpty(t, result.Source.ID)
	assert.Len(t, checks.calls, 1)
}

func TestAcquisitionReuseDoesNotEnqueueAnotherStructureCheck(t *testing.T) {
	store := &acquisitionMemoryStore{existing: domain.SourceMaterial{ID: "existing-source", OwnerID: "owner-id", SourceIdentifier: "existing-source", Title: "Existing"}}
	checks := &recordingStructureChecks{}
	_, err := NewService(store).WithSourceStructureCheckEnqueuer(checks).ImportForAcquisition(context.Background(), "owner-id", "de", fixture(t))
	require.NoError(t, err)
	assert.Empty(t, checks.calls, "reusing unchanged evidence is not a source-evidence change")
}
