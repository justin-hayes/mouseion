package epub

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type memoryStore struct {
	source  domain.SourceMaterial
	history domain.ProcessingHistory
}

func (m *memoryStore) PutSourceMaterial(_ context.Context, v domain.SourceMaterial) (domain.SourceMaterial, error) {
	v.ID = "source-id"
	m.source = v
	return v, nil
}
func (m *memoryStore) PutProcessingHistory(_ context.Context, v domain.ProcessingHistory) (domain.ProcessingHistory, error) {
	v.ID = "history-id"
	m.history = v
	return v, nil
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
	if result.Source.MediaType != MediaType() || len(result.Source.Content) == 0 || result.Source.FullText == "" {
		t.Fatalf("source: %+v", result.Source)
	}
	if result.History.Operation != "epub.import" || result.History.Status != "complete" || len(result.History.Details) == 0 {
		t.Fatalf("history: %+v", result.History)
	}
	if result.Source.ContentHash[:7] != "sha256:" {
		t.Fatalf("hash: %s", result.Source.ContentHash)
	}
}

func TestImportRequiresAuthenticatedOwner(t *testing.T) {
	_, err := NewService(&memoryStore{}).Import(context.Background(), "", "de", fixture(t))
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("error = %v", err)
	}
}
