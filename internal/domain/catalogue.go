package domain

import "time"

// CatalogueIndexEntry is one entry an OPDS connection offers within the
// catalog sync scope. It mirrors upstream metadata and holds no learner state:
// the entry's ownership comes through ConnectionID. An empty CoverHref/MediaType
// means the entry advertises no cover.
type CatalogueIndexEntry struct {
	ConnectionID, EntryIdentifier, Title, Author, LanguageTag string
	CoverHref, CoverMediaType, EPUBHref                       string
	LastSeenSync                                              time.Time
}
