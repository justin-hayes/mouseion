package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

const (
	LanguageUnknown                 = "unknown"
	LanguageChosen                  = "chosen"
	MetadataProvenanceCatalogueSync = "catalog_sync"
	MetadataProvenanceAcquisition   = "acquisition"
	AliasCatalogEntry               = "catalog_entry"
	AliasStrongBibliographic        = "strong_bibliographic"
	NamespaceSourceIdentifier       = "source_identifier"
	MetadataProvenanceBackfill      = "source_materials_backfill"
	BookCoverNone                   = "none"
	BookCoverPending                = "pending"
	BookCoverAvailable              = "available"
	BookCoverUnavailable            = "unavailable"
)

type CatalogueSyncState string

const (
	CatalogueSyncSyncing CatalogueSyncState = "syncing"
	CatalogueSyncSynced  CatalogueSyncState = "synced"
	CatalogueSyncFailed  CatalogueSyncState = "failed"
)

type CatalogueSyncStatus struct {
	OwnerID, ConnectionID string
	State                 CatalogueSyncState
	LastError             string
	LastSyncedAt          *time.Time
	LastUpsertedCount     int
	UpdatedAt             time.Time
}

type BookEvidenceState string

const (
	BookUnavailable        BookEvidenceState = "unavailable"
	BookNotAcquired        BookEvidenceState = "not_acquired"
	BookAcquiredUnassessed BookEvidenceState = "acquired_unassessed"
	BookAnalyzed           BookEvidenceState = "analyzed"
	BookStale              BookEvidenceState = "stale"
)

type Book struct {
	ID, OwnerID, Title, Author, MetadataProvenance, LanguageState, LanguageTag string
	CreatedAt, UpdatedAt                                                       time.Time
}

type BookMembership struct {
	OwnerID, BookID, State string
	CreatedAt              time.Time
	ActivatedAt, RemovedAt *time.Time
}

type BookAlias struct {
	ID, OwnerID, BookID, ConnectionID, AliasType, Namespace, Value string
	CreatedAt                                                      time.Time
}

// BookCover is the byte-free cover projection used by collection views.
type BookCover struct {
	State  string
	Width  int
	Height int
}

// BookCoverResource is the owner-scoped stored resource read by the cover
// endpoint. It is deliberately not embedded in MyBook or any collection query.
type BookCoverResource struct {
	OwnerID, BookID, MediaType, ContentHash string
	Bytes                                   []byte
	Width, Height                           int
}

// BookCoverRetrieval is the owner-scoped cover state read by the retrieval
// worker before it fetches anything. It names the selected Catalog entry so the
// worker can reload the current entry without persisting URLs or credentials.
type BookCoverRetrieval struct {
	State                    string
	SelectedConnectionID     string
	SelectedSourceIdentifier string
	FailureReason            string
	AdvertisedAt             time.Time
}

// MyBook is the complete learner-facing My Books read model. Acquired is nil
// for metadata-only membership; when present it contains only the current
// owner-scoped acquired source and its derived analysis state.
type MyBook struct {
	Book            Book
	Cover           BookCover
	Acquired        *SourceMaterialSummary
	JourneyMember   bool
	JourneyGoal     bool
	JourneyRevision int64
}

// EvidenceState classifies the acquired evidence shown in My Books.
func (m MyBook) EvidenceState() BookEvidenceState {
	if m.Acquired == nil {
		return BookNotAcquired
	}
	return m.Acquired.EvidenceState()
}

func NewBook(ownerID, title, metadataProvenance, languageState, languageTag string) (Book, error) {
	if languageState == LanguageChosen {
		languageTag = canonicalization.NormalizeLanguage(languageTag)
	}
	b := Book{OwnerID: ownerID, Title: title, MetadataProvenance: metadataProvenance, LanguageState: languageState, LanguageTag: languageTag}
	return b, b.Validate()
}

func (b Book) Validate() error {
	if strings.TrimSpace(b.OwnerID) == "" {
		return errors.New("domain: book owner is required")
	}
	if strings.TrimSpace(b.Title) == "" {
		return errors.New("domain: book title is required")
	}
	if strings.TrimSpace(b.MetadataProvenance) == "" {
		return errors.New("domain: book metadata provenance is required")
	}
	switch b.MetadataProvenance {
	case MetadataProvenanceCatalogueSync, MetadataProvenanceAcquisition, MetadataProvenanceBackfill:
	default:
		return errors.New("domain: invalid book metadata provenance")
	}
	switch b.LanguageState {
	case LanguageUnknown:
		if b.LanguageTag != "" {
			return errors.New("domain: unknown book language cannot have a tag")
		}
	case LanguageChosen:
		if strings.TrimSpace(b.LanguageTag) == "" {
			return errors.New("domain: chosen book language requires a tag")
		}
		if b.LanguageTag != canonicalization.NormalizeLanguage(b.LanguageTag) {
			return errors.New("domain: chosen book language must be canonical")
		}
	default:
		return errors.New("domain: invalid book language state")
	}
	return nil
}

func (m BookMembership) Validate() error {
	if strings.TrimSpace(m.OwnerID) == "" || strings.TrimSpace(m.BookID) == "" {
		return errors.New("domain: book membership identity is required")
	}
	if m.State != "active" && m.State != "removed" {
		return errors.New("domain: invalid book membership state")
	}
	if (m.State == "active") != (m.RemovedAt == nil) {
		return errors.New("domain: book membership removal timestamp is inconsistent")
	}
	return nil
}

func (a BookAlias) Validate() error {
	if strings.TrimSpace(a.OwnerID) == "" || strings.TrimSpace(a.BookID) == "" {
		return errors.New("domain: book alias identity is required")
	}
	if a.AliasType != AliasCatalogEntry && a.AliasType != AliasStrongBibliographic {
		return errors.New("domain: invalid book alias type")
	}
	if strings.TrimSpace(a.Namespace) == "" || strings.TrimSpace(a.Value) == "" {
		return errors.New("domain: book alias namespace and value are required")
	}
	return nil
}
