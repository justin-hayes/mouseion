package epub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

var ErrUnauthenticated = errors.New("epub: authenticated owner is required")

type Store interface {
	PutSourceMaterialWithExtractedSnapshot(context.Context, domain.SourceMaterial, ExtractedUnits, domain.SnapshotStructure) (domain.SourceMaterial, error)
	ResolveOrCreateBookForAcquisition(context.Context, string, string, string, string) (string, error)
	LinkSourceToBook(context.Context, string, string, string) error
	GetExtractedUnitSnapshot(context.Context, string, string) (string, domain.ExtractedUnits, error)
	PutProcessingHistory(context.Context, domain.ProcessingHistory) (domain.ProcessingHistory, error)
}

// AcquisitionSourceLookup is the optional owner-scoped lookup used by OPDS
// acquisition to make repeated downloads idempotent without changing the
// behavior of direct EPUB imports.
type AcquisitionSourceLookup interface {
	FindSourceMaterialForAcquisition(context.Context, string, string, string) (domain.SourceMaterial, bool, error)
}

type AcquisitionBookResolver interface {
	ResolveOrCreateBookForAcquisitionForBook(context.Context, string, string, string, string, string) (string, error)
}

// SourceStructureCheckEnqueuer schedules an extraction-only structure check for
// a source whose evidence just changed. It is optional so direct EPUB use and
// tests do not depend on River.
type SourceStructureCheckEnqueuer interface {
	EnqueueSourceStructureCheck(context.Context, string, string) error
}

