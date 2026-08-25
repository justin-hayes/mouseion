package epub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

var ErrUnauthenticated = errors.New("epub: authenticated owner is required")

type Store interface {
	PutSourceMaterialWithExtractedUnits(context.Context, domain.SourceMaterial, ExtractedUnits) (domain.SourceMaterial, error)
	PutProcessingHistory(context.Context, domain.ProcessingHistory) (domain.ProcessingHistory, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

type ImportResult struct {
	Source  domain.SourceMaterial
	Book    ExtractedBook
	History domain.ProcessingHistory
}

func (s *Service) Import(ctx context.Context, ownerID, language string, content []byte) (ImportResult, error) {
	if ownerID == "" {
		return ImportResult{}, ErrUnauthenticated
	}
	if language == "" {
		return ImportResult{}, errors.New("epub: language is required")
	}
	book, err := Extract(content)
	if err != nil {
		return ImportResult{}, err
	}
	sum := sha256.Sum256(content)
	source, err := s.store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: ownerID, Language: language, SourceIdentifier: book.SourceIdentifier, Title: book.Title, MediaType: MediaType(), ContentHash: "sha256:" + hex.EncodeToString(sum[:]), Content: content, FullText: book.FullText}, book.ExtractedUnits)
	if err != nil {
		return ImportResult{}, fmt.Errorf("epub: persist private source material: %w", err)
	}
	details, err := json.Marshal(struct {
		SourceMaterialID string    `json:"source_material_id"`
		SourceIdentifier string    `json:"source_identifier"`
		Chapters         []Chapter `json:"chapters"`
	}{source.ID, book.SourceIdentifier, book.Chapters})
	if err != nil {
		return ImportResult{}, fmt.Errorf("epub: encode source locations: %w", err)
	}
	completed := s.now().UTC()
	history, err := s.store.PutProcessingHistory(ctx, domain.ProcessingHistory{OwnerID: ownerID, Operation: "epub.import", Status: "complete", Details: details, CompletedAt: &completed})
	if err != nil {
		return ImportResult{}, fmt.Errorf("epub: record processing history: %w", err)
	}
	return ImportResult{Source: source, Book: book, History: history}, nil
}
