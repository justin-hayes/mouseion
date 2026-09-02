package epub

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
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
func (m *memoryStore) PutSourceMaterialWithExtractedUnits(_ context.Context, v domain.SourceMaterial, units ExtractedUnits) (domain.SourceMaterial, error) {
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

func TestImportPersistsOwnerScopedArtifactsAndHistory(t *testing.T) {
	store := &memoryStore{}
	result, err := NewService(store).Import(context.Background(), "owner-id", "de", fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Source.OwnerID != "owner-id" || result.History.OwnerID != "owner-id" {
		t.Fatalf("owner scope: %+v", result)
	}
	if store.resolved != 1 || store.linked != 1 {
		t.Fatalf("book linking: resolved=%d linked=%d", store.resolved, store.linked)
	}
	if result.Source.MediaType != MediaType() || len(result.Source.Content) == 0 || result.Source.FullText == "" {
		t.Fatalf("source: %+v", result.Source)
	}
	if err := store.units.ValidateOffsets(result.Source.FullText); err != nil {
		t.Fatalf("stored units: %v", err)
	}
	if result.History.Operation != "epub.import" || result.History.Status != "complete" || len(result.History.Details) == 0 {
		t.Fatalf("history: %+v", result.History)
	}
	if result.Source.ContentHash != ContentDigest(fixture(t)) {
		t.Fatalf("content digest: got=%s want=%s", result.Source.ContentHash, ContentDigest(fixture(t)))
	}
}

func TestImportRequiresAuthenticatedOwner(t *testing.T) {
	_, err := NewService(&memoryStore{}).Import(context.Background(), "", "de", fixture(t))
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("error = %v", err)
	}
}

func TestImportForAcquisitionReturnsExistingSourceWithoutRewritingIt(t *testing.T) {
	store := &acquisitionMemoryStore{existing: domain.SourceMaterial{ID: "existing-source", OwnerID: "owner-id", SourceIdentifier: "existing-source", Title: "Existing"}}
	result, err := NewService(store).ImportForAcquisition(context.Background(), "owner-id", "de", fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if !result.AlreadyPresent || result.Source.ID != "existing-source" {
		t.Fatalf("result=%+v", result)
	}
	if store.puts != 0 || store.histories != 0 {
		t.Fatalf("existing acquisition rewrote source: puts=%d histories=%d", store.puts, store.histories)
	}
	if store.resolved != 1 || store.linked != 1 {
		t.Fatalf("existing acquisition was not linked: resolved=%d linked=%d", store.resolved, store.linked)
	}
}
