package fixtures

// Contract status: contractual. Recording a Catalog entry or Classical Reading
// unit as Previously read must match internal/storecontract scenarios shared
// with PostgreSQL (ADR 0088). The fixtures harness in storecontract_test.go
// runs them.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// MarkBookPreviouslyRead records an unstarted To Read Book as read outside
// Mouseion and clears its To Read disposition in one transition. A Book already
// read is unchanged, so a replay is idempotent.
func (s *Store) MarkBookPreviouslyRead(_ context.Context, owner, bookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.fixtureBookExists(owner, bookID) {
		return errNotFound
	}
	if s.fixtureBookIsCurrentReading(owner, bookID) {
		return domain.ErrBookIsCurrentReading
	}
	disposition := s.bookDispositionLocked(owner, bookID)
	if s.fixtureBookHasHistoryLocked(owner, bookID) {
		if disposition == domain.BookDispositionToRead {
			return domain.ErrPreviouslyReadIneligible
		}
		return nil
	}
	if disposition != domain.BookDispositionToRead || s.fixtureBookEverCurrentReading(owner, bookID) {
		return domain.ErrPreviouslyReadIneligible
	}
	if !s.markFixturePreviouslyReadLocked(owner, bookID) {
		return errNotFound
	}
	s.setFixtureDispositionLocked(owner, bookID, domain.BookDispositionInbox)
	return nil
}

// fixtureBookHasHistoryLocked reports whether the Book already has any reading
// completion, from either a finished reading or a Previously read assertion.
// Callers hold s.mu.
func (s *Store) fixtureBookHasHistoryLocked(owner, bookID string) bool {
	if _, ok := s.importedHistory[fixtureDispositionKey(owner, bookID)]; ok {
		return true
	}
	for _, completion := range s.readingHistory {
		if completion.OwnerID == owner && completion.BookID == bookID {
			return true
		}
	}
	return false
}

