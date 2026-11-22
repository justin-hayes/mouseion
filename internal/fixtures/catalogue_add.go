package fixtures

// Contract status: contractual. Catalog Add must match internal/storecontract
// scenarios shared with PostgreSQL (ADR 0088). The fixtures harness in
// storecontract_test.go runs them.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// AddCatalogueEntry creates the learner's To Read Book for one indexed Catalog
// entry, or restores the existing Book behind its alias. It downloads nothing
// and starts no job; the Add route submits preparation after this returns.
func (s *Store) AddCatalogueEntry(_ context.Context, owner, connectionID, entryIdentifier string) (domain.CatalogueAddResult, error) {
	owner, connectionID, entryIdentifier = strings.TrimSpace(owner), strings.TrimSpace(connectionID), strings.TrimSpace(entryIdentifier)
	if owner == "" || connectionID == "" || entryIdentifier == "" {
		return domain.CatalogueAddResult{}, errors.New("fixtures: catalog Add identity is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.getOpdsConnectionLocked(owner, connectionID); err != nil {
		return domain.CatalogueAddResult{}, err
	}
	entry, ok := fixtureCatalogueEntryLocked(s.catalogueEntries[connectionID], entryIdentifier)
	if !ok {
		return domain.CatalogueAddResult{}, errNotFound
	}
	language := normalizeFixtureLanguage(entry.LanguageTag)
	if language == "" {
		return domain.CatalogueAddResult{}, errors.New("fixtures: catalog entry has no language")
	}
	s.ensureFixtureSupportedLanguageLocked(language, fixtureSupportedLanguageNameLocked(s.supported, language))
	if bookID := s.fixtureCatalogueAliasBookLocked(owner, connectionID, entryIdentifier); bookID != "" {
		changed := s.restoreFixtureBookMembershipLocked(owner, bookID, domain.BookDispositionToRead)
		if index := s.fixtureMyBookIndexLocked(owner, bookID); index >= 0 {
			book := &s.myBooks[index]
			book.Book.Title = entry.Title
			book.Book.Author = entry.Author
			book.Book.LanguageState = domain.LanguageChosen
			book.Book.LanguageTag = language
			book.Book.UpdatedAt = fixtureJourneyTime
			return domain.CatalogueAddResult{Book: book.Book, Changed: changed}, nil
		}
		for i := range s.books {
			source := &s.books[i]
			if source.Source.OwnerID != owner || source.BookID != bookID {
				continue
			}
			source.BookTitle = entry.Title
			source.BookAuthor = entry.Author
			source.Source.Language = language
			return domain.CatalogueAddResult{Book: fixtureBookFromSource(owner, *source), Changed: changed}, nil
		}
		return domain.CatalogueAddResult{}, errNotFound
	}

	s.catalogueBookCount++
	book := domain.Book{
		ID:                 fmt.Sprintf("fixture-catalogue-%d", s.catalogueBookCount),
		OwnerID:            owner,
		Title:              entry.Title,
		Author:             entry.Author,
		MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        language,
		CreatedAt:          fixtureJourneyTime,
		UpdatedAt:          fixtureJourneyTime,
	}
	s.myBooks = append(s.myBooks, domain.MyBook{Book: book})
	s.aliases = append(s.aliases, domain.BookAlias{
		ID:           "fixture-catalogue-add-" + book.ID,
		OwnerID:      owner,
		BookID:       book.ID,
		ConnectionID: connectionID,
		AliasType:    domain.AliasCatalogEntry,
		Namespace:    domain.NamespaceSourceIdentifier,
		Value:        entryIdentifier,
	})
	s.restoreFixtureToReadLocked(owner, book.ID)
	return domain.CatalogueAddResult{Book: book, Created: true, Changed: true}, nil
}

// fixtureCatalogueEntryLocked finds one entry in a connection's local index.
func fixtureCatalogueEntryLocked(entries []domain.CatalogueIndexEntry, entryIdentifier string) (domain.CatalogueIndexEntry, bool) {
	for _, entry := range entries {
		if entry.EntryIdentifier == entryIdentifier {
			return entry, true
		}
	}
	return domain.CatalogueIndexEntry{}, false
}

// fixtureCatalogueAliasBookLocked resolves the Book an active catalog-entry
// alias names, including one whose membership was removed.
func (s *Store) fixtureCatalogueAliasBookLocked(owner, connectionID, entryIdentifier string) string {
	for _, alias := range s.aliases {
		if alias.OwnerID == owner && alias.ConnectionID == connectionID &&
			alias.AliasType == domain.AliasCatalogEntry && alias.Namespace == domain.NamespaceSourceIdentifier &&
			alias.Value == entryIdentifier {
			return alias.BookID
		}
	}
	return ""
}

func (s *Store) fixtureMyBookIndexLocked(owner, bookID string) int {
	for i := range s.myBooks {
		if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == bookID {
			return i
		}
	}
	return -1
}

// restoreFixtureBookMembershipLocked makes an existing Book active, visible and
// carry the requested disposition again. It reports whether any of those facts
// changed, so a replayed Add starts no preparation. Reading history and
// analysis stay untouched.
func (s *Store) restoreFixtureBookMembershipLocked(owner, bookID string, disposition domain.BookDisposition) bool {
	changed := false
	key := fixtureDispositionKey(owner, bookID)
	if _, removed := s.removedBooks[key]; removed {
		delete(s.removedBooks, key)
		changed = true
	}
	if state := s.visibility[key]; state.hidden {
		s.visibility[key] = fixtureVisibility{hidden: false, revision: state.revision + 1}
		changed = true
	}
	changed = s.setFixtureDispositionLocked(owner, bookID, disposition) || changed
	return changed
}

// fixtureSupportedLanguageNameLocked prefers the analyzer's display name and
// falls back to the tag, as derived study languages do.
func fixtureSupportedLanguageNameLocked(supported []domain.SupportedLanguage, language string) string {
	for _, candidate := range supported {
		if normalizeFixtureLanguage(candidate.Language) == language && strings.TrimSpace(candidate.DisplayName) != "" {
			return candidate.DisplayName
		}
	}
	return language
}
