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

// catalogDigestStore records every digest the acquisition lookup tries and
// resolves only the configured one, as a revision stored before slimming would.
type catalogDigestStore struct {
	memoryStore
	existing  domain.SourceMaterial
	foundHash string
	lookupErr error
	linkErr   error
	requested []string
}

func (m *catalogDigestStore) FindSourceMaterialForAcquisition(_ context.Context, _, _, contentHash string) (domain.SourceMaterial, bool, error) {
	m.requested = append(m.requested, contentHash)
	if m.lookupErr != nil {
		return domain.SourceMaterial{}, false, m.lookupErr
	}
	if contentHash == m.foundHash {
		return m.existing, true, nil
	}
	return domain.SourceMaterial{}, false, nil
}

func (m *catalogDigestStore) LinkSourceToBook(_ context.Context, _, _, _ string) error {
	if m.linkErr != nil {
		return m.linkErr
	}
	m.linked++
	return nil
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
	result, err := NewService(store).ImportForAcquisition(context.Background(), "owner-id", "de", AcquisitionContent{Content: fixture(t)})
	require.NoError(t, err)
	assert.True(t, result.AlreadyPresent)
	assert.Equal(t, "existing-source", result.Source.ID)
	assert.Equal(t, 0, store.puts)
	assert.Equal(t, 0, store.histories)
	assert.Equal(t, 1, store.resolved)
	assert.Equal(t, 1, store.linked)
}

func TestImportForAcquisitionMatchesEitherTheRetainedOrCatalogDigest(t *testing.T) {
	content := fixture(t)
	catalogDigest := domain.EPUBContentDigest([]byte("catalog file before slimming"))
	store := &catalogDigestStore{
		existing:  domain.SourceMaterial{ID: "existing-source", OwnerID: "owner-id", SourceIdentifier: "existing-source", Title: "Existing"},
		foundHash: catalogDigest,
	}
	result, err := NewService(store).ImportForAcquisition(context.Background(), "owner-id", "de", AcquisitionContent{Content: content, CatalogDigest: catalogDigest})
	require.NoError(t, err)
	assert.True(t, result.AlreadyPresent)
	assert.Equal(t, "existing-source", result.Source.ID)
	assert.Equal(t, []string{ContentDigest(content), catalogDigest}, store.requested, "the lookup must try the Retained EPUB digest before the Catalog file digest")
	assert.Zero(t, store.puts, "the existing content revision was duplicated")
}

func TestImportForAcquisitionLooksUpAnIdenticalCatalogDigestOnce(t *testing.T) {
	content := fixture(t)
	digest := ContentDigest(content)
	store := &catalogDigestStore{foundHash: "never"}
	_, err := NewService(store).ImportForAcquisition(context.Background(), "owner-id", "de", AcquisitionContent{Content: content, CatalogDigest: digest})
	require.NoError(t, err)
	assert.Equal(t, []string{digest}, store.requested)
}

func TestImportForAcquisitionPropagatesLookupAndLinkFailures(t *testing.T) {
	content := fixture(t)
	t.Run("lookup failure", func(t *testing.T) {
		store := &catalogDigestStore{lookupErr: errors.New("database unavailable")}
		_, err := NewService(store).ImportForAcquisition(context.Background(), "owner-id", "de", AcquisitionContent{Content: content})
		require.ErrorContains(t, err, "look up existing source material")
		assert.Zero(t, store.puts, "a failed idempotency lookup was ignored")
	})
	t.Run("link failure", func(t *testing.T) {
		store := &catalogDigestStore{
			foundHash: ContentDigest(content),
			existing:  domain.SourceMaterial{ID: "existing-source", OwnerID: "owner-id", SourceIdentifier: "existing-source", Title: "Existing"},
			linkErr:   errors.New("link failed"),
		}
		_, err := NewService(store).ImportForAcquisition(context.Background(), "owner-id", "de", AcquisitionContent{Content: content})
		require.ErrorContains(t, err, "link source existing-source to book")
		assert.Zero(t, store.puts, "reused content was imported again after a link failure")
	})
}

func TestImportForAcquisitionWithoutARevisionLookupImportsNormally(t *testing.T) {
	store := &memoryStore{}
	result, err := NewService(store).ImportForAcquisition(context.Background(), "owner-id", "de", AcquisitionContent{Content: fixture(t)})
	require.NoError(t, err)
	assert.False(t, result.AlreadyPresent)
	assert.Equal(t, 1, store.puts)
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
	_, err := NewService(store).WithSourceStructureCheckEnqueuer(checks).ImportForAcquisition(context.Background(), "owner-id", "de", AcquisitionContent{Content: fixture(t)})
	require.NoError(t, err)
	assert.Empty(t, checks.calls, "reusing unchanged evidence is not a source-evidence change")
}
