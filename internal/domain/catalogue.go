package domain

import (
	"strings"
	"time"
)

// CatalogueIndexEntry is one entry an OPDS connection offers within the
// catalog sync scope. It mirrors upstream metadata and holds no learner state:
// the entry's ownership comes through ConnectionID. An empty CoverHref/MediaType
// means the entry advertises no cover.
type CatalogueIndexEntry struct {
	ConnectionID, EntryIdentifier, Title, Author, LanguageTag string
	CoverHref, CoverMediaType, EPUBHref                       string
	LastSeenSync                                              time.Time
}

// Normalized trims the whitespace-bearing metadata a catalog advertises so the
// index compares and stores one spelling of it.
func (e CatalogueIndexEntry) Normalized() CatalogueIndexEntry {
	e.EntryIdentifier = strings.TrimSpace(e.EntryIdentifier)
	e.Title = strings.TrimSpace(e.Title)
	e.Author = strings.TrimSpace(e.Author)
	e.LanguageTag = strings.TrimSpace(e.LanguageTag)
	e.CoverHref = strings.TrimSpace(e.CoverHref)
	e.CoverMediaType = strings.TrimSpace(e.CoverMediaType)
	e.EPUBHref = strings.TrimSpace(e.EPUBHref)
	return e
}
