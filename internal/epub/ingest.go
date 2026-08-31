package epub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

var ErrUnauthenticated = errors.New("epub: authenticated owner is required")

type Store interface {
	PutSourceMaterialWithExtractedUnits(context.Context, domain.SourceMaterial, ExtractedUnits) (domain.SourceMaterial, error)
	ResolveOrCreateBookForAcquisition(context.Context, string, string, string, string) (string, error)
	LinkSourceToBook(context.Context, string, string, string) error
	GetExtractedUnitSnapshot(context.Context, string, string) (string, domain.ExtractedUnits, error)
	ReplaceEPUBUnitClassifications(context.Context, string, string, []domain.EPUBUnitClassification) error
	PutProcessingHistory(context.Context, domain.ProcessingHistory) (domain.ProcessingHistory, error)
}

// AcquisitionSourceLookup is the optional owner-scoped lookup used by OPDS
// acquisition to make repeated downloads idempotent without changing the
// behavior of direct EPUB imports.
type AcquisitionSourceLookup interface {
	FindSourceMaterialForAcquisition(context.Context, string, string, string) (domain.SourceMaterial, bool, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

type ImportResult struct {
	Source         domain.SourceMaterial
	Book           ExtractedBook
	History        domain.ProcessingHistory
	AlreadyPresent bool
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
	return s.importBook(ctx, ownerID, language, content, book)
}

// ImportForAcquisition validates and imports an OPDS download. When the
// owner already has the same source identity or content, it returns that
// source without rewriting its extracted snapshot or processing history.
// Direct imports continue to use Import and retain their existing behavior.
func (s *Service) ImportForAcquisition(ctx context.Context, ownerID, language string, content []byte) (ImportResult, error) {
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
	contentHash := domain.EPUBContentDigest(content)
	if lookup, ok := s.store.(AcquisitionSourceLookup); ok {
		existing, found, lookupErr := lookup.FindSourceMaterialForAcquisition(ctx, ownerID, book.SourceIdentifier, contentHash)
		if lookupErr != nil {
			return ImportResult{}, fmt.Errorf("epub: look up existing source material: %w", lookupErr)
		}
		if found {
			if err = s.linkAcquiredSource(ctx, ownerID, language, existing, book.Title); err != nil {
				return ImportResult{}, err
			}
			return ImportResult{Source: existing, Book: book, AlreadyPresent: true}, nil
		}
	}
	return s.importBook(ctx, ownerID, language, content, book)
}

func (s *Service) importBook(ctx context.Context, ownerID, language string, content []byte, book ExtractedBook) (ImportResult, error) {
	contentHash := domain.EPUBContentDigest(content)
	source, err := s.store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: ownerID, Language: language, SourceIdentifier: book.SourceIdentifier, Title: book.Title, MediaType: MediaType(), ContentHash: contentHash, Content: content, FullText: book.FullText}, book.ExtractedUnits)
	if err != nil {
		return ImportResult{}, fmt.Errorf("epub: persist private source material: %w", err)
	}
	if err = s.linkAcquiredSource(ctx, ownerID, language, source, book.Title); err != nil {
		return ImportResult{}, fmt.Errorf("epub: link source material to My Books: %w", err)
	}
	snapshotID, persistedUnits, err := s.store.GetExtractedUnitSnapshot(ctx, ownerID, source.ID)
	if err != nil {
		return ImportResult{}, fmt.Errorf("epub: load extracted-unit snapshot: %w", err)
	}
	classifications, err := ClassifyUnits(snapshotID, persistedUnits)
	if err != nil {
		return ImportResult{}, fmt.Errorf("epub: classify extracted units: %w", err)
	}
	if err = s.store.ReplaceEPUBUnitClassifications(ctx, ownerID, source.ID, classifications); err != nil {
		return ImportResult{}, fmt.Errorf("epub: persist unit classifications: %w", err)
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

func (s *Service) linkAcquiredSource(ctx context.Context, ownerID, language string, source domain.SourceMaterial, fallbackTitle string) error {
	title := source.Title
	if title == "" {
		title = fallbackTitle
	}
	bookID, err := s.store.ResolveOrCreateBookForAcquisition(ctx, ownerID, source.SourceIdentifier, language, title)
	if err != nil {
		return fmt.Errorf("resolve acquisition book: %w", err)
	}
	if err = s.store.LinkSourceToBook(ctx, ownerID, bookID, source.ID); err != nil {
		return fmt.Errorf("link source %s to book %s: %w", source.ID, bookID, err)
	}
	return nil
}
