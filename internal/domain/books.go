package domain

import (
	"errors"
	"strings"
	"time"
)

const (
	LanguageUnknown                 = "unknown"
	LanguageChosen                  = "chosen"
	MetadataProvenanceManualEntry   = "manual_entry"
	MetadataProvenanceCatalogueSync = "catalog_sync"
	AliasCatalogEntry               = "catalog_entry"
	AliasStrongBibliographic        = "strong_bibliographic"
	NamespaceSourceIdentifier       = "source_identifier"
	MetadataProvenanceBackfill      = "source_materials_backfill"
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

type MyBookEvidenceState string

const (
	MyBookUnavailable        MyBookEvidenceState = "unavailable"
	MyBookNotAcquired        MyBookEvidenceState = "not_acquired"
	MyBookAcquiredUnassessed MyBookEvidenceState = "acquired_unassessed"
	MyBookAnalyzed           MyBookEvidenceState = "analyzed"
	MyBookStale              MyBookEvidenceState = "stale"
)

type Book struct {
	ID, OwnerID, Title, MetadataProvenance, LanguageState, LanguageTag string
	CreatedAt, UpdatedAt                                               time.Time
}

type BookMembership struct {
	OwnerID, BookID, State string
	CreatedAt              time.Time
	ActivatedAt, RemovedAt *time.Time
}

type BookAlias struct {
	ID, OwnerID, BookID, AliasType, Namespace, Value string
	CreatedAt                                        time.Time
}

// MyBook is the complete learner-facing My Books read model. Acquired is nil
// for metadata-only membership; when present it contains only the current
// owner-scoped acquired source and its derived analysis state.
type MyBook struct {
	Book          Book
	Acquired      *SourceMaterialSummary
	EvidenceState MyBookEvidenceState
}

func NewBook(ownerID, title, metadataProvenance, languageState, languageTag string) (Book, error) {
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
	switch b.LanguageState {
	case LanguageUnknown:
		if b.LanguageTag != "" {
			return errors.New("domain: unknown book language cannot have a tag")
		}
	case LanguageChosen:
		if strings.TrimSpace(b.LanguageTag) == "" {
			return errors.New("domain: chosen book language requires a tag")
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
