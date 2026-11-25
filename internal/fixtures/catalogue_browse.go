package fixtures

// Contract status: contractual. The Catalog browse read must match
// internal/storecontract scenarios shared with PostgreSQL (ADR 0088); the canned
// fixture index data itself is illustrative.

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// BrowseCatalogueEntries is the Catalog read the browser fixture shares with
// PostgreSQL through internal/storecontract. It reads only the local index and
// never contacts an OPDS server.
func (s *Store) BrowseCatalogueEntries(_ context.Context, owner, connectionID string, query domain.CatalogueQuery) (domain.CataloguePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.getOpdsConnectionLocked(owner, connectionID); err != nil {
		return domain.CataloguePage{}, nil
	}
	owned := make(map[string]string)
	for _, alias := range s.aliases {
		if alias.OwnerID != owner || alias.ConnectionID != connectionID {
			continue
		}
		if alias.AliasType != domain.AliasCatalogEntry || alias.Namespace != domain.NamespaceSourceIdentifier {
			continue
		}
		if s.fixtureBookActiveLocked(owner, alias.BookID) {
			owned[alias.Value] = alias.BookID
		}
	}
	return domain.BrowseCataloguePage(s.catalogueEntries[connectionID], owned, query), nil
}

// SeedCatalogueEntries installs one connection's Catalog index for a test or
// browser scenario. The fixture never syncs, so seeding replaces the local
// mirror directly. An unknown owner or connection is an error rather than a
// silent no-op.
func (s *Store) SeedCatalogueEntries(owner, connectionID string, entries []domain.CatalogueIndexEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.getOpdsConnectionLocked(owner, connectionID); err != nil {
		return err
	}
	s.catalogueEntries[connectionID] = append([]domain.CatalogueIndexEntry(nil), entries...)
	return nil
}

// getOpdsConnectionLocked is the owner-scoped connection check that avoids
// re-locking.
func (s *Store) getOpdsConnectionLocked(owner, id string) error {
	for _, connection := range s.connections {
		if connection.ID == id && connection.OwnerID == owner {
			return nil
		}
	}
	return errNotFound
}

// fixtureCatalogueIndexEntries is the canned local mirror each fixture
// connection browses. Titles intentionally carry accents and mixed case so
// browser acceptance can exercise the Go search key without an OPDS request.
func fixtureCatalogueIndexEntries() map[string][]domain.CatalogueIndexEntry {
	return map[string][]domain.CatalogueIndexEntry{
		"fixture-connection": {
			{ConnectionID: "fixture-connection", EntryIdentifier: "fixture-entry", Title: "Metadata-only migration book", Author: "Fixture Catalogue Author", LanguageTag: "de", EPUBHref: "https://fixture.invalid/fixture-entry.epub", LastSeenSync: fixtureJourneyTime},
			{ConnectionID: "fixture-connection", EntryIdentifier: "fixture-uber", Title: "Über die Berge", Author: "Anna Berg", LanguageTag: "de", EPUBHref: "https://fixture.invalid/uber.epub", LastSeenSync: fixtureJourneyTime},
			{ConnectionID: "fixture-connection", EntryIdentifier: "fixture-weg", Title: "Der lange Weg nach Hause", Author: "Mara Weiss", LanguageTag: "de", EPUBHref: "https://fixture.invalid/weg.epub", LastSeenSync: fixtureJourneyTime},
			{ConnectionID: "fixture-connection", EntryIdentifier: "fixture-sommer", Title: "Ein kurzer Sommer", Author: "Bernd Alt", LanguageTag: "de", EPUBHref: "https://fixture.invalid/sommer.epub", LastSeenSync: fixtureJourneyTime},
			{ConnectionID: "fixture-connection", EntryIdentifier: "fixture-odipus", Title: "Ödipus auf Kolonos", Author: "Hómēr Nachfolger", LanguageTag: "de", EPUBHref: "https://fixture.invalid/odipus.epub", LastSeenSync: fixtureJourneyTime},
			{ConnectionID: "fixture-connection", EntryIdentifier: "fixture-gedicht", Title: "Gedichte aus dem Norden", Author: "Klara Frost", LanguageTag: "de", EPUBHref: "https://fixture.invalid/gedicht.epub", LastSeenSync: fixtureJourneyTime},
			{ConnectionID: "fixture-connection", EntryIdentifier: "fixture-roman", Title: "Der stille Roman", Author: "Otto Sturm", LanguageTag: "de", EPUBHref: "https://fixture.invalid/roman.epub", LastSeenSync: fixtureJourneyTime},
			{ConnectionID: "fixture-connection", EntryIdentifier: "fixture-citta", Title: "La città delle meraviglie", Author: "Luca Bianchi", LanguageTag: "it", EPUBHref: "https://fixture.invalid/citta.epub", LastSeenSync: fixtureJourneyTime},
		},
		"fixture-browser-sync-connection": {
			{ConnectionID: "fixture-browser-sync-connection", EntryIdentifier: "fixture-browser-sync-entry", Title: "Browser sync metadata book", Author: "Browser Author", LanguageTag: "de", EPUBHref: "https://browser-sync.fixture.invalid/entry.epub", LastSeenSync: fixtureJourneyTime},
			{ConnectionID: "fixture-browser-sync-connection", EntryIdentifier: "fixture-browser-second", Title: "Browser sync second book", Author: "Browser Author", LanguageTag: "de", EPUBHref: "https://browser-sync.fixture.invalid/second.epub", LastSeenSync: fixtureJourneyTime},
		},
	}
}

// SeedCatalogEntryCover retains an image for one indexed entry, marking the
// entry's card as having a retained cover. The fixture never fetches covers.
func (s *Store) SeedCatalogEntryCover(owner, connectionID, entryIdentifier string, resource domain.CatalogEntryCoverResource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.getOpdsConnectionLocked(owner, connectionID); err != nil {
		return err
	}
	for i := range s.catalogueEntries[connectionID] {
		if s.catalogueEntries[connectionID][i].EntryIdentifier == entryIdentifier {
			s.catalogueEntries[connectionID][i].CoverRetained = true
			if s.entryCovers == nil {
				s.entryCovers = make(map[string]domain.CatalogEntryCoverResource)
			}
			s.entryCovers[connectionID+"\x00"+entryIdentifier] = resource
			return nil
		}
	}
	return errNotFound
}

// GetCatalogEntryCoverResource serves a seeded entry cover to its owner.
func (s *Store) GetCatalogEntryCoverResource(_ context.Context, owner, connectionID, entryIdentifier string) (domain.CatalogEntryCoverResource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.getOpdsConnectionLocked(owner, connectionID); err != nil {
		return domain.CatalogEntryCoverResource{}, domain.ErrNotFound
	}
	resource, ok := s.entryCovers[connectionID+"\x00"+entryIdentifier]
	if !ok {
		return domain.CatalogEntryCoverResource{}, domain.ErrNotFound
	}
	return resource, nil
}