// MarkCatalogueEntryPreviouslyRead creates or restores the learner's Book for
// one indexed Catalog entry as Read. It records one Previously read assertion,
// downloads nothing and changes no Known vocabulary.
func (s *Store) MarkCatalogueEntryPreviouslyRead(_ context.Context, owner, connectionID, entryIdentifier string) (domain.CatalogueReadResult, error) {
	owner, connectionID, entryIdentifier = strings.TrimSpace(owner), strings.TrimSpace(connectionID), strings.TrimSpace(entryIdentifier)
	if owner == "" || connectionID == "" || entryIdentifier == "" {
		return domain.CatalogueReadResult{}, errors.New("fixtures: catalog previously read identity is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.getOpdsConnectionLocked(owner, connectionID); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	entry, ok := fixtureCatalogueEntryLocked(s.catalogueEntries[connectionID], entryIdentifier)
	if !ok {
		return domain.CatalogueReadResult{}, errNotFound
	}
	language := normalizeFixtureLanguage(entry.LanguageTag)
	if language == "" {
		return domain.CatalogueReadResult{}, errors.New("fixtures: catalog entry has no language")
	}
	s.ensureFixtureSupportedLanguageLocked(language, fixtureSupportedLanguageNameLocked(s.supported, language))
	if bookID := s.fixtureCatalogueAliasBookLocked(owner, connectionID, entryIdentifier); bookID != "" {
		return s.restoreReadFixtureCatalogueBookLocked(owner, bookID, entry, language)
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
		ID:           "fixture-catalogue-read-" + book.ID,
		OwnerID:      owner,
		BookID:       book.ID,
		ConnectionID: connectionID,
		AliasType:    domain.AliasCatalogEntry,
		Namespace:    domain.NamespaceSourceIdentifier,
		Value:        entryIdentifier,
	})
	s.setFixtureDispositionLocked(owner, book.ID, domain.BookDispositionInbox)
	s.markFixturePreviouslyReadLocked(owner, book.ID)
	return domain.CatalogueReadResult{Book: book, Created: true, Changed: true}, nil
}

// restoreReadFixtureCatalogueBookLocked reactivates the Book behind an existing
// catalog-entry alias, refreshes its metadata from the entry and records it as
// Read. Callers hold s.mu.
func (s *Store) restoreReadFixtureCatalogueBookLocked(owner, bookID string, entry domain.CatalogueIndexEntry, language string) (domain.CatalogueReadResult, error) {
	changed, err := s.fixtureRestoreReadBookLocked(owner, bookID)
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if index := s.fixtureMyBookIndexLocked(owner, bookID); index >= 0 {
		book := &s.myBooks[index]
		book.Book.Title = entry.Title
		book.Book.Author = entry.Author
		book.Book.LanguageState = domain.LanguageChosen
		book.Book.LanguageTag = language
		book.Book.UpdatedAt = fixtureJourneyTime
		return domain.CatalogueReadResult{Book: book.Book, Changed: changed}, nil
	}
	for i := range s.books {
		source := &s.books[i]
		if source.Source.OwnerID != owner || source.BookID != bookID {
			continue
		}
		source.BookTitle = entry.Title
		source.BookAuthor = entry.Author
		source.Source.Language = language
		return domain.CatalogueReadResult{Book: fixtureBookFromSource(owner, *source), Changed: changed}, nil
	}
	return domain.CatalogueReadResult{}, errNotFound
}

// fixtureRestoreReadBookLocked refuses the Current reading, reactivates the
// Book as visible and not-To-Read, and records its Previously read assertion.
// Callers hold s.mu.
func (s *Store) fixtureRestoreReadBookLocked(owner, bookID string) (bool, error) {
	if s.fixtureBookIsCurrentReading(owner, bookID) {
		return false, domain.ErrBookIsCurrentReading
	}
	changed := s.restoreFixtureBookMembershipLocked(owner, bookID, domain.BookDispositionInbox)
	return s.markFixturePreviouslyReadLocked(owner, bookID) || changed, nil
}

// MarkClassicalReadingUnitPreviouslyRead creates or restores the learner's Book
// for one current Reading unit as Read. It records one Previously read
// assertion and starts no preparation.
func (s *Store) MarkClassicalReadingUnitPreviouslyRead(_ context.Context, owner, readingUnitKey string) (domain.CatalogueReadResult, error) {
	owner, readingUnitKey = strings.TrimSpace(owner), strings.TrimSpace(readingUnitKey)
	if owner == "" || readingUnitKey == "" {
		return domain.CatalogueReadResult{}, errors.New("fixtures: classical previously read identity is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	definition, ok := s.classicalDefinitions[readingUnitKey]
	if !ok {
		return domain.CatalogueReadResult{}, fmt.Errorf("fixtures: classical Reading unit %q is not current", readingUnitKey)
	}
	s.ensureFixtureSupportedLanguageLocked(classical.Language, classical.LanguageDisplayName)
	if index := s.fixtureClassicalBookIndexLocked(owner, readingUnitKey); index >= 0 {
		bookID := s.myBooks[index].Book.ID
		changed, err := s.fixtureRestoreReadBookLocked(owner, bookID)
		if err != nil {
			return domain.CatalogueReadResult{}, err
		}
		return domain.CatalogueReadResult{Book: s.myBooks[index].Book, Changed: changed}, nil
	}
	book := s.newFixtureClassicalBookLocked(owner, definition)
	s.setFixtureDispositionLocked(owner, book.ID, domain.BookDispositionInbox)
	s.markFixturePreviouslyReadLocked(owner, book.ID)
	return domain.CatalogueReadResult{Book: book, Created: true, Changed: true}, nil
}

// markFixturePreviouslyReadLocked records one Previously read assertion for an
// active, chosen-language Book without any reading history. A Book already read
// or currently being read keeps its existing state. Callers hold s.mu.
func (s *Store) markFixturePreviouslyReadLocked(owner, bookID string) bool {
	key := fixtureDispositionKey(owner, bookID)
	if _, ok := s.importedHistory[key]; ok {
		return false
	}
	if !s.fixtureBookActiveLocked(owner, bookID) || s.fixtureBookIsCurrentReading(owner, bookID) {
		return false
	}
	var book domain.MyBook
	found := false
	for _, candidate := range s.myBooksForOwner(owner) {
		if candidate.Book.ID == bookID {
			book, found = candidate, true
			break
		}
	}
	if !found || book.Book.LanguageState != domain.LanguageChosen {
		return false
	}
	for _, completion := range s.readingHistory {
		if completion.OwnerID == owner && completion.BookID == bookID {
			return false
		}
	}
	s.importedHistory[key] = domain.ReadingCompletion{
		OwnerID: owner, BookID: bookID, Language: book.Book.LanguageTag,
		CompletedAt: time.Now().UTC(), Source: domain.ReadingCompletionPreviouslyRead,
	}
	return true
}

// setFixtureDispositionLocked writes one disposition and advances its revision
// only when the value really changes. Callers hold s.mu.
func (s *Store) setFixtureDispositionLocked(owner, bookID string, disposition domain.BookDisposition) bool {
	key := fixtureDispositionKey(owner, bookID)
	if s.bookDispositionLocked(owner, bookID) == disposition {
		return false
	}
	s.dispositionRevisions[key] = s.bookDispositionRevisionLocked(owner, bookID) + 1
	s.dispositions[key] = disposition
	return true
}
