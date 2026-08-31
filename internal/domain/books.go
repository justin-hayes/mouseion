package domain

import (
	"errors"
	"strings"
	"time"
)

const (
	LanguageUnknown            = "unknown"
	LanguageChosen             = "chosen"
	AliasCatalogEntry          = "catalog_entry"
	AliasStrongBibliographic   = "strong_bibliographic"
	NamespaceSourceIdentifier  = "source_identifier"
	MetadataProvenanceBackfill = "source_materials_backfill"
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