type Service struct {
	store           Store
	now             func() time.Time
	structureChecks SourceStructureCheckEnqueuer
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

// WithSourceStructureCheckEnqueuer attaches the source structure check
// scheduler. It returns the same service so callers can chain construction.
func (s *Service) WithSourceStructureCheckEnqueuer(enqueuer SourceStructureCheckEnqueuer) *Service {
	s.structureChecks = enqueuer
	return s
}

type ImportResult struct {
	Source         domain.SourceMaterial
	Book           ExtractedBook
	History        domain.ProcessingHistory
	AlreadyPresent bool
}

// AcquisitionContent is an EPUB downloaded for acquisition, together with the
// Catalog file identity it arrived with. Until slimming lands the content is
// the Catalog file itself, but catalog digest resolution already has to work
// for revisions stored before slimming changed stored bytes.
type AcquisitionContent struct {
	Content       []byte
	CatalogDigest string
}

func (s *Service) Import(ctx context.Context, ownerID, language string, content []byte) (ImportResult, error) {
	if ownerID == "" {
		return ImportResult{}, ErrUnauthenticated
	}
	if language == "" {
		return ImportResult{}, errors.New("epub: language is required")
	}
	extraction, err := ExtractV2(content)
	if err != nil {
		return ImportResult{}, err
	}
	return s.importBook(ctx, ownerID, language, content, extraction)
}

// ImportForAcquisition validates and imports an OPDS download. When the
// owner already has the same source identity or content, it returns that
// source without rewriting its extracted snapshot or processing history.
// Direct imports continue to use Import and retain their existing behavior.
func (s *Service) ImportForAcquisition(ctx context.Context, ownerID, language string, acquisition AcquisitionContent) (ImportResult, error) {
	return s.importForAcquisition(ctx, ownerID, language, "", acquisition)
}

// ImportForAcquisitionForBook is the explicit promotion path from a
// metadata-only My Books identity. The selected Book is owner-validated by
// persistence before source evidence is linked.
func (s *Service) ImportForAcquisitionForBook(ctx context.Context, ownerID, language, bookID string, acquisition AcquisitionContent) (ImportResult, error) {
	if bookID == "" {
		return ImportResult{}, errors.New("epub: acquisition book is required")
	}
	return s.importForAcquisition(ctx, ownerID, language, bookID, acquisition)
}

func (s *Service) importForAcquisition(ctx context.Context, ownerID, language, bookID string, acquisition AcquisitionContent) (ImportResult, error) {
	if ownerID == "" {
		return ImportResult{}, ErrUnauthenticated
	}
	if language == "" {
		return ImportResult{}, errors.New("epub: language is required")
	}
	content := acquisition.Content
	extraction, err := ExtractV2(content)
	if err != nil {
		return ImportResult{}, err
	}
	book := extraction.Book
	contentHash := domain.EPUBContentDigest(content)
	existing, found, err := s.findAcquisitionSource(ctx, ownerID, book.SourceIdentifier, contentHash, acquisition.CatalogDigest)
	if err != nil {
		return ImportResult{}, err
	}
	if found {
		if err := s.linkAcquiredSourceForBook(ctx, ownerID, language, bookID, existing, book.Title); err != nil {
			return ImportResult{}, err
		}
		return ImportResult{Source: existing, Book: book, AlreadyPresent: true}, nil
	}
	return s.importBookForBook(ctx, ownerID, language, bookID, content, extraction)
}

// findAcquisitionSource resolves an existing revision by the Retained EPUB's
// digest first, then by the Catalog file's. A revision stored before slimming
// holds the Catalog file, so its digest is the catalog's, and re-acquiring it
// is not a content change.
func (s *Service) findAcquisitionSource(ctx context.Context, ownerID, sourceIdentifier, contentHash, catalogDigest string) (domain.SourceMaterial, bool, error) {
	lookup, ok := s.store.(AcquisitionSourceLookup)
	if !ok {
		return domain.SourceMaterial{}, false, nil
	}
	digests := []string{contentHash}
	if catalogDigest != "" && catalogDigest != contentHash {
		digests = append(digests, catalogDigest)
	}
	for _, digest := range digests {
		existing, found, err := lookup.FindSourceMaterialForAcquisition(ctx, ownerID, sourceIdentifier, digest)
		if err != nil {
			return domain.SourceMaterial{}, false, fmt.Errorf("epub: look up existing source material: %w", err)
		}
		if found {
			return existing, true, nil
		}
	}
	return domain.SourceMaterial{}, false, nil
}

func (s *Service) importBook(ctx context.Context, ownerID, language string, content []byte, extraction Extraction) (ImportResult, error) {
	return s.importBookForBook(ctx, ownerID, language, "", content, extraction)
}

func (s *Service) importBookForBook(ctx context.Context, ownerID, language, bookID string, content []byte, extraction Extraction) (ImportResult, error) {
	book := extraction.Book
	contentHash := domain.EPUBContentDigest(content)
	source, err := s.store.PutSourceMaterialWithExtractedSnapshot(ctx, domain.SourceMaterial{OwnerID: ownerID, Language: language, SourceIdentifier: book.SourceIdentifier, Title: book.Title, MediaType: MediaType(), ContentHash: contentHash, Content: content, FullText: book.FullText}, book.ExtractedUnits, extraction.Structure)
	if err != nil {
		return ImportResult{}, fmt.Errorf("epub: persist private source material: %w", err)
	}
	if err = s.linkAcquiredSourceForBook(ctx, ownerID, language, bookID, source, book.Title); err != nil {
		return ImportResult{}, fmt.Errorf("epub: link source material to My Books: %w", err)
	}
	s.enqueueSourceStructureCheck(ctx, ownerID, source.ID)
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

// enqueueSourceStructureCheck reports a scheduling failure without failing the
// already-committed import; the bounded startup scan retries the exact key.
func (s *Service) enqueueSourceStructureCheck(ctx context.Context, ownerID, sourceMaterialID string) {
	if s.structureChecks == nil {
		return
	}
	if err := s.structureChecks.EnqueueSourceStructureCheck(ctx, ownerID, sourceMaterialID); err != nil {
		log.Printf("mouseion: could not enqueue source structure check for %s: %v", sourceMaterialID, err)
	}
}

func (s *Service) linkAcquiredSourceForBook(ctx context.Context, ownerID, language, requestedBookID string, source domain.SourceMaterial, fallbackTitle string) error {
	title := source.Title
	if title == "" {
		title = fallbackTitle
	}
	bookID := requestedBookID
	var err error
	if bookID != "" {
		resolver, ok := s.store.(AcquisitionBookResolver)
		if !ok {
			return errors.New("epub: selected acquisition book cannot be promoted")
		}
		bookID, err = resolver.ResolveOrCreateBookForAcquisitionForBook(ctx, ownerID, bookID, source.SourceIdentifier, language, title)
	} else {
		bookID, err = s.store.ResolveOrCreateBookForAcquisition(ctx, ownerID, source.SourceIdentifier, language, title)
	}
	if err != nil {
		return fmt.Errorf("resolve acquisition book: %w", err)
	}
	if err = s.store.LinkSourceToBook(ctx, ownerID, bookID, source.ID); err != nil {
		return fmt.Errorf("link source %s to book %s: %w", source.ID, bookID, err)
	}
	return nil
}
